package middleware

import (
	"context"
	"encoding/json"

	"github.com/nep-0/harness/agent"
)

// MetadataProvider returns JSON-serializable runtime facts for one model request.
type MetadataProvider func(context.Context) (map[string]any, error)

// RuntimeMetadata injects stable facts near the cacheable prompt prefix and
// volatile facts at the context suffix. Neither is stored in the transcript.
type RuntimeMetadata struct {
	Stable   MetadataProvider
	Volatile MetadataProvider
}

func NewRuntimeMetadata(stable, volatile MetadataProvider) *RuntimeMetadata {
	return &RuntimeMetadata{Stable: stable, Volatile: volatile}
}

func (*RuntimeMetadata) ID() string                             { return "runtime_metadata" }
func (*RuntimeMetadata) MarshalState() (json.RawMessage, error) { return nil, nil }
func (*RuntimeMetadata) UnmarshalState(json.RawMessage) error   { return nil }

func (m *RuntimeMetadata) Context(ctx context.Context, transcript agent.Transcript) (agent.Transcript, error) {
	stable, err := metadataMessage(ctx, m.Stable, "Stable runtime metadata")
	if err != nil {
		return nil, err
	}
	volatile, err := metadataMessage(ctx, m.Volatile, "Volatile runtime metadata")
	if err != nil {
		return nil, err
	}
	context := append(agent.Transcript(nil), transcript...)
	if stable.Content != "" {
		index := 0
		for index < len(context) && (context[index].Role == agent.RoleSystem || context[index].Role == agent.RoleDeveloper) {
			index++
		}
		context = append(context, agent.Message{})
		copy(context[index+1:], context[index:])
		context[index] = stable
	}
	if volatile.Content != "" {
		context = append(context, volatile)
	}
	return context, nil
}

func metadataMessage(ctx context.Context, provider MetadataProvider, label string) (agent.Message, error) {
	if provider == nil {
		return agent.Message{}, nil
	}
	fields, err := provider(ctx)
	if err != nil {
		return agent.Message{}, err
	}
	if len(fields) == 0 {
		return agent.Message{}, nil
	}
	content, err := json.Marshal(fields)
	if err != nil {
		return agent.Message{}, err
	}
	return agent.Message{Role: agent.RoleDeveloper, Content: label + ":\n" + string(content)}, nil
}
