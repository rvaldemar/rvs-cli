package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rvaldemar/rvs-cli/internal/api"
)

func TestPrintPlaybookRuns(t *testing.T) {
	var buf bytes.Buffer
	err := printPlaybookRuns(&buf, []api.PlaybookRun{{
		ID:             "run-1",
		Status:         "running",
		PlaybookCode:   "S01",
		CurrentStepID:  "extract",
		StartedAt:      "2026-07-02T10:00:00Z",
		TotalCostCents: 3,
	}})
	if err != nil {
		t.Fatalf("printPlaybookRuns: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"ID", "run-1", "running", "S01", "extract", "3c"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintPlaybookRunDetail(t *testing.T) {
	var buf bytes.Buffer
	err := printPlaybookRun(&buf, &api.PlaybookRun{
		ID:             "run-1",
		Status:         "failed",
		PlaybookCode:   "S01",
		CurrentStepID:  "approve",
		ErrorMessage:   "top-level failure",
		TotalCostCents: 8,
		StepResults: []api.StepResult{{
			StepID:       "approve",
			Status:       "failed",
			DurationMS:   1250,
			CostCents:    5,
			ErrorMessage: "approval failed",
		}},
		Output: map[string]any{"ok": false},
	})
	if err != nil {
		t.Fatalf("printPlaybookRun: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"run-1", "failed", "top-level failure", "approve", "1.25s", "5c", "approval failed", `"ok": false`} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestReadableAPIErrorExtractsErrorMessage(t *testing.T) {
	err := readableAPIError(&api.APIError{Status: 422, Body: `{"error":"Run already terminated (done)"}`})
	if err == nil || err.Error() != "Run already terminated (done)" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigRedactsToken(t *testing.T) {
	if got := redactToken("rvs_cli_abcdefghijklmnop"); got != "rvs_cli_...mnop" {
		t.Fatalf("redactToken: got %q", got)
	}
}
