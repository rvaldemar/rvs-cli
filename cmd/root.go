package cmd

import (
	"github.com/spf13/cobra"
)

var (
	cliVersion = "dev"
	cliCommit  = "none"
	cliDate    = "unknown"
)

func SetVersionInfo(version, commit, date string) {
	cliVersion = version
	cliCommit = commit
	cliDate = date
}

var rootCmd = &cobra.Command{
	Use:   "rvs",
	Short: "rvs — CLI client for the Agents Hub",
	Long: `rvs is the command-line client for the RVS Agents Hub.
Log in once, then chat with your org's agents from the terminal.
Streaming, attachments, exports, and the same quota as the web app.

Get started:
  rvs login         issue a CLI token (or paste an existing one)
  rvs config show   inspect the effective API/token configuration
  rvs chat          interactive REPL with your agents
  rvs runs list     observe playbook execution from the terminal
  rvs code          agentic coding loop (laptop tools + Hub-hosted LLM)
  rvs version       print the binary version`,
}

// Execute runs the CLI. Returns the error so main can set the exit code.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(loginCmd, logoutCmd, chatCmd, codeCmd, listCmd, versionCmd, meCmd, modelsCmd)
	rootCmd.PersistentFlags().String("api", "", "override API base URL (default: https://agents.rvs.solutions)")
}
