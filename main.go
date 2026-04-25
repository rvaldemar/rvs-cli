package main

import (
	"fmt"
	"os"

	"github.com/rvaldemar/rvs-cli/cmd"
)

// Set at link time by GoReleaser. See .goreleaser.yml.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetVersionInfo(version, commit, date)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
