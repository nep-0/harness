package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sync"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Runner executes an Agent against streamed Chat Completions.
type Runner struct {
	config  runnerConfig
	client  openai.Client
	tools   map[string]Tool
	runMu   sync.Mutex
	eventMu sync.Mutex
}

func NewRunner(options ...RunnerOption) (*Runner, error) {
	config := runnerConfig{}
	for _, option := range options {
		option(&config)
	}
	if config.APIKey == "" && config.BaseURL == "" {
		return nil, errors.New("agent: APIKey is required")
	}
	if config.Model == "" {
		return nil, errors.New("agent: Model is required")
	}
	if config.MaxTurns < 0 {
		return nil, errors.New("agent: MaxTurns must not be negative")
	}
	r := &Runner{config: config, tools: make(map[string]Tool, len(config.Tools))}
	for _, tool := range config.Tools {
		if !toolNamePattern.MatchString(tool.Name) {
			return nil, fmt.Errorf("agent: invalid tool name %q", tool.Name)
		}
		if tool.Handler == nil {
			return nil, fmt.Errorf("agent: tool %q has no handler", tool.Name)
		}
		if _, ok := r.tools[tool.Name]; ok {
			return nil, fmt.Errorf("agent: duplicate tool %q", tool.Name)
		}
		if len(tool.Parameters) > 0 {
			var schema map[string]any
			if json.Unmarshal(tool.Parameters, &schema) != nil || schema == nil {
				return nil, fmt.Errorf("agent: tool %q parameters must be a JSON object", tool.Name)
			}
		}
		r.tools[tool.Name] = tool
	}
	opts := []option.RequestOption{option.WithAPIKey(config.APIKey)}
	if config.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(config.BaseURL))
	}
	r.client = openai.NewClient(opts...)
	return r, nil
}

// RunTurn returns the updated snapshot, or the partial snapshot when it fails.
// It never waits for external input. A Runner serializes calls because its
// middleware state belongs to one active conversation at a time.
func (r *Runner) RunTurn(ctx context.Context, snapshot RunSnapshot, messages []Message, options ...RunOption) (RunSnapshot, error) {
	r.runMu.Lock()
	defer r.runMu.Unlock()
	settings := runOptions{}
	for _, option := range options {
		option(&settings)
	}
	for _, middleware := range r.config.Middlewares {
		if err := middleware.UnmarshalState(snapshot.MiddlewareState[middleware.ID()]); err != nil {
			return snapshot, err
		}
	}
	transcript := snapshot.Transcript
	checkpoint := func() error {
		if settings.checkpoint == nil {
			return nil
		}
		return settings.checkpoint(ctx, r.snapshot(transcript))
	}
	turns := 0
	for {
		if err := validateAgentMessages(messages); err != nil {
			return r.snapshot(transcript), err
		}
		transcript = append(transcript, messages...)
		if err := checkpoint(); err != nil {
			return r.snapshot(transcript), err
		}
		for {
			if r.config.MaxTurns > 0 && turns >= r.config.MaxTurns {
				return r.snapshot(transcript), ErrMaxTurns
			}
			turns++
			assistant, err := r.complete(ctx, transcript)
			if err != nil {
				return r.snapshot(transcript), err
			}
			if err := checkpoint(); err != nil {
				return r.snapshot(transcript), err
			}
			transcript = append(transcript, assistant)
			if err := checkpoint(); err != nil {
				return r.snapshot(transcript), err
			}
			if len(assistant.ToolCalls) == 0 {
				return r.snapshot(transcript), nil
			}
			results, err := r.executeTools(ctx, assistant.ToolCalls)
			if err != nil {
				return r.snapshot(transcript), err
			}
			transcript = append(transcript, results...)
			if err := checkpoint(); err != nil {
				return r.snapshot(transcript), err
			}
		}
	}
}

