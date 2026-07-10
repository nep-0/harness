// Package agent provides an agent-controlled OpenAI Chat Completions runner.
package agent

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrMaxTurns is returned when a configured completion-request limit is reached.
var ErrMaxTurns = errors.New("agent: maximum completion turns reached")

type Role string

const (
	RoleSystem    Role = "system"
	RoleDeveloper Role = "developer"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ToolCall struct {
	ID, Name  string
	Arguments json.RawMessage
}
type Message struct {
	Role       Role
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}
type Transcript []Message
type Turn struct {
	Messages []Message
	Done     bool
}

// Agent supplies messages for each application-controlled turn.
type Agent interface {
	Next(context.Context, Transcript) (Turn, error)
}
type ToolHandler func(context.Context, json.RawMessage) (string, error)
type Tool struct {
	Name, Description string
	Parameters        json.RawMessage
	Handler           ToolHandler
}

type EventType string

const (
	EventCompletionStarted  EventType = "completion_started"
	EventTextDelta          EventType = "text_delta"
	EventCompletionFinished EventType = "completion_finished"
	EventToolStarted        EventType = "tool_started"
	EventToolFinished       EventType = "tool_finished"
)

type Event struct {
	Type     EventType
	Delta    string
	Message  Message
	ToolCall ToolCall
}

// Config configures a Runner. APIKey and Model are required. MaxTurns of zero
// permits unlimited completion requests.
type Config struct {
	APIKey, Model, BaseURL string
	MaxTurns               int
	Tools                  []Tool
	OnEvent                func(Event) error
	Middlewares            []ContextMiddleware
}

// ContextMiddleware derives the transcript sent to the model. It must not
// mutate its input; the runner preserves the canonical transcript separately.
type ContextMiddleware interface {
	ID() string
	Context(context.Context, Transcript) (Transcript, error)
	MarshalState() (json.RawMessage, error)
	UnmarshalState(json.RawMessage) error
}

// RunOption configures one invocation of Run.
type RunOption func(*runOptions)

type runOptions struct {
	snapshot   RunSnapshot
	checkpoint func(context.Context, RunSnapshot) error
}
type RunSnapshot struct {
	Transcript      Transcript
	MiddlewareState map[string]json.RawMessage
}

// WithTranscript resumes a run from a previously saved canonical transcript.
func WithTranscript(transcript Transcript) RunOption {
	return func(options *runOptions) { options.snapshot.Transcript = cloneTranscript(transcript) }
}
func WithSnapshot(snapshot RunSnapshot) RunOption {
	return func(options *runOptions) { options.snapshot = cloneSnapshot(snapshot) }
}

// WithCheckpoint is called after each durable transcript change.
func WithCheckpoint(checkpoint func(context.Context, RunSnapshot) error) RunOption {
	return func(options *runOptions) { options.checkpoint = checkpoint }
}
