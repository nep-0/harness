package middleware

import (
	"context"
	"testing"

	"github.com/nep-0/harness/agent"
)

func TestSlidingWindowReturnsNewestMessages(t *testing.T) {
	window, err := NewSlidingWindow(2)
	if err != nil {
		t.Fatal(err)
	}
	context, err := window.Context(context.Background(), agent.Transcript{{Content: "one"}, {Content: "two"}, {Content: "three"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(context) != 2 || context[0].Content != "two" || context[1].Content != "three" {
		t.Fatalf("context %#v", context)
	}
}
