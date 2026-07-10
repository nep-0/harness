package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nep-0/harness/agent"
)

func TestInteractiveAgentReadsMessagesAndExit(t *testing.T) {
	var output bytes.Buffer
	interactive := NewInteractiveAgent(strings.NewReader("\nhello\n/exit\n"), &output)
	turn, err := interactive.Next(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := turn.Messages; len(got) != 1 || got[0].Role != agent.RoleUser || got[0].Content != "hello" {
		t.Fatalf("messages = %#v", got)
	}
	turn, err = interactive.Next(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !turn.Done {
		t.Fatal("expected /exit to end the session")
	}
}
