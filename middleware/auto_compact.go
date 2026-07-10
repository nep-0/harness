package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"harness/agent"
)

// Compactor summarizes older conversation messages for use as model context.
type Compactor interface {
	Compact(context.Context, string, agent.Transcript) (string, error)
}

// AutoCompact replaces an old prefix in the model-facing context with a
// summary when its approximate token cost exceeds MaxTokens. The canonical
// transcript is never changed.
type AutoCompact struct {
	MaxTokens         int
	KeepRecent        int
	Compactor         Compactor
	summary           string
	summarizedThrough int
}

func NewAutoCompact(maxTokens, keepRecent int, compactor Compactor) (*AutoCompact, error) {
	if maxTokens < 1 {
		return nil, errors.New("middleware: MaxTokens must be positive")
	}
	if keepRecent < 1 {
		return nil, errors.New("middleware: KeepRecent must be positive")
	}
	if compactor == nil {
		return nil, errors.New("middleware: compactor is required")
	}
	return &AutoCompact{MaxTokens: maxTokens, KeepRecent: keepRecent, Compactor: compactor}, nil
}

func (m *AutoCompact) Context(ctx context.Context, transcript agent.Transcript) (agent.Transcript, error) {
	if m.summarizedThrough > len(transcript) {
		m.summary = ""
		m.summarizedThrough = 0
	}
	view := m.context(transcript)
	if approximateTokens(view) <= m.MaxTokens || len(transcript) <= m.KeepRecent {
		return view, nil
	}
	cut := len(transcript) - m.KeepRecent
	// Never leave a tool result separated from its assistant tool-call message.
	for cut > 0 && transcript[cut].Role == agent.RoleTool {
		cut--
	}
	if cut <= m.summarizedThrough {
		return view, nil
	}
	summary, err := m.Compactor.Compact(ctx, m.summary, transcript[m.summarizedThrough:cut])
	if err != nil {
		return nil, err
	}
	if summary == "" {
		return nil, errors.New("middleware: compactor returned an empty summary")
	}
	m.summary, m.summarizedThrough = summary, cut
	return m.context(transcript), nil
}
func (m *AutoCompact) context(transcript agent.Transcript) agent.Transcript {
	if m.summary == "" {
		return transcript
	}
	context := make(agent.Transcript, 0, 1+len(transcript)-m.summarizedThrough)
	context = append(context, agent.Message{Role: agent.RoleDeveloper, Content: fmt.Sprintf("Conversation summary of earlier messages:\n%s", m.summary)})
	return append(context, transcript[m.summarizedThrough:]...)
}
func (*AutoCompact) ID() string { return "auto_compact" }
func (m *AutoCompact) MarshalState() (json.RawMessage, error) {
	return json.Marshal(struct {
		Summary string `json:"summary"`
		Through int    `json:"through"`
	}{m.summary, m.summarizedThrough})
}
func (m *AutoCompact) UnmarshalState(data json.RawMessage) error {
	if len(data) == 0 {
		return nil
	}
	var state struct {
		Summary string `json:"summary"`
		Through int    `json:"through"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	m.summary, m.summarizedThrough = state.Summary, state.Through
	return nil
}

func approximateTokens(transcript agent.Transcript) int {
	tokens := 0
	for _, message := range transcript {
		tokens += 4 + (len(message.Content)+3)/4
		for _, call := range message.ToolCalls {
			tokens += 4 + (len(call.Name)+len(call.Arguments)+3)/4
		}
	}
	return tokens
}
