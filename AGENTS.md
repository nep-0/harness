# Repository Guidelines

## Project Overview

Harness is a small Go framework for multi-turn agents backed by OpenAI Chat Completions. Applications provide messages or an autonomous decision policy; the runner owns the canonical transcript, streams model output, dispatches tools, and can checkpoint resumable sessions.

## Architecture & Data Flow

- `agent.Runner` is the core orchestration boundary. `RunTurn` accepts a `RunSnapshot` plus caller-supplied system/developer/user messages, applies context middleware to a model-facing copy, streams a completion, executes returned tool calls, appends assistant/tool messages to the canonical transcript, and returns the updated snapshot.
- `agent.Drive` is the autonomous loop: it clones the snapshot for `Agent.Decide`, stops on `Action.Done`, and otherwise feeds returned messages into `Runner.RunTurn`. It never waits for external user input.
- Canonical transcript and middleware state are separate from model context. `middleware.SlidingWindow` and `middleware.AutoCompact` reduce only the transcript sent to the model; they must preserve valid complete tool-call turns.
- `session.Store` persists transcript and serializable middleware state. `session.Checkpoint` saves after durable transcript changes. `FileStore` uses JSON, restrictive permissions, atomic temporary-file replacement, and version/timestamp metadata.
- `cli.InteractiveAgent` converts terminal lines into user messages and treats EOF, `/exit`, and `/quit` as completion. `cmd/harness` wires flags, runner options, middleware, standard HTTP tools, optional coding workspace tools, CLI, and session persistence.

## Key Directories

- `agent/`: public messages, tools, events, snapshots, runner, and autonomous driver.
- `middleware/`: model-context policies and serializable middleware state (`SlidingWindow`, `AutoCompact`, runtime metadata, OpenAI compactor).
- `session/`: `Store` interface, JSON file store, and checkpoint callback.
- `cli/`: stdin/stdout interactive agent.
- `tools/coding/`: rooted filesystem tools (`read_file`, `list_dir`, `grep_files`, `apply_patch`) plus the explicitly trusted `shell` tool.
- `cmd/harness/`: runnable interactive CLI entry point.

## Development Commands

```bash
go run ./cmd/harness -model gpt-4.1
go run ./cmd/harness -model gpt-4.1 -session-dir .sessions -session project-chat
go run ./cmd/harness -model gpt-4.1 -compact-tokens 6000
go run ./cmd/harness -model gpt-4.1 -window 20
go run ./cmd/harness -model gpt-4.1 -coding-tools-root .
go test ./...
go vet ./...
```

Set `OPENAI_API_KEY` for the default OpenAI endpoint. A custom OpenAI-compatible `-base-url` may allow operation without a key. `-window` and `-compact-tokens` are mutually exclusive. `-coding-tools-root` enables bounded workspace tools rooted at its directory; `shell` remains trusted and unrestricted, so require an external OS sandbox where host filesystem or network access is unacceptable. Keep `.sessions/` local; it is ignored by Git.

## Code Conventions & Common Patterns

- Standard Go formatting and package layout. Exported APIs use Go doc comments; errors are returned, not hidden. Use package-prefixed, actionable error text such as `agent: ...`, `middleware: ...`, and `session: ...`.
- Prefer small interfaces and functional options for construction/configuration: `RunnerOption`, `RunOption`, `ContextMiddleware`, `Store`, and `Agent` are the principal seams. Inject `*http.Client` and base URLs for proxies, local endpoints, and deterministic HTTP tests.
- Treat `RunSnapshot` and transcripts as state passed between operations. Runner/middleware boundaries clone or copy state where needed; middleware must not mutate its input. Preserve the canonical transcript even when context policies summarize or window it.
- Context cancellation is propagated through HTTP requests and tool handlers. Runner calls are serialized because middleware state belongs to one active conversation.
- Tools expose a stable name, description, JSON parameter schema, and `ToolHandler`. Validate inputs at the boundary, honor `context.Context`, use bounded HTTP clients, check non-2xx responses, decode explicit payloads, and return useful errors.
- Streaming is event-driven through `Event` and an optional event handler. Keep logically separate streamed tool calls separate even when an upstream provider reuses an index.
- `tools/coding.Workspace` is the standard coding-agent bundle. Filesystem operations must use its `os.Root` operations rather than resolve-then-open paths, enforce configured read/search/output bounds, and honor cancellation. `apply_patch` accepts exact whole-file replacement only and serializes calls made through one workspace; external writers still require external coordination. Preserve operational failures as tool-result text so compiler output and stale-edit reasons reach the model; reserve handler errors for internal faults.

## Important Files

- `agent/runner.go`: runner construction, turn lifecycle, streaming, tool execution, OpenAI message conversion, and transcript/snapshot copying.
- `agent/types.go`: public roles, messages, tools, events, snapshots, options, and middleware contract.
- `agent/driver.go`: autonomous `Drive` loop and empty-action guard.
- `middleware/sliding_window.go`: complete-turn windowing while retaining leading instructions.
- `middleware/auto_compact.go`: summary state and canonical-message boundary for compaction.
- `middleware/runtime_metadata.go`: stable/volatile runtime facts placement.
- `session/store.go`, `session/file_store.go`: persistence contract, checkpointing, and atomic JSON storage.
- `cli/interactive.go`: terminal input semantics.
- `cmd/harness/main.go`: executable flags and dependency wiring.
- `tools/coding/workspace.go`, `tools/coding/read.go`, `tools/coding/write.go`: coding workspace contracts, bounded rooted file access, patch serialization, and trusted shell execution.
- `README.md`: public usage examples and package-level behavior.
- `go.mod`: module path, Go version, and OpenAI client dependency.

## Runtime/Tooling Preferences

Use Go 1.26 as declared in `go.mod`; use the Go toolchain and module-aware commands (`go run`, `go test`, `go vet`). The only direct external dependency is `github.com/openai/openai-go/v3`; do not introduce a second package manager or runtime. Prefer injected HTTP clients and local `httptest` servers over live network calls in tests. Shell tooling is not a sandbox: directory, argv, and filtered environment do not constrain host access.

## Testing & QA

Tests live beside implementation files and use the standard library `testing` package. Unit tests cover validation and state policies; HTTP behavior uses `httptest` local-compatible completion/tool servers; cancellation, streaming, tool success/failure, session persistence, middleware state, CLI input, workspace bounds, patch concurrency, and process cleanup are observable contracts worth preserving. Name tests `Test...` and assert returned snapshots, emitted behavior, persisted state, or user-visible output rather than implementation details.

Run focused tests while iterating, for example:

```bash
go test ./agent -run 'TestRunTurn|TestNewRunner'
go test ./middleware
go test ./session ./cli ./tools/...
```

Before delivery, run the repository-wide checks from the README: `go test ./...` and `go vet ./...`. Do not require live OpenAI or weather services; configure local HTTP-compatible endpoints or injected clients in tests.
