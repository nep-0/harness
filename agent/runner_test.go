package agent

import (
	"testing"

	"github.com/openai/openai-go"
)

func TestNewRunnerValidatesConfig(t *testing.T) {
	if _, err := NewRunner(Config{}); err == nil {
		t.Fatal("expected missing API key error")
	}
	if _, err := NewRunner(Config{APIKey: "key"}); err == nil {
		t.Fatal("expected missing model error")
	}
}

func TestStreamedToolCallsSplitsReusedParallelIndex(t *testing.T) {
	var collector streamedToolCalls
	collector.Add([]openai.ChatCompletionChunkChoiceDeltaToolCall{
		{Index: 0, ID: "one", Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{Name: "weather", Arguments: `{"location":"Nanjing"}`}},
		{Index: 0, ID: "two", Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{Name: "weather", Arguments: `{"location":"Chengdu"}`}},
		{Index: 0, ID: "three", Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{Name: "weather", Arguments: `{"location":"Xiamen"}`}},
	})
	calls := collector.Calls()
	if len(calls) != 3 || calls[0].ID != "one" || calls[1].Arguments != `{"location":"Chengdu"}` || calls[2].Name != "weather" {
		t.Fatalf("calls = %#v", calls)
	}
}
