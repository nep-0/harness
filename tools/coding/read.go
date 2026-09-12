package coding

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/nep-0/harness/agent"
)

func (w *Workspace) ReadFileTool() agent.Tool {
	return agent.Tool{Name: "read_file", Description: "Read a UTF-8 text file inside the workspace. Optional line bounds are inclusive.", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"start_line":{"type":"integer","minimum":1},"end_line":{"type":"integer","minimum":1}},"required":["path"],"additionalProperties":false}`), Handler: w.readFile}
}
func (w *Workspace) readFile(ctx context.Context, arguments json.RawMessage) (string, error) {
	var input struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := decode(arguments, &input); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	name, err := cleanPath(input.Path)
	if err != nil {
		return "", err
	}
	if input.EndLine > 0 && (input.StartLine == 0 || input.EndLine < input.StartLine) {
		return "", fmt.Errorf("coding: invalid line range")
	}
	file, err := w.root.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("coding: %q is a directory", input.Path)
	}
	data, err := readBounded(file, w.maxReadBytes)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("coding: %q is not UTF-8 text", input.Path)
	}
	return numberedLines(string(data), input.StartLine, input.EndLine), nil
}
func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("coding: file exceeds %d-byte limit", limit)
	}
	return data, nil
}
func numberedLines(content string, start, end int) string {
	if start == 0 {
		start = 1
	}
	var out strings.Builder
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	for i, line := range lines {
		n := i + 1
		if n >= start && (end == 0 || n <= end) {
			fmt.Fprintf(&out, "%d:%s\n", n, line)
		}
	}
	return out.String()
}

func (w *Workspace) ListDirTool() agent.Tool {
	return agent.Tool{Name: "list_dir", Description: "List direct children of a workspace directory in deterministic order. Output is bounded.", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`), Handler: w.listDir}
}
func (w *Workspace) listDir(ctx context.Context, arguments json.RawMessage) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := decode(arguments, &input); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	name, err := cleanPath(input.Path)
	if err != nil {
		return "", err
	}
	entries, err := fs.ReadDir(w.root.FS(), name)
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	out := &limitedBuffer{limit: w.maxOutputBytes}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		line := entry.Name() + "\n"
		if entry.IsDir() {
			line = entry.Name() + "/\n"
		}
		out.Write([]byte(line))
		if out.truncated {
			return out.String() + "[output truncated]\n", nil
		}
	}
	return out.String(), nil
}

func (w *Workspace) GrepFilesTool() agent.Tool {
	return agent.Tool{Name: "grep_files", Description: "Find literal text in UTF-8 files below a workspace path. Reads, traversal, and results are bounded.", Parameters: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","minLength":1},"path":{"type":"string"},"max_results":{"type":"integer","minimum":1}},"required":["pattern","path"],"additionalProperties":false}`), Handler: w.grepFiles}
}
func (w *Workspace) grepFiles(ctx context.Context, arguments json.RawMessage) (string, error) {
	var input struct {
		Pattern    string `json:"pattern"`
		Path       string `json:"path"`
		MaxResults int    `json:"max_results"`
	}
	if err := decode(arguments, &input); err != nil {
		return "", err
	}
	if input.Pattern == "" {
		return "", fmt.Errorf("coding: pattern is required")
	}
	name, err := cleanPath(input.Path)
	if err != nil {
		return "", err
	}
	limit := w.maxMatches
	if input.MaxResults > 0 && input.MaxResults < limit {
		limit = input.MaxResults
	}
	var matches []string
	stop := errors.New("result limit")
	err = fs.WalkDir(w.root.FS(), name, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(matches) == limit {
			return stop
		}
		if entry.IsDir() {
			return nil
		}
		f, err := w.root.Open(file)
		if err != nil {
			return nil
		}
		data, rerr := readBounded(f, w.maxReadBytes)
		f.Close()
		if rerr != nil || !utf8.Valid(data) {
			return nil
		}
		r := bufio.NewReader(strings.NewReader(string(data)))
		for n := 1; ; n++ {
			line, e := r.ReadString('\n')
			if len(line) > bufio.MaxScanTokenSize {
				return fmt.Errorf("coding: line in %q exceeds %d bytes", file, bufio.MaxScanTokenSize)
			}
			line = strings.TrimSuffix(line, "\n")
			if strings.Contains(line, input.Pattern) {
				matches = append(matches, fmt.Sprintf("%s:%d:%s", path.Clean(file), n, line))
			}
			if len(matches) == limit {
				return stop
			}
			if e == io.EOF {
				return nil
			}
			if e != nil {
				return e
			}
		}
	})
	if err != nil && !errors.Is(err, stop) {
		return "", err
	}
	return strings.Join(matches, "\n"), nil
}
