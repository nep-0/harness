package middleware

import (
	"context"
	"errors"

	"github.com/nep-0/harness/agent"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

// OpenAICompactor summarizes history through Chat Completions.
type OpenAICompactor struct {
	client openai.Client
	model  string
}

func NewOpenAICompactor(apiKey, model, baseURL string) (*OpenAICompactor, error) {
	if apiKey == "" {
		return nil, errors.New("middleware: API key is required")
	}
	if model == "" {
		return nil, errors.New("middleware: model is required")
	}
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return &OpenAICompactor{client: openai.NewClient(opts...), model: model}, nil
}

func (c *OpenAICompactor) Compact(ctx context.Context, previousSummary string, transcript agent.Transcript) (string, error) {
	messages := []openai.ChatCompletionMessageParamUnion{openai.DeveloperMessage("Summarize the conversation for a future assistant turn. Preserve user goals, decisions, important facts, unresolved work, and tool results. Be concise.")}
	if previousSummary != "" {
		messages = append(messages, openai.DeveloperMessage("Existing summary to preserve and update:\n"+previousSummary))
	}
	for _, message := range transcript {
		switch message.Role {
		case agent.RoleSystem:
			messages = append(messages, openai.SystemMessage(message.Content))
		case agent.RoleDeveloper:
			messages = append(messages, openai.DeveloperMessage(message.Content))
		case agent.RoleUser:
			messages = append(messages, openai.UserMessage(message.Content))
		case agent.RoleAssistant:
			assistant := openai.AssistantMessage(message.Content)
			for _, call := range message.ToolCalls {
				assistant.OfAssistant.ToolCalls = append(assistant.OfAssistant.ToolCalls, openai.ChatCompletionMessageToolCallParam{ID: call.ID, Function: openai.ChatCompletionMessageToolCallFunctionParam{Name: call.Name, Arguments: call.Arguments}})
			}
			messages = append(messages, assistant)
		case agent.RoleTool:
			messages = append(messages, openai.ToolMessage(message.Content, message.ToolCallID))
		}
	}
	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{Messages: messages, Model: shared.ChatModel(c.model)})
	if err != nil {
		return "", err
	}
	if len(completion.Choices) == 0 {
		return "", errors.New("middleware: compaction returned no choices")
	}
	return completion.Choices[0].Message.Content, nil
}
