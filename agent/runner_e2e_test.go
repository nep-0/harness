package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunTurnStreamsTextAgainstLocalCompatibleEndpoint(t *testing.T) {
	server := completionServer(t, func(_ int, _ request) []streamChunk {
		return []streamChunk{{`{"role":"assistant","content":"hello "}`, ""}, {`{"content":"world"}`, "stop"}}
	})
	defer server.Close()
	var text string
	runner := testRunner(t, server.URL, nil, func(event Event) error {
		if event.Type == EventTextDelta {
			text += event.Delta
		}
		return nil
	})
	snapshot, err := runner.RunTurn(context.Background(), RunSnapshot{}, []Message{{Role: RoleUser, Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello world" || snapshot.Transcript[1].Content != "hello world" {
		t.Fatalf("text=%q transcript=%#v", text, snapshot.Transcript)
	}
}

func TestRunTurnExecutesToolsAndSendsFailuresBackToModel(t *testing.T) {
	server := completionServer(t, func(call int, body request) []streamChunk {
		if call == 1 {
			return []streamChunk{{`{"role":"assistant","tool_calls":[{"index":0,"id":"invalid","type":"function","function":{"name":"echo","arguments":"{"}},{"index":1,"id":"failed","type":"function","function":{"name":"echo","arguments":"{\\"value\\":\\"ok\\"}"}}]}`, "tool_calls"}}
		}
		if len(body.Messages) != 4 || body.Messages[2].ToolCallID != "invalid" || body.Messages[3].ToolCallID != "failed" {
			t.Fatalf("tool results %#v", body.Messages)
		}
		return []streamChunk{{`{"role":"assistant","content":"recovered"}`, "stop"}}
	})
	defer server.Close()
	runner := testRunner(t, server.URL, []Tool{{Name: "echo", Handler: func(context.Context, json.RawMessage) (string, error) { return "", errors.New("boom") }}}, nil)
	snapshot, err := runner.RunTurn(context.Background(), RunSnapshot{}, []Message{{Role: RoleUser, Content: "tools"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot.Transcript[len(snapshot.Transcript)-1].Content; got != "recovered" {
		t.Fatalf("final=%q", got)
	}
	if snapshot.Transcript[2].Content != `tool "echo" received invalid JSON arguments` || snapshot.Transcript[3].Content != `tool "echo" failed` {
		t.Fatalf("results %#v", snapshot.Transcript)
	}
}

func TestRunTurnExecutesSuccessfulToolCall(t *testing.T) {
	server := completionServer(t, func(call int, body request) []streamChunk {
		if call == 1 {
			return []streamChunk{{`{"role":"assistant","tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"echo","arguments":"{\\"value\\":\\"ok\\"}"}}]}`, "tool_calls"}}
		}
		if len(body.Messages) != 3 || body.Messages[2].ToolCallID != "call-1" {
			t.Fatalf("tool request %#v", body.Messages)
		}
		return []streamChunk{{`{"role":"assistant","content":"done"}`, "stop"}}
	})
	defer server.Close()
	runner := testRunner(t, server.URL, []Tool{{Name: "echo", Handler: func(_ context.Context, arguments json.RawMessage) (string, error) { return string(arguments), nil }}}, nil)
	snapshot, err := runner.RunTurn(context.Background(), RunSnapshot{}, []Message{{Role: RoleUser, Content: "go"}})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Transcript[2].Content != `{"value":"ok"}` || snapshot.Transcript[3].Content != "done" {
		t.Fatalf("transcript %#v", snapshot.Transcript)
	}
}

func TestRunTurnHonorsCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	runner := testRunner(t, server.URL, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := runner.RunTurn(ctx, RunSnapshot{}, []Message{{Role: RoleUser, Content: "wait"}})
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancellation error")
		}
	case <-time.After(time.Second):
		t.Fatal("RunTurn did not stop after cancellation")
	}
}

type streamChunk struct{ delta, finish string }
type request struct {
	Messages []struct {
		Role       string `json:"role"`
		ToolCallID string `json:"tool_call_id"`
	} `json:"messages"`
}

func completionServer(t *testing.T, response func(int, request) []streamChunk) *httptest.Server {
	t.Helper()
	var calls atomic.Int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path %s", r.URL.Path)
		}
		var body request
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range response(int(calls.Add(1)), body) {
			fmt.Fprintf(w, "data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":%s,\"finish_reason\":%q}]}\n\n", chunk.delta, chunk.finish)
			w.(http.Flusher).Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}
func testRunner(t *testing.T, baseURL string, tools []Tool, event func(Event) error) *Runner {
	t.Helper()
	options := []RunnerOption{WithBaseURL(baseURL), WithModel("test"), WithEventHandler(event)}
	for _, tool := range tools {
		options = append(options, WithTool(tool))
	}
	runner, err := NewRunner(options...)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}
