// Package middleware contains reusable agent wrappers.
package middleware

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/nep-0/harness/agent"
)

// SlidingWindow retains only the newest MaxMessages in model-facing context.
// The runner retains the full canonical transcript.
type SlidingWindow struct {
	MaxMessages int
}

func (*SlidingWindow) ID() string                             { return "sliding_window" }
func (*SlidingWindow) MarshalState() (json.RawMessage, error) { return nil, nil }
func (*SlidingWindow) UnmarshalState(json.RawMessage) error   { return nil }

// NewSlidingWindow creates context middleware that bounds model-visible history.
func NewSlidingWindow(maxMessages int) (*SlidingWindow, error) {
	if maxMessages < 1 {
		return nil, errors.New("middleware: MaxMessages must be positive")
	}
	return &SlidingWindow{MaxMessages: maxMessages}, nil
}

func (m *SlidingWindow) Context(_ context.Context, transcript agent.Transcript) (agent.Transcript, error) {
	start := len(transcript) - m.MaxMessages
	if start < 0 {
		start = 0
	}
	window := append(agent.Transcript(nil), transcript[start:]...)
	return window, nil
}
