// Package middleware contains reusable agent wrappers.
package middleware

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/nep-0/harness/agent"
)

// SlidingWindow retains leading instructions and the newest complete logical
// turns in model-facing context. The runner retains the full canonical transcript.
type SlidingWindow struct {
	MaxTurns int
}

func (*SlidingWindow) ID() string                             { return "sliding_window" }
func (*SlidingWindow) MarshalState() (json.RawMessage, error) { return nil, nil }
func (*SlidingWindow) UnmarshalState(json.RawMessage) error   { return nil }

// NewSlidingWindow creates context middleware that bounds model-visible turns.
func NewSlidingWindow(maxTurns int) (*SlidingWindow, error) {
	if maxTurns < 1 {
		return nil, errors.New("middleware: MaxTurns must be positive")
	}
	return &SlidingWindow{MaxTurns: maxTurns}, nil
}

func (m *SlidingWindow) Context(_ context.Context, transcript agent.Transcript) (agent.Transcript, error) {
	leadingEnd := 0
	for leadingEnd < len(transcript) && (transcript[leadingEnd].Role == agent.RoleSystem || transcript[leadingEnd].Role == agent.RoleDeveloper) {
		leadingEnd++
	}
	leading := append(agent.Transcript(nil), transcript[:leadingEnd]...)
	turns := splitTurns(transcript[leadingEnd:])
	selected := make([]agent.Transcript, 0, m.MaxTurns)
	for index := len(turns) - 1; index >= 0 && len(selected) < m.MaxTurns; index-- {
		if completeTurn(turns[index]) {
			selected = append(selected, turns[index])
		}
	}
	window := leading
	for index := len(selected) - 1; index >= 0; index-- {
		window = append(window, selected[index]...)
	}
	return window, nil
}

func splitTurns(messages agent.Transcript) []agent.Transcript {
	var turns []agent.Transcript
	var current agent.Transcript
	for _, message := range messages {
		if message.Role == agent.RoleUser || message.Role == agent.RoleSystem || message.Role == agent.RoleDeveloper {
			if len(current) > 0 {
				turns = append(turns, current)
			}
			current = nil
		}
		current = append(current, message)
	}
	if len(current) > 0 {
		turns = append(turns, current)
	}
	return turns
}

func completeTurn(turn agent.Transcript) bool {
	pending := map[string]struct{}{}
	for _, message := range turn {
		if message.Role == agent.RoleAssistant {
			for _, call := range message.ToolCalls {
				pending[call.ID] = struct{}{}
			}
		}
		if message.Role == agent.RoleTool {
			delete(pending, message.ToolCallID)
		}
	}
	return len(pending) == 0
}
