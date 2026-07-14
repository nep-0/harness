package agent

import (
	"testing"

	"github.com/openai/openai-go/v3"
)

func TestNewRunnerValidatesConfig(t *testing.T) {
	if _, err := NewRunner(); err == nil {
		t.Fatal("expected missing API key error")
	}
	if _, err := NewRunner(WithAPIKey("key")); err == nil {
		t.Fatal("expected missing model error")
	}
	if _, err := NewRunner(WithBaseURL("http://localhost:8080"), WithModel("local-model")); err != nil {
		t.Fatalf("local endpoint without API key: %v", err)
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
