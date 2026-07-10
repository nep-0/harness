package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Paris" || r.URL.Query().Get("format") != "j1" {
			t.Fatalf("request %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"current_condition":[{"weatherDesc":[{"value":"Sunny"}],"temp_C":"20","FeelsLikeC":"19","humidity":"50","windspeedKmph":"10"}],"nearest_area":[{"value":"Paris"}]}`))
	}))
	defer server.Close()
	report, err := (Client{HTTPClient: server.Client(), BaseURL: server.URL}).Lookup(context.Background(), "Paris")
	if err != nil {
		t.Fatal(err)
	}
	if report.Condition != "Sunny" || report.TemperatureC != "20" {
		t.Fatalf("report %#v", report)
	}
}
