package session

import (
	"context"
	"testing"

	"harness/agent"
)

func TestFileStoreRoundTrip(t *testing.T) {
	store := FileStore{Dir: t.TempDir()}
	saved, err := store.Save(context.Background(), Session{ID: "chat-1", Transcript: agent.Transcript{{Role: agent.RoleUser, Content: "hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), "chat-1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != saved.Version || len(loaded.Transcript) != 1 || loaded.Transcript[0].Content != "hello" {
		t.Fatalf("loaded = %#v", loaded)
	}
}
