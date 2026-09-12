package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nep-0/harness/agent"
)

func testWorkspace(t *testing.T) *Workspace {
	t.Helper()
	w, err := New(Config{Root: t.TempDir(), MaxReadBytes: 64, MaxMatches: 2, MaxOutputBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}
func call(t *testing.T, h func(context.Context, json.RawMessage) (string, error), v any) (string, error) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return h(context.Background(), b)
}

func TestReadFileConfinesAndNumbersContent(t *testing.T) {
	w := testWorkspace(t)
	if err := os.WriteFile(filepath.Join(w.root.Name(), "source.txt"), []byte("one\ntwo\nthree\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := call(t, w.readFile, map[string]any{"path": "source.txt", "start_line": 2, "end_line": 3})
	if err != nil {
		t.Fatal(err)
	}
	if out != "2:two\n3:three\n" {
		t.Fatalf("output = %q", out)
	}
	if _, err := call(t, w.readFile, map[string]any{"path": "../outside"}); err == nil {
		t.Fatal("expected escape rejection")
	}
}
func TestListDirAndGrepBounded(t *testing.T) {
	w := testWorkspace(t)
	r := w.root.Name()
	if err := os.Mkdir(filepath.Join(r, "dir"), 0700); err != nil {
		t.Fatal(err)
	}
	for n, s := range map[string]string{"b.txt": "needle\nneedle\n", "a.txt": "needle\n"} {
		if err := os.WriteFile(filepath.Join(r, n), []byte(s), 0600); err != nil {
			t.Fatal(err)
		}
	}
	list, err := call(t, w.listDir, map[string]any{"path": "."})
	if err != nil {
		t.Fatal(err)
	}
	if list != "a.txt\nb.txt\ndir/\n" {
		t.Fatalf("list=%q", list)
	}
	hits, err := call(t, w.grepFiles, map[string]any{"path": ".", "pattern": "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if hits != "a.txt:1:needle\nb.txt:1:needle" {
		t.Fatalf("hits=%q", hits)
	}
}
func TestGrepCancellationAndReadBounds(t *testing.T) {
	w := testWorkspace(t)
	r := w.root.Name()
	if err := os.WriteFile(filepath.Join(r, "large"), []byte(strings.Repeat("x", 65)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r, "match"), []byte("needle"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := call(t, w.grepFiles, map[string]any{"path": ".", "pattern": "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "match:1:needle" {
		t.Fatalf("out=%q", out)
	}
	b, _ := json.Marshal(map[string]any{"path": ".", "pattern": "needle"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = w.grepFiles(ctx, b); err != context.Canceled {
		t.Fatalf("err=%v", err)
	}
}
func TestListDirOutputBound(t *testing.T) {
	w := testWorkspace(t)
	w.maxOutputBytes = 5
	if err := os.WriteFile(filepath.Join(w.root.Name(), "alpha"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	out, err := call(t, w.listDir, map[string]any{"path": "."})
	if err != nil {
		t.Fatal(err)
	}
	if out != "alpha[output truncated]\n" {
		t.Fatalf("out=%q", out)
	}
}
func TestApplyPatchRejectsStaleContent(t *testing.T) {
	w := testWorkspace(t)
	p := filepath.Join(w.root.Name(), "source")
	if err := os.WriteFile(p, []byte("old"), 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, w.applyPatch, map[string]string{"path": "source", "expected": "old", "replacement": "new"}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, w.applyPatch, map[string]string{"path": "source", "expected": "old", "replacement": "other"}); err == nil {
		t.Fatal("expected stale error")
	}
}

func TestApplyPatchSerializesSameBaseEdits(t *testing.T) {
	w := testWorkspace(t)
	if err := os.WriteFile(filepath.Join(w.root.Name(), "source"), []byte("base"), 0600); err != nil {
		t.Fatal(err)
	}
	arguments := []json.RawMessage{
		json.RawMessage(`{"path":"source","expected":"base","replacement":"one"}`),
		json.RawMessage(`{"path":"source","expected":"base","replacement":"two"}`),
	}
	results := make(chan error, len(arguments))
	for _, argument := range arguments {
		go func(argument json.RawMessage) {
			_, err := w.applyPatch(context.Background(), argument)
			results <- err
		}(argument)
	}
	successes := 0
	for range arguments {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful patches = %d", successes)
	}
}
func TestShellTimeoutCleanupAndBounds(t *testing.T) {
	w := testWorkspace(t)
	w.maxOutputBytes = 16
	out, err := call(t, w.shell, map[string]any{"argv": []string{"sh", "-c", "printf '12345678901234567890'"}, "timeout_seconds": 1})
	if err != nil {
		t.Fatal(err)
	}
	if out != "1234567890123456\n[output truncated]" {
		t.Fatalf("out=%q", out)
	}
	for _, v := range []map[string]any{{"argv": []string{"true"}}, {"argv": []string{"true"}, "timeout_seconds": 301}} {
		if _, err := call(t, w.shell, v); err == nil || !strings.Contains(err.Error(), "timeout_seconds") {
			t.Fatalf("err=%v", err)
		}
	}
	at := time.Now()
	_, err = call(t, w.shell, map[string]any{"argv": []string{"sh", "-c", "sleep 5 & wait"}, "timeout_seconds": 1})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err=%v", err)
	}
	if time.Since(at) > 3*time.Second {
		t.Fatal("child survived timeout")
	}
}

func TestShellFailureReachesRunnerTranscript(t *testing.T) {
	w := testWorkspace(t)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(response, "data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call\",\"type\":\"function\",\"function\":{\"name\":\"shell\",\"arguments\":\"{\\\"argv\\\":[\\\"sh\\\",\\\"-c\\\",\\\"echo compiler-error >&2; exit 7\\\"],\\\"timeout_seconds\\\":1}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
		fmt.Fprint(response, "data: [DONE]\n\n")
	}))
	defer server.Close()
	runner, err := agent.NewRunner(agent.WithBaseURL(server.URL), agent.WithModel("test"), agent.WithMaxTurns(1), agent.WithTool(w.ShellTool()))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := runner.RunTurn(context.Background(), agent.RunSnapshot{}, []agent.Message{{Role: agent.RoleUser, Content: "run"}})
	if err != agent.ErrMaxTurns {
		t.Fatalf("run error = %v", err)
	}
	if len(snapshot.Transcript) < 3 || !strings.Contains(snapshot.Transcript[2].Content, "compiler-error") || !strings.Contains(snapshot.Transcript[2].Content, "exit status 7") {
		t.Fatalf("tool result = %#v", snapshot.Transcript)
	}
}
