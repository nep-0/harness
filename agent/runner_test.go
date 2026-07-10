package agent

import "testing"

func TestNewRunnerValidatesConfig(t *testing.T) {
	if _, err := NewRunner(Config{}); err == nil {
		t.Fatal("expected missing API key error")
	}
	if _, err := NewRunner(Config{APIKey: "key"}); err == nil {
		t.Fatal("expected missing model error")
	}
}
