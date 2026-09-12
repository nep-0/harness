// Package coding provides bounded workspace tools for coding agents.
package coding

import (
	"encoding/json"
	"fmt"
	"github.com/nep-0/harness/agent"
	"os"
	"path"
	"strings"
	"sync"
)

const (
	defaultMaxReadBytes = 64 << 10
	defaultMaxMatches   = 100
	defaultMaxOutput    = 64 << 10
)

// Workspace confines filesystem tools to Root. Shell is deliberately not
// confined: it is for trusted use and can access the host filesystem and network.
type Workspace struct {
	root           *os.Root
	maxReadBytes   int64
	maxMatches     int
	maxOutputBytes int
	shellEnv       []string
	patchMu        sync.Mutex
}

// Config configures a workspace. Zero limits use bounded defaults.
type Config struct {
	Root           string
	MaxReadBytes   int64
	MaxMatches     int
	MaxOutputBytes int
	// ShellEnv is the complete environment supplied to the trusted shell tool.
	// It defaults to PATH only. It does not sandbox commands.
	ShellEnv []string
}

// New opens Root as the confinement root for filesystem tools.
func New(config Config) (*Workspace, error) {
	if config.Root == "" {
		return nil, fmt.Errorf("coding: Root is required")
	}
	root, err := os.OpenRoot(config.Root)
	if err != nil {
		return nil, err
	}
	maxReadBytes := config.MaxReadBytes
	if maxReadBytes == 0 {
		maxReadBytes = defaultMaxReadBytes
	}
	maxMatches := config.MaxMatches
	if maxMatches == 0 {
		maxMatches = defaultMaxMatches
	}
	maxOutputBytes := config.MaxOutputBytes
	if maxOutputBytes == 0 {
		maxOutputBytes = defaultMaxOutput
	}
	if maxReadBytes < 1 || maxMatches < 1 || maxOutputBytes < 1 {
		root.Close()
		return nil, fmt.Errorf("coding: limits must be positive")
	}
	env := config.ShellEnv
	if env == nil {
		env = []string{"PATH=" + os.Getenv("PATH")}
	}
	return &Workspace{root: root, maxReadBytes: maxReadBytes, maxMatches: maxMatches, maxOutputBytes: maxOutputBytes, shellEnv: append([]string(nil), env...)}, nil
}

// Close releases the directory handle backing filesystem confinement.
func (w *Workspace) Close() error { return w.root.Close() }

// Tools returns the standard coding-agent tool set.
func (w *Workspace) Tools() []agent.Tool {
	return []agent.Tool{w.ReadFileTool(), w.ListDirTool(), w.GrepFilesTool(), w.ApplyPatchTool(), w.ShellTool()}
}

func decode(arguments json.RawMessage, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(arguments)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return fmt.Errorf("coding: invalid JSON arguments")
	}
	return nil
}

func cleanPath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("coding: path is required")
	}
	if strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("coding: path must be relative")
	}
	name = path.Clean(name)
	if name == ".." || strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("coding: path escapes workspace")
	}
	return name, nil
}
