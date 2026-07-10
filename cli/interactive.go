// Package cli provides an interactive terminal agent.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"harness/agent"
)

// InteractiveAgent turns terminal lines into user messages. EOF and /exit end
// the conversation.
type InteractiveAgent struct {
	scanner  *bufio.Scanner
	output   io.Writer
	prompted bool
}

func NewInteractiveAgent(input io.Reader, output io.Writer) *InteractiveAgent {
	return &InteractiveAgent{scanner: bufio.NewScanner(input), output: output}
}

func (a *InteractiveAgent) Next(_ context.Context, _ agent.Transcript) (agent.Turn, error) {
	for {
		if a.prompted {
			fmt.Fprintln(a.output)
		}
		fmt.Fprint(a.output, "> ")
		a.prompted = true
		if !a.scanner.Scan() {
			if err := a.scanner.Err(); err != nil {
				return agent.Turn{}, err
			}
			return agent.Turn{Done: true}, nil
		}
		text := strings.TrimSpace(a.scanner.Text())
		if text == "/exit" || text == "/quit" {
			return agent.Turn{Done: true}, nil
		}
		if text != "" {
			return agent.Turn{Messages: []agent.Message{{Role: agent.RoleUser, Content: text}}}, nil
		}
	}
}
