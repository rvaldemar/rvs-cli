package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent conversations",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c, _, err := authenticatedClient(cmd)
		if err != nil {
			return err
		}
		convs, err := c.ListConversations(ctx)
		if err != nil {
			return err
		}
		if len(convs) == 0 {
			fmt.Println("No conversations yet.")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tUPDATED\tTITLE")
		for _, c := range convs {
			title := c.Title
			if title == "" {
				title = "(untitled)"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", c.ID, c.UpdatedAt, title)
		}
		return w.Flush()
	},
}

var meCmd = &cobra.Command{
	Use:   "me",
	Short: "Show the current session identity and quota",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		c, _, err := authenticatedClient(cmd)
		if err != nil {
			return err
		}
		me, err := c.Me(ctx)
		if err != nil {
			return err
		}
		q, err := c.Quota(ctx)
		if err != nil {
			return err
		}
		if me.Data.User != nil {
			fmt.Printf("User:  %s (%s)\n", me.Data.User.Email, me.Data.User.Name)
		}
		fmt.Printf("Org:   %s (tier: %s)\n", me.Data.Organization.Name, me.Data.Organization.AITier)
		runtime := me.Data.Organization.Runtime
		if runtime.Provider == "" {
			runtime = q.Data.Runtime
		}
		if runtime.Provider != "" {
			fmt.Printf("Run:   %s / %s (%s)\n", runtime.Provider, runtime.Model, runtime.Status)
		}
		if q.Data.MonthlyTokenCap > 0 || q.Data.MonthlyCostCapCents > 0 {
			tier := q.Data.TierConfigName
			if tier == "" {
				tier = "unconfigured"
			}
			fmt.Printf("Plan:  %s (scope: %s)\n", tier, q.Data.QuotaScope)
			fmt.Printf("Quota: %d / %d tokens, %d / %d cents (%.1f%%) — period from %s\n",
				q.Data.CurrentMonthTokens, q.Data.MonthlyTokenCap,
				q.Data.CurrentMonthCostCents, q.Data.MonthlyCostCapCents,
				q.Data.QuotaUsedPct, q.Data.PeriodStart)
		} else {
			fmt.Printf("Quota: %d / %d cents (%.1f%%) — period from %s\n",
				q.Data.CurrentMonthSpend, q.Data.MonthlyBudgetCents, q.Data.BudgetUsedPct, q.Data.PeriodStart)
		}
		if q.Data.OverBudget {
			fmt.Println("WARN:  over budget")
		}
		return nil
	},
}

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "List models available to your org",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		c, _, err := authenticatedClient(cmd)
		if err != nil {
			return err
		}
		models, err := c.Models(ctx)
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTIER\tPROVIDER\tNAME")
		for _, m := range models {
			marker := ""
			if m.Default {
				marker = " *"
			}
			fmt.Fprintf(w, "%s%s\t%s\t%s\t%s\n", m.ID, marker, m.Tier, m.Provider, m.Name)
		}
		return w.Flush()
	},
}
