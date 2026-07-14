// Command harness runs an interactive Chat Completions agent.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/nep-0/harness/agent"
	"github.com/nep-0/harness/cli"
	"github.com/nep-0/harness/middleware"
	"github.com/nep-0/harness/session"
	"github.com/nep-0/harness/tools/ip"
	"github.com/nep-0/harness/tools/weather"
)

func main() {
	apiKey := flag.String("api-key", os.Getenv("OPENAI_API_KEY"), "OpenAI API key")
	model := flag.String("model", "", "Chat Completions model")
	baseURL := flag.String("base-url", "", "optional API base URL")
	window := flag.Int("window", 0, "model-visible complete-turn window; zero disables it")
	maxTurns := flag.Int("max-turns", 0, "maximum completion requests; zero is unlimited")
	compactTokens := flag.Int("compact-tokens", 0, "approximate context-token budget; zero disables compaction")
	sessionDir := flag.String("session-dir", "", "directory for persisted sessions")
	sessionID := flag.String("session", "", "session ID to create or resume")
	flag.Parse()

	if *window > 0 && *compactTokens > 0 {
		fmt.Fprintln(os.Stderr, "-window and -compact-tokens cannot be used together")
		os.Exit(2)
	}
	runnerOptions := baseRunnerOptions(*apiKey, *model, *baseURL, *maxTurns)
	if *window > 0 {
		slidingWindow, err := middleware.NewSlidingWindow(*window)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		runnerOptions = append(runnerOptions, agent.WithMiddleware(slidingWindow))
	}
	if *compactTokens > 0 {
		compactor, err := middleware.NewOpenAICompactor(*apiKey, *model, *baseURL)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		compact, err := middleware.NewAutoCompact(*compactTokens, 8, compactor)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		runnerOptions = append(runnerOptions, agent.WithMiddleware(compact))
	}
	runner, err := agent.NewRunner(runnerOptions...)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	interactive := cli.NewInteractiveAgent(os.Stdin, os.Stdout)
	snapshot := agent.RunSnapshot{}
	options := []agent.RunOption{}
	if *sessionDir != "" || *sessionID != "" {
		if *sessionDir == "" || *sessionID == "" {
			fmt.Fprintln(os.Stderr, "-session-dir and -session must be used together")
			os.Exit(2)
		}
		store := session.FileStore{Dir: *sessionDir}
		value, err := store.Load(context.Background(), *sessionID)
		if err == session.ErrNotFound {
			value = session.Session{ID: *sessionID}
		} else if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		snapshot = agent.RunSnapshot{Transcript: value.Transcript, MiddlewareState: value.MiddlewareState}
		options = append(options, agent.WithCheckpoint(session.Checkpoint(store, &value)))
	}
	for {
		turn, err := interactive.Next(context.Background(), snapshot.Transcript)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if turn.Done {
			break
		}
		snapshot, err = runner.RunTurn(context.Background(), snapshot, turn.Messages, options...)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func baseRunnerOptions(apiKey, model, baseURL string, maxTurns int) []agent.RunnerOption {
	return []agent.RunnerOption{
		agent.WithAPIKey(apiKey),
		agent.WithModel(model),
		agent.WithBaseURL(baseURL),
		agent.WithMaxTurns(maxTurns),
		agent.WithTool(weather.Client{}.Tool()),
		agent.WithTool(ip.Client{}.Tool()),
		agent.WithEventHandler(renderEvent),
	}
}

func renderEvent(event agent.Event) error {
	switch event.Type {
	case agent.EventTextDelta:
		_, err := fmt.Fprint(os.Stdout, event.Delta)
		return err
	case agent.EventToolStarted:
		_, err := fmt.Fprintf(os.Stdout, "\n[calling tool: %s]\n", event.ToolCall.Name)
		return err
	case agent.EventToolFinished:
		_, err := fmt.Fprintf(os.Stdout, "[tool finished: %s]\n", event.ToolCall.Name)
		return err
	default:
		return nil
	}
}
