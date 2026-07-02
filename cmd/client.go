package cmd

import (
	"errors"
	"strings"

	"github.com/rvaldemar/rvs-cli/internal/api"
	"github.com/rvaldemar/rvs-cli/internal/config"
	"github.com/spf13/cobra"
)

func commandCredentials(cmd *cobra.Command) (config.Credentials, error) {
	creds, err := config.Load()
	if err != nil {
		return creds, err
	}
	if override := apiOverrideFlag(cmd); override != "" {
		creds.APIBase = strings.TrimRight(override, "/")
		creds.APIBaseSource = "--api"
	}
	if creds.APIBase == "" {
		creds.APIBase = config.DefaultAPIBase
		creds.APIBaseSource = "default"
	}
	return creds, nil
}

func authenticatedClient(cmd *cobra.Command) (*api.Client, config.Credentials, error) {
	creds, err := commandCredentials(cmd)
	if err != nil {
		return nil, creds, err
	}
	if creds.Empty() {
		return nil, creds, errors.New("not logged in. Run: rvs login or set RVS_TOKEN")
	}
	return api.New(creds.APIBase, creds.Token), creds, nil
}

func apiOverrideFlag(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	if flag := cmd.Flags().Lookup("api"); flag != nil {
		value, _ := cmd.Flags().GetString("api")
		return strings.TrimSpace(value)
	}
	if flag := cmd.InheritedFlags().Lookup("api"); flag != nil {
		value, _ := cmd.InheritedFlags().GetString("api")
		return strings.TrimSpace(value)
	}
	if root := cmd.Root(); root != nil {
		if flag := root.PersistentFlags().Lookup("api"); flag != nil {
			value, _ := root.PersistentFlags().GetString("api")
			return strings.TrimSpace(value)
		}
	}
	return ""
}
