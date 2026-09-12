package main

import (
	"testing"

	"github.com/nep-0/harness/agent"
	"github.com/nep-0/harness/tools/coding"
)

func TestCodingToolsAppendToRunnerOptions(t *testing.T) {
	workspace, err := coding.New(coding.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()

	options := baseRunnerOptions("key", "model", "", 0)
	for _, tool := range workspace.Tools() {
		options = append(options, agent.WithTool(tool))
	}
	if _, err := agent.NewRunner(options...); err != nil {
		t.Fatal(err)
	}
}
