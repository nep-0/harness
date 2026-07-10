package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FileStore struct{ Dir string }

func (s FileStore) path(id string) (string, error) {
	if id == "" || strings.ContainsAny(id, `/\\`) {
		return "", fmt.Errorf("session: invalid ID %q", id)
	}
	return filepath.Join(s.Dir, id+".json"), nil
}
func (s FileStore) Load(ctx context.Context, id string) (Session, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	path, err := s.path(id)
	if err != nil {
		return Session{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}
	var value Session
	if err := json.Unmarshal(data, &value); err != nil {
		return Session{}, err
	}
	return value, nil
}
func (s FileStore) Save(ctx context.Context, value Session) (Session, error) {
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	path, err := s.path(value.ID)
	if err != nil {
		return Session{}, err
	}
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return Session{}, err
	}
	now := time.Now().UTC()
	if value.CreatedAt.IsZero() {
		value.CreatedAt = now
	}
	value.UpdatedAt = now
	value.Version++
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return Session{}, err
	}
	temp, err := os.CreateTemp(s.Dir, ".session-*")
	if err != nil {
		return Session{}, err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err = temp.Write(data); err == nil {
		err = temp.Chmod(0600)
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return Session{}, err
	}
	if err := os.Rename(name, path); err != nil {
		return Session{}, err
	}
	return value, nil
}
