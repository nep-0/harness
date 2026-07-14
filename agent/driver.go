package agent

import (
	"context"
	"errors"
)

// Drive repeatedly executes an autonomous agent's decisions.
func Drive(ctx context.Context, runner *Runner, source Agent, snapshot RunSnapshot, options ...RunOption) (RunSnapshot, error) {
	if source == nil {
		return snapshot, errors.New("agent: agent is required")
	}
	for {
		action, err := source.Decide(ctx, cloneSnapshot(snapshot))
		if err != nil {
			return snapshot, err
		}
		if action.Done {
			return snapshot, nil
		}
		if len(action.Messages) == 0 {
			return snapshot, errors.New("agent: action requires messages or Done")
		}
		snapshot, err = runner.RunTurn(ctx, snapshot, action.Messages, options...)
		if err != nil {
			return snapshot, err
		}
	}
}
