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
	window := flag.Int("window", 0, "model-visible message window; zero disables it")
	maxTurns := flag.Int("max-turns", 0, "maximum completion requests; zero is unlimited")
	compactTokens := flag.Int("compact-tokens", 0, "approximate context-token budget; zero disables compaction")
	sessionDir := flag.String("session-dir", "", "directory for persisted sessions")
	sessionID := flag.String("session", "", "session ID to create or resume")
	flag.Parse()

	if *window > 0 && *compactTokens > 0 {
		fmt.Fprintln(os.Stderr, "-window and -compact-tokens cannot be used together")
		os.Exit(2)
	}
	config := agent.Config{APIKey: *apiKey, Model: *model, BaseURL: *baseURL, MaxTurns: *maxTurns, Tools: []agent.Tool{weather.Client{}.Tool(), ip.Client{}.Tool()}, OnEvent: func(event agent.Event) error {
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
		}
		return nil
	}}
	if *window > 0 {
		slidingWindow, err := middleware.NewSlidingWindow(*window)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		config.Middlewares = append(config.Middlewares, slidingWindow)
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
		config.Middlewares = append(config.Middlewares, compact)
	}
	runner, err := agent.NewRunner(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	interactive := cli.NewInteractiveAgent(os.Stdin, os.Stdout)
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
		options = append(options, agent.WithSnapshot(agent.RunSnapshot{Transcript: value.Transcript, MiddlewareState: value.MiddlewareState}), agent.WithCheckpoint(session.Checkpoint(store, &value)))
	}
	if _, err := runner.Run(context.Background(), interactive, options...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
