package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/rvaldemar/rvs-cli/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect CLI configuration and authentication",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return cmd.Help()
		}
		status, err := configStatus(cmd)
		if err != nil {
			return err
		}
		if taskJSONFlag(cmd) {
			return printJSON(status)
		}
		return printConfigStatus(os.Stdout, status)
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show effective CLI configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		status, err := configStatus(cmd)
		if err != nil {
			return err
		}
		if taskJSONFlag(cmd) {
			return printJSON(status)
		}
		return printConfigStatus(os.Stdout, status)
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the credentials file path",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := config.Path()
		if err != nil {
			return err
		}
		fmt.Println(p)
		return nil
	},
}

var configDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Verify that the effective token can reach the Hub",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, creds, err := authenticatedClient(cmd)
		if err != nil {
			return err
		}
		me, err := client.Me(ctx)
		if err != nil {
			return fmt.Errorf("Hub auth check failed: %w", err)
		}
		if taskJSONFlag(cmd) {
			return printJSON(me)
		}
		if me.Data.User != nil {
			fmt.Printf("OK: %s as %s (%s)\n", creds.APIBase, me.Data.User.Email, me.Data.Organization.Name)
		} else {
			fmt.Printf("OK: %s (%s)\n", creds.APIBase, me.Data.Organization.Name)
		}
		return nil
	},
}

type configStatusPayload struct {
	APIBase       string   `json:"api_base"`
	APIBaseSource string   `json:"api_base_source"`
	Token         string   `json:"token"`
	TokenSource   string   `json:"token_source"`
	LoggedIn      bool     `json:"logged_in"`
	Ready         bool     `json:"ready"`
	UserEmail     string   `json:"user_email,omitempty"`
	OrgSlug       string   `json:"org_slug,omitempty"`
	Credentials   string   `json:"credentials_path"`
	Warnings      []string `json:"warnings,omitempty"`
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configShowCmd, configPathCmd, configDoctorCmd)
	configCmd.PersistentFlags().Bool("json", false, "write JSON output")
}

func configStatus(cmd *cobra.Command) (configStatusPayload, error) {
	creds, err := commandCredentials(cmd)
	if err != nil {
		return configStatusPayload{}, err
	}
	p, err := config.Path()
	if err != nil {
		return configStatusPayload{}, err
	}
	tokenSource := creds.TokenSource
	if tokenSource == "" && creds.Token == "" {
		tokenSource = "none"
	}
	apiSource := creds.APIBaseSource
	if apiSource == "" {
		apiSource = "default"
	}
	return configStatusPayload{
		APIBase:       creds.APIBase,
		APIBaseSource: apiSource,
		Token:         redactToken(creds.Token),
		TokenSource:   tokenSource,
		LoggedIn:      !creds.Empty(),
		Ready:         !creds.Empty(),
		UserEmail:     creds.UserEmail,
		OrgSlug:       creds.OrgSlug,
		Credentials:   p,
		Warnings:      configWarnings(creds),
	}, nil
}

func printConfigStatus(w io.Writer, status configStatusPayload) error {
	fmt.Fprintln(w, "CLI configuration")
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "  API\t%s (%s)\n", status.APIBase, status.APIBaseSource)
	fmt.Fprintf(tw, "  Token\t%s (%s)\n", status.Token, status.TokenSource)
	fmt.Fprintf(tw, "  Logged in\t%t\n", status.LoggedIn)
	fmt.Fprintf(tw, "  Credentials\t%s\n", status.Credentials)
	if status.UserEmail != "" {
		fmt.Fprintf(tw, "  User\t%s\n", status.UserEmail)
	}
	if status.OrgSlug != "" {
		fmt.Fprintf(tw, "  Org\t%s\n", status.OrgSlug)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	fmt.Fprintln(w, "")
	if len(status.Warnings) == 0 {
		fmt.Fprintln(w, "Status: ready")
		return nil
	}

	fmt.Fprintln(w, "Status: needs setup")
	fmt.Fprintln(w, "Actions:")
	for _, warning := range status.Warnings {
		fmt.Fprintf(w, "  - %s\n", warning)
	}
	return nil
}

func configWarnings(creds config.Credentials) []string {
	var warnings []string
	if creds.Empty() {
		warnings = append(warnings, "No token configured. Run `rvs login` or set `RVS_TOKEN`.")
	}
	if creds.APIBase == "" {
		warnings = append(warnings, "API base is empty. Set --api or `RVS_API_BASE`.")
	}
	return warnings
}

func redactToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return "(none)"
	}
	if len(token) <= 12 {
		return "***"
	}
	return token[:8] + "..." + token[len(token)-4:]
}
