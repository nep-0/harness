package middleware

import (
	"context"
	"testing"

	"github.com/nep-0/harness/agent"
)

func TestRuntimeMetadataPlacesStablePrefixAndVolatileSuffix(t *testing.T) {
	middleware := NewRuntimeMetadata(
		func(context.Context) (map[string]any, error) { return map[string]any{"city": "Nanjing"}, nil },
		func(context.Context) (map[string]any, error) { return map[string]any{"time": "now"}, nil },
	)
	context, err := middleware.Context(context.Background(), agent.Transcript{{Role: agent.RoleSystem, Content: "system"}, {Role: agent.RoleUser, Content: "question"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(context) != 4 || context[1].Content != "Stable runtime metadata:\n{\"city\":\"Nanjing\"}" || context[3].Content != "Volatile runtime metadata:\n{\"time\":\"now\"}" {
		t.Fatalf("context %#v", context)
	}
}
