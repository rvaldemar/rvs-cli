package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var templatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "Manage playbook templates",
}

var templatesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available playbook templates",
	Long: `List available playbook templates.

Example:
  rvs templates list
  rvs templates list --domain coordination`,
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, _ := cmd.Flags().GetString("domain")

		ctx := context.Background()
		client, _, err := taskClient(cmd)
		if err != nil {
			return err
		}

		templates, err := client.ListPlaybookTemplates(ctx, domain)
		if err != nil {
			return err
		}

		if len(templates) == 0 {
			fmt.Println("No playbook templates found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "CODE\tDOMAIN\tNAME\tVERSION\tCERTIFIED")
		for _, t := range templates {
			certified := "no"
			if t.Certified {
				certified = "yes"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.Code, t.Domain, t.Name, t.Version, certified)
		}
		return w.Flush()
	},
}

var templatesUseCmd = &cobra.Command{
	Use:   "use <template-id>",
	Short: "Instantiate a playbook template",
	Long: `Create a new playbook instance from a template.

Example:
  rvs templates use <template-id>
  rvs templates use <template-id> --code my-playbook-001`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		templateID := args[0]
		if templateID == "" {
			return errors.New("template-id is required")
		}
		code, _ := cmd.Flags().GetString("code")

		ctx := context.Background()
		client, _, err := taskClient(cmd)
		if err != nil {
			return err
		}

		instance, err := client.InstantiatePlaybookTemplate(ctx, templateID, code)
		if err != nil {
			return err
		}

		fmt.Printf("Playbook '%s' created from template %s.\n", instance.Code, templateID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(templatesCmd)
	templatesCmd.AddCommand(templatesListCmd, templatesUseCmd)

	templatesListCmd.Flags().String("domain", "", "filter by domain")
	templatesUseCmd.Flags().String("code", "", "custom playbook code for the new instance (default: server auto-derive)")
}
