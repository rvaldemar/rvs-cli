package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/rvaldemar/rvs-cli/internal/api"
	"github.com/rvaldemar/rvs-cli/internal/config"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent conversations",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		creds, err := config.Load()
		if err != nil {
			return err
		}
		if creds.Empty() {
			return errors.New("not logged in. Run: rvs login")
		}
		c := api.New(creds.APIBase, creds.Token)
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
		creds, err := config.Load()
		if err != nil {
			return err
		}
		if creds.Empty() {
			return errors.New("not logged in. Run: rvs login")
		}
		c := api.New(creds.APIBase, creds.Token)
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
		fmt.Printf("Quota: %d / %d cents (%.1f%%) — period from %s\n",
			q.Data.CurrentMonthSpend, q.Data.MonthlyBudgetCents, q.Data.BudgetUsedPct, q.Data.PeriodStart)
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
		creds, err := config.Load()
		if err != nil {
			return err
		}
		if creds.Empty() {
			return errors.New("not logged in. Run: rvs login")
		}
		c := api.New(creds.APIBase, creds.Token)
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
