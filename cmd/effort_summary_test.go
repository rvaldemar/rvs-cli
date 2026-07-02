package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rvaldemar/rvs-cli/internal/api"
)

func TestGetEffortSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/effort_entries/summary" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"at_risk":             false,
				"last_mile_pct":       42.5,
				"first_half_minutes":  120.0,
				"second_half_minutes": 90.0,
				"total_entries":       10,
				"measured_entries":    7,
				"window_days":         14,
				"evaluated_at":        "2026-06-30T00:00:00Z",
			},
		})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "test-token")
	summary, err := client.GetEffortSummary(t.Context(), 0)
	if err != nil {
		t.Fatalf("GetEffortSummary: %v", err)
	}

	if summary.AtRisk {
		t.Error("expected at_risk=false")
	}
	if summary.LastMilePct != 42.5 {
		t.Errorf("last_mile_pct: got %.1f, want 42.5", summary.LastMilePct)
	}
	if summary.WindowDays != 14 {
		t.Errorf("window_days: got %d, want 14", summary.WindowDays)
	}
	if summary.TotalEntries != 10 {
		t.Errorf("total_entries: got %d, want 10", summary.TotalEntries)
	}
	if summary.FirstHalfMinutes != 120.0 {
		t.Errorf("first_half_minutes: got %.1f, want 120.0", summary.FirstHalfMinutes)
	}
}

func TestGetEffortSummaryWindowParam(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"at_risk":             false,
				"last_mile_pct":       0.0,
				"first_half_minutes":  0.0,
				"second_half_minutes": 0.0,
				"total_entries":       0,
				"measured_entries":    0,
				"window_days":         30,
				"evaluated_at":        "2026-06-30T00:00:00Z",
			},
		})
	}))
	defer srv.Close()

	client := api.New(srv.URL, "test-token")
	_, err := client.GetEffortSummary(t.Context(), 30)
	if err != nil {
		t.Fatalf("GetEffortSummary: %v", err)
	}
	if !strings.Contains(gotPath, "window_days=30") {
		t.Errorf("expected window_days=30 in request path, got %q", gotPath)
	}
}

func TestPrintEffortSummary(t *testing.T) {
	s := &api.EffortSummary{
		AtRisk:            true,
		LastMilePct:       75.0,
		FirstHalfMinutes:  200.0,
		SecondHalfMinutes: 210.0,
		TotalEntries:      20,
		MeasuredEntries:   15,
		WindowDays:        14,
		EvaluatedAt:       "2026-06-30T00:00:00Z",
	}

	var buf bytes.Buffer
	if err := printEffortSummary(&buf, s); err != nil {
		t.Fatalf("printEffortSummary: %v", err)
	}
	out := buf.String()

	checks := []string{
		"14 days",
		"AT RISK",
		"75.0%",
		"200",
		"210",
		"20",
		"15",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}
