package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rvaldemar/rvs-cli/internal/config"
	"github.com/spf13/cobra"
)

func TestConfigStatusForMissingToken(t *testing.T) {
	cmd := &cobra.Command{}
	t.Setenv("RVS_CONFIG_DIR", t.TempDir())

	status, err := configStatus(cmd)
	if err != nil {
		t.Fatalf("configStatus: %v", err)
	}
	if status.LoggedIn {
		t.Fatalf("expected not logged in")
	}
	if status.Ready {
		t.Fatalf("expected not ready")
	}
	if len(status.Warnings) == 0 {
		t.Fatalf("expected at least one warning")
	}
}

func TestPrintConfigStatusWithWarnings(t *testing.T) {
	var buf bytes.Buffer
	status := configStatusPayload{
		APIBase:       "https://agents.rvs.solutions",
		APIBaseSource: "default",
		Token:         "(none)",
		TokenSource:   "none",
		LoggedIn:      false,
		Ready:         false,
		Credentials:   "/tmp/rvs/credentials",
		Warnings:      configWarnings(config.Credentials{}),
	}
	if err := printConfigStatus(&buf, status); err != nil {
		t.Fatalf("printConfigStatus: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"CLI configuration",
		"Status: needs setup",
		"Actions:",
		"rvs login",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintConfigStatusReady(t *testing.T) {
	var buf bytes.Buffer
	status := configStatusPayload{
		APIBase:       "https://agents.rvs.solutions",
		APIBaseSource: "file",
		Token:         "rvs_cli_abc...1234",
		TokenSource:   "file",
		LoggedIn:      true,
		Ready:         true,
		UserEmail:     "alice@example.com",
		OrgSlug:       "acme",
		Credentials:   "/tmp/rvs/credentials",
	}
	if err := printConfigStatus(&buf, status); err != nil {
		t.Fatalf("printConfigStatus: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"Status: ready",
		"alice@example.com",
		"acme",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
