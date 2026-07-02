// Package config persists CLI credentials to the user's config dir.
//
// File: $XDG_CONFIG_HOME/rvs/credentials (or ~/.config/rvs/credentials).
// Permissions: 0600. Format: simple key=value, one per line. Stored values:
//
//	api=https://agents.rvs.solutions
//	token=<bearer token>
//	user_email=alice@example.com
//	org_slug=acme
//
// Environment overrides are intentionally supported for CI and agent runners:
// RVS_TOKEN overrides token, and RVS_API_BASE / RVS_API override api.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultAPIBase = "https://agents.rvs.solutions"

type Credentials struct {
	APIBase       string
	Token         string
	UserEmail     string
	OrgSlug       string
	APIBaseSource string
	TokenSource   string
}

func (c Credentials) Empty() bool {
	return c.Token == ""
}

func path() (string, error) {
	if override := os.Getenv("RVS_CONFIG_DIR"); override != "" {
		return filepath.Join(override, "credentials"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(dir, "rvs", "credentials"), nil
}

func Path() (string, error) {
	return path()
}

func Load() (Credentials, error) {
	p, err := path()
	if err != nil {
		return Credentials{}, err
	}

	c := Credentials{APIBase: DefaultAPIBase, APIBaseSource: "default"}
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			applyEnvOverrides(&c)
			return c, nil
		}
		return Credentials{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "api":
			if value := strings.TrimSpace(v); value != "" {
				c.APIBase = value
				c.APIBaseSource = "file"
			}
		case "token":
			if value := strings.TrimSpace(v); value != "" {
				c.Token = value
				c.TokenSource = "file"
			}
		case "user_email":
			c.UserEmail = strings.TrimSpace(v)
		case "org_slug":
			c.OrgSlug = strings.TrimSpace(v)
		}
	}
	if err := scanner.Err(); err != nil {
		return c, err
	}
	if c.APIBase == "" {
		c.APIBase = DefaultAPIBase
		c.APIBaseSource = "default"
	}
	applyEnvOverrides(&c)
	return c, nil
}

func applyEnvOverrides(c *Credentials) {
	if api := strings.TrimSpace(os.Getenv("RVS_API_BASE")); api != "" {
		c.APIBase = api
		c.APIBaseSource = "RVS_API_BASE"
	} else if api := strings.TrimSpace(os.Getenv("RVS_API")); api != "" {
		c.APIBase = api
		c.APIBaseSource = "RVS_API"
	}

	if token := strings.TrimSpace(os.Getenv("RVS_TOKEN")); token != "" {
		c.Token = token
		c.TokenSource = "RVS_TOKEN"
	}
}

func Save(c Credentials) error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	fmt.Fprintln(w, "# rvs CLI credentials. Do not edit manually.")
	fmt.Fprintf(w, "api=%s\n", c.APIBase)
	fmt.Fprintf(w, "token=%s\n", c.Token)
	if c.UserEmail != "" {
		fmt.Fprintf(w, "user_email=%s\n", c.UserEmail)
	}
	if c.OrgSlug != "" {
		fmt.Fprintf(w, "org_slug=%s\n", c.OrgSlug)
	}
	return w.Flush()
}

func Clear() error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
