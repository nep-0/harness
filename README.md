# Harness

Harness is a Go framework for multi-turn agents backed by OpenAI Chat
Completions. Your application implements an agent that supplies messages; the
runner owns the canonical transcript, streams model output, dispatches tools,
and can persist a session for later resumption.

## Install

```bash
go get github.com/nep-0/harness
```

This repository's module path is `github.com/nep-0/harness`.

## Interactive CLI

Set `OPENAI_API_KEY` and start a chat:

```bash
go run ./cmd/harness -model gpt-4.1
```

Type a message and press Enter. Use `/exit`, `/quit`, or EOF to finish.

Resume a persisted session:

```bash
go run ./cmd/harness \
  -model gpt-4.1 \
  -session-dir .sessions \
  -session project-chat
```

Enable automatic context compaction with an approximate context budget:

```bash
go run ./cmd/harness -model gpt-4.1 -compact-tokens 6000
```

Alternatively, retain only a fixed number of recent messages in model context:

```bash
go run ./cmd/harness -model gpt-4.1 -window 20
```

`-window` and `-compact-tokens` cannot be combined. A sliding window discards
old model context, while compaction retains it in a summary.

## Run one turn

Construct a runner with an explicit API key and model, then submit messages:

```go
runner, err := agent.NewRunner(
	agent.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
	agent.WithModel("gpt-4.1"),
)
if err != nil {
	return err
}

snapshot, err := runner.RunTurn(ctx, agent.RunSnapshot{}, []agent.Message{{
	Role: agent.RoleUser,
	Content: "Say hello.",
})
```

Callers create system, developer, and user messages. The runner creates
assistant and tool-result messages, so it can preserve valid tool-call sequences.

For autonomous workflows, implement `agent.Agent.Decide` and use `agent.Drive`.
The runner itself never waits for external input.

## Autonomous workflow

An autonomous agent decides what messages to submit next. `Drive` owns the
loop; `RunTurn` still owns each model/tool interaction.

```go
type ResearchAgent struct{}

func (ResearchAgent) Decide(
	ctx context.Context,
	snapshot agent.RunSnapshot,
) (agent.Action, error) {
	if len(snapshot.Transcript) == 0 {
		return agent.Action{Messages: []agent.Message{{
			Role:    agent.RoleUser,
			Content: "Find the current weather in Nanjing and summarize it.",
		}}}, nil
	}

	// The previous logical turn, including any tool calls, is complete.
	return agent.Action{Done: true}, nil
}

snapshot, err := agent.Drive(
	ctx,
	runner,
	ResearchAgent{},
	agent.RunSnapshot{},
)
```

Return `Done: true` to stop. Returning neither messages nor `Done` is an
error, preventing a busy loop. For user-driven applications, do not use
`Drive`; submit each incoming message directly with `RunTurn`.

## Packages

- `agent`: runner, messages, tools, events, run snapshots, and middleware API.
- `middleware`: model-context policies, including `SlidingWindow` and
  `AutoCompact`.
- `session`: JSON file-backed session persistence through the `Store` interface.
- `cli`: an interactive terminal agent.
- `cmd/harness`: the runnable interactive CLI.

## Context middleware

Context middleware changes only the transcript sent to the model. The runner
retains the complete canonical transcript and returns it in a `RunSnapshot`.

Every middleware has a stable ID and serializable state:

```go
type ContextMiddleware interface {
	ID() string
	Context(context.Context, Transcript) (Transcript, error)
	MarshalState() (json.RawMessage, error)
	UnmarshalState(json.RawMessage) error
}
```

An API key is required for the default OpenAI endpoint. When `WithBaseURL` is
set for a local OpenAI-compatible server, the key may be omitted.

Use `agent.WithHTTPClient` to provide a custom `*http.Client` for proxies,
custom transports, or request-level test control.

`AutoCompact` stores its generated summary and the canonical-message boundary
it represents. After compaction, later requests reuse that summary until newly
added messages exceed the configured approximate token budget. The summary
state is saved in a `RunSnapshot`, so resumed sessions do not immediately
compact the same history again.

## Sessions

Persist a run by passing a checkpoint and, later, resume with its snapshot:

```go
store := session.FileStore{Dir: ".sessions"}
value := session.Session{ID: "project-chat"}

snapshot := agent.RunSnapshot{
	Transcript:      value.Transcript,
	MiddlewareState: value.MiddlewareState,
}
snapshot, err := runner.RunTurn(ctx, snapshot, messages,
	agent.WithCheckpoint(session.Checkpoint(store, &value)),
)
```

`session.FileStore` uses atomic file replacement and restrictive file
permissions. Sessions persist transcript and middleware state only; API keys,
HTTP clients, tool handlers, and Go agent implementations remain application
configuration.

## Verification

```bash
go test ./...
go vet ./...
```
