package ip

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "8.8.8.8" {
			t.Fatalf("request %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"ip":"8.8.8.8","location":{"country":"US"}}`))
	}))
	defer server.Close()
	result, err := (Client{HTTPClient: server.Client(), BaseURL: server.URL}).Lookup(context.Background(), "8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != `{"ip":"8.8.8.8","location":{"country":"US"}}` {
		t.Fatalf("result %s", result)
	}
}
