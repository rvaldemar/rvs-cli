package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/rvaldemar/rvs-cli/internal/api"
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

var effortSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show aggregated effort totals and thesis status",
	Long: `Show aggregated effort totals and thesis-risk status.

Calls GET /api/v1/effort_entries/summary on the Hub and prints a table of
minutes by half-window plus the last-mile percentage and thesis-risk flag.

Example:
  rvs effort summary
  rvs effort summary --window 30`,
	RunE: func(cmd *cobra.Command, args []string) error {
		windowDays, _ := cmd.Flags().GetInt("window")

		ctx := context.Background()
		client, _, err := taskClient()
		if err != nil {
			return err
		}

		s, err := client.GetEffortSummary(ctx, windowDays)
		if err != nil {
			return err
		}

		return printEffortSummary(os.Stdout, s)
	},
}

// printEffortSummary writes the effort summary table to w. Extracted for testability.
func printEffortSummary(w io.Writer, s *api.EffortSummary) error {
	riskLabel := "OK"
	if s.AtRisk {
		riskLabel = "AT RISK"
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "Window\t%d days\n", s.WindowDays)
	fmt.Fprintf(tw, "Thesis\t%s\n", riskLabel)
	fmt.Fprintf(tw, "Last-mile %%\t%.1f%%\n", s.LastMilePct)
	fmt.Fprintf(tw, "Total entries\t%d\n", s.TotalEntries)
	fmt.Fprintf(tw, "Measured entries\t%d\n", s.MeasuredEntries)
	fmt.Fprintf(tw, "First-half minutes\t%.0f\n", s.FirstHalfMinutes)
	fmt.Fprintf(tw, "Second-half minutes\t%.0f\n", s.SecondHalfMinutes)
	fmt.Fprintf(tw, "Evaluated at\t%s\n", s.EvaluatedAt)
	return tw.Flush()
}

func init() {
	rootCmd.AddCommand(effortCmd)
	effortCmd.AddCommand(effortLogCmd)
	effortCmd.AddCommand(effortSummaryCmd)

	effortLogCmd.Flags().Int("minutes", 0, "minutes spent (required)")
	effortLogCmd.Flags().String("category", "other", "category: coordination|financial_review|client_communication|escalation|other")
	effortLogCmd.Flags().String("note", "", "optional free-text note")

	effortSummaryCmd.Flags().Int("window", 0, "analysis window in days (default: Hub default, 14)")
}
