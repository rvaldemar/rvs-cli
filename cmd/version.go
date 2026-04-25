package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the rvs CLI version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("rvs %s (%s, %s)\n", cliVersion, cliCommit, cliDate)
	},
}