func (r *Runner) complete(ctx context.Context, transcript Transcript) (Message, error) {
	if err := r.emit(Event{Type: EventCompletionStarted}); err != nil {
		return Message{}, err
	}
	contextTranscript := cloneTranscript(transcript)
	var err error
	for _, middleware := range r.config.Middlewares {
		contextTranscript, err = middleware.Context(ctx, cloneTranscript(contextTranscript))
		if err != nil {
			return Message{}, err
		}
	}
	params, err := r.params(contextTranscript)
	if err != nil {
		return Message{}, err
	}
	stream := r.client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()
	var acc openai.ChatCompletionAccumulator
	var calls streamedToolCalls
	for stream.Next() {
		chunk := stream.Current()
		if !acc.AddChunk(chunk) {
			return Message{}, errors.New("agent: invalid streamed completion sequence")
		}
		for _, choice := range chunk.Choices {
			if choice.Index == 0 {
				calls.Add(choice.Delta.ToolCalls)
			}
			if choice.Index == 0 && choice.Delta.Content != "" {
				if err := r.emit(Event{Type: EventTextDelta, Delta: choice.Delta.Content}); err != nil {
					return Message{}, err
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return Message{}, err
	}
	if len(acc.Choices) == 0 {
		return Message{}, errors.New("agent: completion returned no choices")
	}
	message := Message{Role: RoleAssistant, Content: acc.Choices[0].Message.Content}
	message.ToolCalls = calls.Calls()
	if err := r.emit(Event{Type: EventCompletionFinished, Message: message}); err != nil {
		return Message{}, err
	}
	return message, nil
}

// streamedToolCalls keeps logically separate calls apart even when an upstream
// stream reuses a tool-call index for parallel calls.
type streamedToolCalls struct{ slots map[int][]ToolCall }

func (s *streamedToolCalls) Add(deltas []openai.ChatCompletionChunkChoiceDeltaToolCall) {
	if s.slots == nil {
		s.slots = map[int][]ToolCall{}
	}
	for _, delta := range deltas {
		index := int(delta.Index)
		calls := s.slots[index]
		if delta.Function.Name != "" && (len(calls) == 0 || calls[len(calls)-1].Name != "") {
			calls = append(calls, ToolCall{ID: delta.ID, Name: delta.Function.Name, Arguments: delta.Function.Arguments})
		} else if len(calls) > 0 {
			current := &calls[len(calls)-1]
			if delta.ID != "" {
				current.ID = delta.ID
			}
			current.Name += delta.Function.Name
			current.Arguments += delta.Function.Arguments
		}
		s.slots[index] = calls
	}
}
func (s *streamedToolCalls) Calls() []ToolCall {
	var calls []ToolCall
	for index := 0; index < len(s.slots); index++ {
		calls = append(calls, s.slots[index]...)
	}
	return calls
}

func (r *Runner) executeTools(ctx context.Context, calls []ToolCall) ([]Message, error) {
	results := make([]Message, len(calls))
	var wg sync.WaitGroup
	var once sync.Once
	var callbackErr error
	for i, call := range calls {
		wg.Add(1)
		go func(i int, call ToolCall) {
			defer wg.Done()
			if err := r.emit(Event{Type: EventToolStarted, ToolCall: call}); err != nil {
				once.Do(func() { callbackErr = err })
				return
			}
			results[i] = Message{Role: RoleTool, Content: r.callTool(ctx, call), ToolCallID: call.ID}
			if err := r.emit(Event{Type: EventToolFinished, ToolCall: call, Message: results[i]}); err != nil {
				once.Do(func() { callbackErr = err })
			}
		}(i, call)
	}
	wg.Wait()
	return results, callbackErr
}

func (r *Runner) callTool(ctx context.Context, call ToolCall) (content string) {
	tool, ok := r.tools[call.Name]
	if !ok {
		return fmt.Sprintf("tool %q is not registered", call.Name)
	}
	var arguments any
	if call.Arguments == "" || json.Unmarshal([]byte(call.Arguments), &arguments) != nil {
		return fmt.Sprintf("tool %q received invalid JSON arguments", call.Name)
	}
	defer func() {
		if recover() != nil {
			content = fmt.Sprintf("tool %q failed", call.Name)
		}
	}()
	content, err := tool.Handler(ctx, json.RawMessage(call.Arguments))
	if err != nil {
		return fmt.Sprintf("tool %q failed", call.Name)
	}
	return content
}

func (r *Runner) params(transcript Transcript) (openai.ChatCompletionNewParams, error) {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(transcript))
	for _, message := range transcript {
		converted, err := toOpenAIMessage(message)
		if err != nil {
			return openai.ChatCompletionNewParams{}, err
		}
		messages = append(messages, converted)
	}
	params := openai.ChatCompletionNewParams{Messages: messages, Model: shared.ChatModel(r.config.Model)}
	for _, tool := range r.config.Tools {
		function := shared.FunctionDefinitionParam{Name: tool.Name}
		if tool.Description != "" {
			function.Description = param.NewOpt(tool.Description)
		}
		if len(tool.Parameters) > 0 {
			var parameters shared.FunctionParameters
			_ = json.Unmarshal(tool.Parameters, &parameters)
			function.Parameters = parameters
		}
		params.Tools = append(params.Tools, openai.ChatCompletionFunctionTool(function))
	}
	return params, nil
}

func toOpenAIMessage(message Message) (openai.ChatCompletionMessageParamUnion, error) {
	switch message.Role {
	case RoleSystem:
		return openai.SystemMessage(message.Content), nil
	case RoleDeveloper:
		return openai.DeveloperMessage(message.Content), nil
	case RoleUser:
		return openai.UserMessage(message.Content), nil
	case RoleTool:
		if message.ToolCallID == "" {
			return openai.ChatCompletionMessageParamUnion{}, errors.New("agent: tool message is missing ToolCallID")
		}
		return openai.ToolMessage(message.Content, message.ToolCallID), nil
	case RoleAssistant:
		converted := openai.AssistantMessage(message.Content)
		for _, call := range message.ToolCalls {
			converted.OfAssistant.ToolCalls = append(converted.OfAssistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{ID: call.ID, Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{Name: call.Name, Arguments: call.Arguments}}})
		}
		return converted, nil
	default:
		return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("agent: unsupported message role %q", message.Role)
	}
}
func validateAgentMessages(messages []Message) error {
	for _, message := range messages {
		if message.Role != RoleSystem && message.Role != RoleDeveloper && message.Role != RoleUser {
			return fmt.Errorf("agent: agents may only produce system, developer, or user messages (got %q)", message.Role)
		}
	}
	return nil
}
func (r *Runner) emit(event Event) error {
	if r.config.OnEvent == nil {
		return nil
	}
	r.eventMu.Lock()
	defer r.eventMu.Unlock()
	return r.config.OnEvent(event)
}

func cloneTranscript(transcript Transcript) Transcript {
	cloned := make(Transcript, len(transcript))
	for i, message := range transcript {
		cloned[i] = message
		cloned[i].ToolCalls = append([]ToolCall(nil), message.ToolCalls...)
	}
	return cloned
}
func cloneSnapshot(snapshot RunSnapshot) RunSnapshot {
	state := map[string]json.RawMessage{}
	for k, v := range snapshot.MiddlewareState {
		state[k] = append(json.RawMessage(nil), v...)
	}
	return RunSnapshot{Transcript: cloneTranscript(snapshot.Transcript), MiddlewareState: state}
}
func (r *Runner) snapshot(transcript Transcript) RunSnapshot {
	state := map[string]json.RawMessage{}
	for _, m := range r.config.Middlewares {
		if value, err := m.MarshalState(); err == nil {
			state[m.ID()] = value
		}
	}
	return RunSnapshot{Transcript: cloneTranscript(transcript), MiddlewareState: state}
}
