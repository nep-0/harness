// Package agent provides an agent-controlled OpenAI Chat Completions runner.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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
	ID, Name string
	// Arguments is untrusted model output. It is validated before a handler runs.
	Arguments string
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

// Agent is an optional autonomous policy used with Drive.
type Agent interface {
	Decide(context.Context, RunSnapshot) (Action, error)
}
type Action struct {
	Messages []Message
	Done     bool
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

type runnerConfig struct {
	APIKey, Model, BaseURL string
	MaxTurns               int
	Tools                  []Tool
	OnEvent                func(Event) error
	Middlewares            []ContextMiddleware
	HTTPClient             *http.Client
}

// RunnerOption configures a Runner at construction time.
type RunnerOption func(*runnerConfig)

func WithAPIKey(value string) RunnerOption {
	return func(config *runnerConfig) { config.APIKey = value }
}
func WithModel(value string) RunnerOption { return func(config *runnerConfig) { config.Model = value } }
func WithBaseURL(value string) RunnerOption {
	return func(config *runnerConfig) { config.BaseURL = value }
}

// WithHTTPClient sets the HTTP client used for OpenAI API requests.
func WithHTTPClient(client *http.Client) RunnerOption {
	return func(config *runnerConfig) { config.HTTPClient = client }
}
func WithMaxTurns(value int) RunnerOption {
	return func(config *runnerConfig) { config.MaxTurns = value }
}
func WithTool(tool Tool) RunnerOption {
	return func(config *runnerConfig) { config.Tools = append(config.Tools, tool) }
}
func WithMiddleware(middleware ContextMiddleware) RunnerOption {
	return func(config *runnerConfig) { config.Middlewares = append(config.Middlewares, middleware) }
}
func WithEventHandler(handler func(Event) error) RunnerOption {
	return func(config *runnerConfig) { config.OnEvent = handler }
}

// ContextMiddleware derives the transcript sent to the model. It must not
// mutate its input; the runner preserves the canonical transcript separately.
type ContextMiddleware interface {
	ID() string
	Context(context.Context, Transcript) (Transcript, error)
	MarshalState() (json.RawMessage, error)
	UnmarshalState(json.RawMessage) error
}

// RunOption configures one RunTurn invocation.
type RunOption func(*runOptions)

type runOptions struct {
	checkpoint func(context.Context, RunSnapshot) error
}
type RunSnapshot struct {
	Transcript      Transcript
	MiddlewareState map[string]json.RawMessage
}

// WithCheckpoint is called after each durable transcript change.
func WithCheckpoint(checkpoint func(context.Context, RunSnapshot) error) RunOption {
	return func(options *runOptions) { options.checkpoint = checkpoint }
}
