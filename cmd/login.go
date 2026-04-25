package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/rvaldemar/rvs-cli/internal/api"
	"github.com/rvaldemar/rvs-cli/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in and store a CLI token",
	Long: `Log in to the Agents Hub.

Two modes:
  1. Email + password — POSTs to /api/v1/auth/cli/login and stores
     the issued CliToken.
  2. Paste an existing token — useful when an admin has already
     issued one for you (Settings → CLI Tokens).

Use --token to skip the prompt.`,
	RunE: runLogin,
}

func init() {
	loginCmd.Flags().String("token", "", "use an existing token issued by an admin")
	loginCmd.Flags().String("api", "", "override API base URL")
}

func runLogin(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	creds, err := config.Load()
	if err != nil {
		return err
	}
	if api, _ := cmd.Flags().GetString("api"); api != "" {
		creds.APIBase = api
	}
	if creds.APIBase == "" {
		creds.APIBase = config.DefaultAPIBase
	}

	if t, _ := cmd.Flags().GetString("token"); t != "" {
		creds.Token = strings.TrimSpace(t)
		return finalizeLogin(ctx, creds)
	}

	envToken := strings.TrimSpace(os.Getenv("RVS_TOKEN"))
	if envToken != "" {
		creds.Token = envToken
		return finalizeLogin(ctx, creds)
	}

	fmt.Println("Log in to", creds.APIBase)
	fmt.Print("Email (or paste token, leave blank for token-only): ")
	reader := bufio.NewReader(os.Stdin)
	first, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	first = strings.TrimSpace(first)

	if first == "" || looksLikeToken(first) {
		token := first
		if token == "" {
			fmt.Print("Paste token: ")
			line, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			token = strings.TrimSpace(line)
		}
		if token == "" {
			return errors.New("token cannot be empty")
		}
		creds.Token = token
		return finalizeLogin(ctx, creds)
	}

	email := first
	fmt.Print("Password: ")
	password, err := readPassword()
	if err != nil {
		return err
	}
	fmt.Println()

	client := api.New(creds.APIBase, "")
	resp, err := client.Login(ctx, email, password, "")
	if err != nil {
		return err
	}
	creds.Token = resp.Token
	creds.UserEmail = resp.User.Email
	creds.OrgSlug = resp.Org.Slug
	if err := config.Save(creds); err != nil {
		return err
	}
	fmt.Printf("Logged in as %s (%s). Token prefix: %s\n", resp.User.Email, resp.Org.Name, resp.Prefix)
	return nil
}

func finalizeLogin(ctx context.Context, creds config.Credentials) error {
	client := api.New(creds.APIBase, creds.Token)
	me, err := client.Me(ctx)
	if err != nil {
		return fmt.Errorf("verify token: %w", err)
	}
	if me.Data.User != nil {
		creds.UserEmail = me.Data.User.Email
	}
	if err := config.Save(creds); err != nil {
		return err
	}
	if me.Data.User != nil {
		fmt.Printf("Logged in as %s (%s).\n", me.Data.User.Email, me.Data.Organization.Name)
	} else {
		fmt.Printf("Logged in to %s.\n", me.Data.Organization.Name)
	}
	return nil
}

func looksLikeToken(s string) bool {
	// Heuristic: tokens are >24 chars, no @, no whitespace.
	if strings.Contains(s, "@") || strings.Contains(s, " ") {
		return false
	}
	return len(s) >= 24
}

func readPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		// Fallback: read from stdin in plain text (CI/tests).
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		return strings.TrimSpace(line), err
	}
	pw, err := term.ReadPassword(fd)
	if err != nil {
		return "", err
	}
	return string(pw), nil
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Erase the saved token",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Clear(); err != nil {
			return err
		}
		fmt.Println("Logged out.")
		return nil
	},
}
