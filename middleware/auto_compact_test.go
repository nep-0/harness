package middleware

import (
	"context"
	"testing"

	"github.com/nep-0/harness/agent"
)

type fixedCompactor struct{}

func (fixedCompactor) Compact(_ context.Context, _ string, _ agent.Transcript) (string, error) {
	return "summary", nil
}

func TestAutoCompactPreservesCanonicalTail(t *testing.T) {
	middleware, err := NewAutoCompact(5, 1, fixedCompactor{})
	if err != nil {
		t.Fatal(err)
	}
	original := agent.Transcript{{Role: agent.RoleUser, Content: "a long first message"}, {Role: agent.RoleAssistant, Content: "a long second message"}}
	context, err := middleware.Context(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	if len(context) != 2 || context[0].Role != agent.RoleDeveloper || context[1].Content != original[1].Content {
		t.Fatalf("context = %#v", context)
	}
	if len(original) != 2 || original[0].Content != "a long first message" {
		t.Fatalf("canonical transcript changed: %#v", original)
	}
}
