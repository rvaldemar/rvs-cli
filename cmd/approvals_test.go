package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rvaldemar/rvs-cli/internal/api"
)

func TestPrintApprovals(t *testing.T) {
	var buf bytes.Buffer
	if err := printApprovals(&buf, []api.Approval{{
		ID:            "ap-1",
		Status:        "pending",
		PlaybookRunID: "run-1",
		StepID:        "human-review",
		RequestedVia:  "whatsapp",
		RequestedAt:   "2026-07-01T10:00:00Z",
		TimeoutAt:     "2026-07-01T10:05:00Z",
	}}); err != nil {
		t.Fatalf("printApprovals: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"ID", "ap-1", "pending", "run-1", "human-review", "whatsapp"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintApproval(t *testing.T) {
	var buf bytes.Buffer
	if err := printApproval(&buf, &api.Approval{
		ID:            "ap-1",
		Status:        "approved",
		PlaybookRunID: "run-1",
		StepID:        "human-review",
		RequestedVia:  "web",
		RequestedAt:   "2026-07-01T10:00:00Z",
		DecidedBy:     "maria",
		DecidedAt:     "2026-07-01T10:01:00Z",
		DecisionNote:  "go ahead",
		Context:       map[string]any{"step_name": "review"},
	}); err != nil {
		t.Fatalf("printApproval: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"ap-1", "approved", "run-1", "human-review", "Decided by", "go ahead", "Context", "step_name"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestMapApprovalDecision(t *testing.T) {
	got, err := mapApprovalDecision("approve")
	if err != nil || got != "approved" {
		t.Fatalf("approve: got %q err %v", got, err)
	}
	got, err = mapApprovalDecision("rejected")
	if err != nil || got != "rejected" {
		t.Fatalf("rejected: got %q err %v", got, err)
	}
	if _, err := mapApprovalDecision("ignore"); err == nil {
		t.Fatal("expected error for invalid decision")
	}
}
