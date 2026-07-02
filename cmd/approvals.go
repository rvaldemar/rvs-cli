package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/rvaldemar/rvs-cli/internal/api"
	"github.com/spf13/cobra"
)

var approvalsCmd = &cobra.Command{
	Use:   "approvals",
	Short: "Inspect and decide pending approvals",
}

var approvalsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List playbook approvals",
	Long: `List playbook approvals for the current organization.

Examples:
  rvs approvals list
  rvs approvals list --status pending
  rvs approvals list --status approved --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		filter, err := approvalFilterFromFlags(cmd)
		if err != nil {
			return err
		}
		ctx := context.Background()
		client, _, err := authenticatedClient(cmd)
		if err != nil {
			return err
		}
		approvals, err := client.ListApprovals(ctx, filter)
		if err != nil {
			return readableAPIError(err)
		}
		if taskJSONFlag(cmd) {
			return printJSON(approvals)
		}
		return printApprovals(os.Stdout, approvals)
	},
}

var approvalsShowCmd = &cobra.Command{
	Use:   "show APPROVAL_ID",
	Short: "Show a playbook approval",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, _, err := authenticatedClient(cmd)
		if err != nil {
			return err
		}
		approval, err := client.GetApproval(ctx, args[0])
		if err != nil {
			return readableAPIError(err)
		}
		if taskJSONFlag(cmd) {
			return printJSON(approval)
		}
		return printApproval(os.Stdout, approval)
	},
}

var approvalsDecideCmd = &cobra.Command{
	Use:   "decide APPROVAL_ID [approve|reject]",
	Short: "Approve or reject a playbook approval",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		decision, err := mapApprovalDecision(args[1])
		if err != nil {
			return err
		}
		note, _ := cmd.Flags().GetString("note")
		decidedBy, _ := cmd.Flags().GetString("decided-by")
		effortMinutes, _ := cmd.Flags().GetInt("effort-minutes")
		var effort *int
		if cmd.Flags().Lookup("effort-minutes").Changed {
			effort = &effortMinutes
		}

		ctx := context.Background()
		client, _, err := authenticatedClient(cmd)
		if err != nil {
			return err
		}
		approval, err := client.DecideApproval(ctx, id, decision, decidedBy, note, effort)
		if err != nil {
			return readableAPIError(err)
		}
		if taskJSONFlag(cmd) {
			return printJSON(approval)
		}
		fmt.Printf("Approval %s marked as %s.\n", approval.ID, approval.Status)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(approvalsCmd)
	approvalsCmd.AddCommand(approvalsListCmd, approvalsShowCmd, approvalsDecideCmd)
	for _, c := range []*cobra.Command{approvalsListCmd, approvalsShowCmd, approvalsDecideCmd} {
		c.Flags().Bool("json", false, "write JSON output")
	}

	approvalsListCmd.Flags().String("status", "", "filter by status: pending|approved|rejected|expired")
	approvalsDecideCmd.Flags().String("note", "", "decision note")
	approvalsDecideCmd.Flags().String("decided-by", "", "override decision actor")
	approvalsDecideCmd.Flags().Int("effort-minutes", -1, "minutes spent on the decision")
}

func approvalFilterFromFlags(cmd *cobra.Command) (api.ApprovalFilter, error) {
	status, _ := cmd.Flags().GetString("status")
	if status != "" && !validApprovalStatus(status) {
		return api.ApprovalFilter{}, fmt.Errorf("invalid --status %q", status)
	}
	return api.ApprovalFilter{Status: status}, nil
}

func mapApprovalDecision(raw string) (string, error) {
	switch strings.ToLower(raw) {
	case "approve", "approved":
		return "approved", nil
	case "reject", "rejected":
		return "rejected", nil
	default:
		return "", fmt.Errorf("invalid decision %q: expected approve or reject", raw)
	}
}

func validApprovalStatus(status string) bool {
	switch status {
	case "pending", "approved", "rejected", "expired":
		return true
	default:
		return false
	}
}

func printApprovals(w io.Writer, approvals []api.Approval) error {
	if len(approvals) == 0 {
		fmt.Fprintln(w, "No approvals found.")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tRUN\tSTEP\tREQUESTED_VIA\tREQUESTED_AT\tTIMEOUT_AT")
	for _, approval := range approvals {
		fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			approval.ID,
			valueOrDash(approval.Status),
			valueOrDash(approval.PlaybookRunID),
			valueOrDash(approval.StepID),
			valueOrDash(approval.RequestedVia),
			valueOrDash(approval.RequestedAt),
			valueOrDash(approval.TimeoutAt),
		)
	}
	return tw.Flush()
}

func printApproval(w io.Writer, approval *api.Approval) error {
	if approval == nil {
		fmt.Fprintln(w, "No approval.")
		return nil
	}
	fmt.Fprintf(w, "ID:            %s\n", approval.ID)
	fmt.Fprintf(w, "Status:        %s\n", approval.Status)
	fmt.Fprintf(w, "Playbook run:  %s\n", valueOrDash(approval.PlaybookRunID))
	fmt.Fprintf(w, "Step:          %s\n", valueOrDash(approval.StepID))
	fmt.Fprintf(w, "Requested via: %s\n", valueOrDash(approval.RequestedVia))
	fmt.Fprintf(w, "Requested at:  %s\n", valueOrDash(approval.RequestedAt))
	if approval.DecidedAt != "" {
		fmt.Fprintf(w, "Decided at:    %s\n", approval.DecidedAt)
	}
	fmt.Fprintf(w, "Decided by:    %s\n", valueOrDash(approval.DecidedBy))
	if approval.DecisionNote != "" {
		fmt.Fprintf(w, "Decision note: %s\n", approval.DecisionNote)
	}
	if approval.TimeoutAt != "" {
		fmt.Fprintf(w, "Timeout at:    %s\n", approval.TimeoutAt)
	}
	if approval.Context != nil {
		fmt.Fprintln(w, "\nContext:")
		fmt.Fprintln(w, prettyJSON(approval.Context))
	}
	return nil
}
