// Package cmd — `rvs code` runs Anthropic's Claude Code CLI with the Hub
// inserted between it and api.anthropic.com. The Hub applies tier caps,
// budget, and cost tracking; the user gets the full Claude Code experience
// (tools, MCP, slash commands, permissions) charged against their org's
// Hub plan instead of a personal Anthropic account.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/rvaldemar/rvs-cli/internal/config"
	"github.com/spf13/cobra"
)

var codeCmd = &cobra.Command{
	Use:                "code [args...]",
	Short:              "Run Claude Code routed through the Hub",
	DisableFlagParsing: true,
	Long: `Run Anthropic's Claude Code CLI with ANTHROPIC_BASE_URL pointing at the
Agents Hub passthrough. Your CliToken is used as the API key, so every call
goes through the Hub's quota, budget, and cost tracking.

Requires the 'claude' binary on PATH (https://docs.anthropic.com/claude/code).
All arguments after 'code' are forwarded to claude verbatim, including
--help, --version, --print, etc.

Examples:
  rvs code                          # interactive REPL
  rvs code --print "fix the bug"    # one-shot
  rvs code --version                # show claude version
`,
	RunE: runCode,
}

func runCode(_ *cobra.Command, args []string) error {
	creds, err := config.Load()
	if err != nil {
		return fmt.Errorf("read credentials: %w", err)
	}
	if creds.Empty() {
		return errors.New("not logged in. Run: rvs login")
	}
	if creds.APIBase == "" {
		creds.APIBase = config.DefaultAPIBase
	}

	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		return errors.New(claudeNotFoundMessage())
	}

	baseURL := strings.TrimRight(creds.APIBase, "/") + "/api/v1/anthropic"

	env := append(os.Environ(),
		"ANTHROPIC_BASE_URL="+baseURL,
		"ANTHROPIC_API_KEY="+creds.Token,
	)

	c := exec.Command(claudeBin, args...) //nolint:gosec // forwarding args to a known binary
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = env

	if err := c.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("claude: %w", err)
	}
	return nil
}

func claudeNotFoundMessage() string {
	return strings.Join([]string{
		"'claude' binary not found in PATH.",
		"",
		"Install Anthropic's Claude Code first:",
		"  npm install -g @anthropic-ai/claude-code",
		"",
		"Then re-run 'rvs code'.",
	}, "\n")
}
