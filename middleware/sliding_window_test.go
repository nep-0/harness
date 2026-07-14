package middleware

import (
	"context"
	"testing"

	"github.com/nep-0/harness/agent"
)

func TestSlidingWindowRetainsInstructionsAndNewestCompleteTurns(t *testing.T) {
	window, err := NewSlidingWindow(2)
	if err != nil {
		t.Fatal(err)
	}
	context, err := window.Context(context.Background(), agent.Transcript{
		{Role: agent.RoleSystem, Content: "system"}, {Role: agent.RoleDeveloper, Content: "developer"},
		{Role: agent.RoleUser, Content: "one"}, {Role: agent.RoleAssistant, Content: "answer one"},
		{Role: agent.RoleUser, Content: "two"}, {Role: agent.RoleAssistant, Content: "answer two"},
		{Role: agent.RoleUser, Content: "three"}, {Role: agent.RoleAssistant, Content: "answer three"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(context) != 6 || context[0].Content != "system" || context[2].Content != "two" || context[4].Content != "three" {
		t.Fatalf("context %#v", context)
	}
}

func TestSlidingWindowOmitsIncompleteToolTurn(t *testing.T) {
	window, err := NewSlidingWindow(2)
	if err != nil {
		t.Fatal(err)
	}
	context, err := window.Context(context.Background(), agent.Transcript{
		{Role: agent.RoleUser, Content: "complete"}, {Role: agent.RoleAssistant, Content: "done"},
		{Role: agent.RoleUser, Content: "incomplete"}, {Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "call", Name: "tool"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(context) != 2 || context[0].Content != "complete" {
		t.Fatalf("context %#v", context)
	}
}
