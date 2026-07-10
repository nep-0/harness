// Package session persists resumable agent conversations.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/nep-0/harness/agent"
)

var ErrNotFound = errors.New("session: not found")

// Session stores canonical agent state. Runner configuration and Go tool
// handlers are deliberately not persisted.
type Session struct {
	ID              string                     `json:"id"`
	Version         int64                      `json:"version"`
	CreatedAt       time.Time                  `json:"created_at"`
	UpdatedAt       time.Time                  `json:"updated_at"`
	Transcript      agent.Transcript           `json:"transcript"`
	MiddlewareState map[string]json.RawMessage `json:"middleware_state,omitempty"`
	Metadata        map[string]string          `json:"metadata,omitempty"`
}

type Store interface {
	Load(context.Context, string) (Session, error)
	Save(context.Context, Session) (Session, error)
}

// Checkpoint returns a runner callback that saves a copy after every change.
func Checkpoint(store Store, current *Session) func(context.Context, agent.RunSnapshot) error {
	return func(ctx context.Context, snapshot agent.RunSnapshot) error {
		current.Transcript = snapshot.Transcript
		current.MiddlewareState = snapshot.MiddlewareState
		saved, err := store.Save(ctx, *current)
		if err == nil {
			*current = saved
		}
		return err
	}
}
