package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

var effortCmd = &cobra.Command{
	Use:   "effort",
	Short: "Track time and effort on Hub activities",
}

var effortLogCmd = &cobra.Command{
	Use:   "log",
	Short: "Log minutes spent on a Hub activity",
	Long: `Log minutes spent on a Hub activity.

Categories: coordination, financial_review, client_communication, escalation, other.

Example:
  rvs effort log --minutes 30 --category financial_review --note "Reviewed Bergano invoices"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		minutes, _ := cmd.Flags().GetInt("minutes")
		if minutes <= 0 {
			return errors.New("--minutes is required and must be > 0")
		}
		category, _ := cmd.Flags().GetString("category")
		note, _ := cmd.Flags().GetString("note")

		ctx := context.Background()
		client, _, err := taskClient()
		if err != nil {
			return err
		}

		entry, err := client.LogEffortEntry(ctx, minutes, category, note)
		if err != nil {
			return err
		}

		cat := entry.Category
		if cat == "" {
			cat = category
		}
		fmt.Printf("Logged %d minutes (%s).\n", entry.Minutes, cat)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(effortCmd)
	effortCmd.AddCommand(effortLogCmd)

	effortLogCmd.Flags().Int("minutes", 0, "minutes spent (required)")
	effortLogCmd.Flags().String("category", "other", "category: coordination|financial_review|client_communication|escalation|other")
	effortLogCmd.Flags().String("note", "", "optional free-text note")
}
