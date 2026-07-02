package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/rvaldemar/rvs-cli/internal/api"
	"github.com/spf13/cobra"
)

var runsCmd = &cobra.Command{
	Use:   "runs",
	Short: "Observe and control playbook runs",
}

var runsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List playbook runs",
	Long: `List playbook runs for the current organization.

Examples:
  rvs runs list
  rvs runs list --status running
  rvs runs list --from 2026-07-01 --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		filter, err := playbookRunFilterFromFlags(cmd)
		if err != nil {
			return err
		}
		ctx := context.Background()
		client, _, err := authenticatedClient(cmd)
		if err != nil {
			return err
		}
		runs, err := client.ListPlaybookRuns(ctx, filter)
		if err != nil {
			return readableAPIError(err)
		}
		if taskJSONFlag(cmd) {
			return printJSON(runs)
		}
		return printPlaybookRuns(os.Stdout, runs)
	},
}

var runsShowCmd = &cobra.Command{
	Use:   "show RUN_ID",
	Short: "Show a playbook run with step detail",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, _, err := authenticatedClient(cmd)
		if err != nil {
			return err
		}
		run, err := client.GetPlaybookRun(ctx, args[0])
		if err != nil {
			return readableAPIError(err)
		}
		if taskJSONFlag(cmd) {
			return printJSON(run)
		}
		return printPlaybookRun(os.Stdout, run)
	},
}

var runsCancelCmd = &cobra.Command{
	Use:   "cancel RUN_ID",
	Short: "Cancel a running or waiting playbook run",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, _, err := authenticatedClient(cmd)
		if err != nil {
			return err
		}
		run, err := client.CancelPlaybookRun(ctx, args[0])
		if err != nil {
			return readableAPIError(err)
		}
		if taskJSONFlag(cmd) {
			return printJSON(run)
		}
		fmt.Printf("Canceled %s\n", run.ID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(runsCmd)
	runsCmd.AddCommand(runsListCmd, runsShowCmd, runsCancelCmd)
	for _, c := range []*cobra.Command{runsListCmd, runsShowCmd, runsCancelCmd} {
		c.Flags().Bool("json", false, "write JSON output")
	}
	runsListCmd.Flags().String("status", "", "filter by status: pending|running|waiting_approval|waiting_fan_out|done|failed|cancelled")
	runsListCmd.Flags().String("playbook-id", "", "filter by playbook id")
	runsListCmd.Flags().String("from", "", "filter runs created after this timestamp/date")
	runsListCmd.Flags().String("to", "", "filter runs created before this timestamp/date")
}

func playbookRunFilterFromFlags(cmd *cobra.Command) (api.PlaybookRunFilter, error) {
	status, _ := cmd.Flags().GetString("status")
	if status != "" && !validRunStatus(status) {
		return api.PlaybookRunFilter{}, fmt.Errorf("invalid --status %q", status)
	}
	playbookID, _ := cmd.Flags().GetString("playbook-id")
	from, _ := cmd.Flags().GetString("from")
	to, _ := cmd.Flags().GetString("to")
	return api.PlaybookRunFilter{
		Status:     status,
		PlaybookID: playbookID,
		From:       from,
		To:         to,
	}, nil
}

func validRunStatus(status string) bool {
	switch status {
	case "pending", "running", "waiting_approval", "waiting_fan_out", "done", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func printPlaybookRuns(w io.Writer, runs []api.PlaybookRun) error {
	if len(runs) == 0 {
		fmt.Fprintln(w, "No playbook runs found.")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tPLAYBOOK\tCURRENT_STEP\tSTARTED\tCOST")
	for _, run := range runs {
		fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			run.ID,
			run.Status,
			valueOrDash(run.PlaybookCode),
			valueOrDash(run.CurrentStepID),
			valueOrDash(run.StartedAt),
			formatCents(run.TotalCostCents),
		)
	}
	return tw.Flush()
}

func printPlaybookRun(w io.Writer, run *api.PlaybookRun) error {
	if run == nil {
		fmt.Fprintln(w, "No playbook run.")
		return nil
	}
	fmt.Fprintf(w, "ID:           %s\n", run.ID)
	fmt.Fprintf(w, "Status:       %s\n", run.Status)
	fmt.Fprintf(w, "Playbook:     %s\n", valueOrDash(run.PlaybookCode))
	fmt.Fprintf(w, "Current step: %s\n", valueOrDash(run.CurrentStepID))
	fmt.Fprintf(w, "Started:      %s\n", valueOrDash(run.StartedAt))
	if run.FinishedAt != "" {
		fmt.Fprintf(w, "Finished:     %s\n", run.FinishedAt)
	}
	if run.DurationSeconds > 0 {
		fmt.Fprintf(w, "Duration:     %s\n", formatSeconds(run.DurationSeconds))
	}
	fmt.Fprintf(w, "Cost:         %s\n", formatCents(run.TotalCostCents))
	if run.ErrorMessage != "" {
		fmt.Fprintf(w, "\nError:\n%s\n", run.ErrorMessage)
	}
	if len(run.StepResults) > 0 {
		fmt.Fprintln(w, "\nSteps:")
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "STEP\tSTATUS\tDURATION\tCOST\tERROR")
		for _, step := range run.StepResults {
			fmt.Fprintf(
				tw,
				"%s\t%s\t%s\t%s\t%s\n",
				valueOrDash(step.StepID),
				valueOrDash(step.Status),
				formatStepDuration(step),
				formatCents(step.CostCents),
				valueOrDash(stepError(step)),
			)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	if run.Output != nil {
		fmt.Fprintln(w, "\nOutput:")
		fmt.Fprintln(w, truncateForTerminal(prettyJSON(run.Output), 4000))
	}
	return nil
}

func readableAPIError(err error) error {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		if message := apiErrorMessage(apiErr.Body); message != "" {
			return errors.New(message)
		}
	}
	return err
}

func apiErrorMessage(body string) string {
	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return strings.TrimSpace(body)
	}
	if payload.Error != "" {
		return payload.Error
	}
	return payload.Message
}

func stepError(step api.StepResult) string {
	if step.ErrorMessage != "" {
		return step.ErrorMessage
	}
	return step.Error
}

func formatStepDuration(step api.StepResult) string {
	if step.DurationMS > 0 {
		return formatMilliseconds(step.DurationMS)
	}
	return "-"
}

func formatMilliseconds(ms int64) string {
	return (time.Duration(ms) * time.Millisecond).String()
}

func formatSeconds(seconds float64) string {
	return (time.Duration(seconds * float64(time.Second))).String()
}

func formatCents(cents int64) string {
	if cents == 0 {
		return "-"
	}
	return fmt.Sprintf("%dc", cents)
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func prettyJSON(value any) string {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(raw)
}

func truncateForTerminal(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n[truncated]"
}
