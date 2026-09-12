package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nep-0/harness/agent"
)

func toolFailure(err error) string { return "error: " + err.Error() }
func toolFailureWithOutput(err error, output string) string {
	if output == "" {
		return toolFailure(err)
	}
	return toolFailure(err) + "\noutput:\n" + output
}

// ApplyPatchTool atomically replaces one existing UTF-8 workspace file when its content exactly matches expected. Multi-file transactions are intentionally unsupported.
func (w *Workspace) ApplyPatchTool() agent.Tool {
	return agent.Tool{Name: "apply_patch", Description: "Atomically replace one existing UTF-8 workspace file when its current content exactly matches expected content.", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"expected":{"type":"string"},"replacement":{"type":"string"}},"required":["path","expected","replacement"],"additionalProperties":false}`), Handler: w.applyPatchResult}
}
func (w *Workspace) applyPatchResult(ctx context.Context, arguments json.RawMessage) (string, error) {
	result, err := w.applyPatch(ctx, arguments)
	if err != nil {
		return toolFailure(err), nil
	}
	return result, nil
}
func (w *Workspace) applyPatch(_ context.Context, arguments json.RawMessage) (string, error) {
	w.patchMu.Lock()
	defer w.patchMu.Unlock()
	var in struct {
		Path        string `json:"path"`
		Expected    string `json:"expected"`
		Replacement string `json:"replacement"`
	}
	if err := decode(arguments, &in); err != nil {
		return "", err
	}
	name, err := cleanPath(in.Path)
	if err != nil {
		return "", err
	}
	current, err := w.root.ReadFile(name)
	if err != nil {
		return "", err
	}
	if string(current) != in.Expected {
		return "", fmt.Errorf("coding: stale content for %q", in.Path)
	}
	info, err := w.root.Stat(name)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("coding: %q is a directory", in.Path)
	}
	temp := ".apply_patch.tmp"
	for i := 0; ; i++ {
		if i > 100 {
			return "", fmt.Errorf("coding: cannot allocate patch temporary file")
		}
		if i > 0 {
			temp = fmt.Sprintf(".apply_patch.%d.tmp", i)
		}
		f, e := w.root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if os.IsExist(e) {
			continue
		}
		if e != nil {
			return "", e
		}
		_, e = f.WriteString(in.Replacement)
		if closeErr := f.Close(); e == nil {
			e = closeErr
		}
		if e != nil {
			_ = w.root.Remove(temp)
			return "", e
		}
		break
	}
	defer w.root.Remove(temp)
	current, err = w.root.ReadFile(name)
	if err != nil {
		return "", err
	}
	if string(current) != in.Expected {
		return "", fmt.Errorf("coding: stale content for %q", in.Path)
	}
	if err := w.root.Rename(temp, name); err != nil {
		return "", err
	}
	return "updated: " + name, nil
}

// ShellTool runs a trusted, unrestricted argv command; callers must use an OS sandbox when host access is unacceptable.
func (w *Workspace) ShellTool() agent.Tool {
	return agent.Tool{Name: "shell", Description: "Run a trusted, unrestricted argv command in the workspace. It is not sandboxed and may access host resources.", Parameters: json.RawMessage(`{"type":"object","properties":{"argv":{"type":"array","minItems":1,"items":{"type":"string"}},"timeout_seconds":{"type":"integer","minimum":1,"maximum":300}},"required":["argv","timeout_seconds"],"additionalProperties":false}`), Handler: w.shellResult}
}
func (w *Workspace) shellResult(ctx context.Context, arguments json.RawMessage) (string, error) {
	result, err := w.shell(ctx, arguments)
	if err != nil {
		return toolFailureWithOutput(err, result), nil
	}
	return result, nil
}
func (w *Workspace) shell(ctx context.Context, arguments json.RawMessage) (string, error) {
	var in struct {
		Argv           []string `json:"argv"`
		TimeoutSeconds int      `json:"timeout_seconds"`
	}
	if err := decode(arguments, &in); err != nil {
		return "", err
	}
	if len(in.Argv) == 0 || in.Argv[0] == "" {
		return "", fmt.Errorf("coding: argv[0] is required")
	}
	if in.TimeoutSeconds < 1 || in.TimeoutSeconds > 300 {
		return "", fmt.Errorf("coding: timeout_seconds must be between 1 and 300")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(in.TimeoutSeconds)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, in.Argv[0], in.Argv[1:]...)
	configureProcessGroup(cmd)
	cmd.WaitDelay = time.Second
	cmd.Cancel = func() error { terminateProcessGroup(cmd); return nil }
	cmd.Dir = w.root.Name()
	cmd.Env = append([]string(nil), w.shellEnv...)
	out := &limitedBuffer{limit: w.maxOutputBytes}
	cmd.Stdout, cmd.Stderr = out, out
	err := cmd.Run()
	result := out.String()
	if out.truncated {
		result += "\n[output truncated]"
	}
	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("coding: shell timed out")
	}
	if err != nil {
		return result, fmt.Errorf("coding: shell: %w", err)
	}
	return result, nil
}

type limitedBuffer struct {
	data      strings.Builder
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := b.limit - b.data.Len()
	if n <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > n {
		b.data.Write(p[:n])
		b.truncated = true
		return len(p), nil
	}
	b.data.Write(p)
	return len(p), nil
}
func (b *limitedBuffer) String() string { return b.data.String() }
