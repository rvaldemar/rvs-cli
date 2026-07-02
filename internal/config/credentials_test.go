package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RVS_CONFIG_DIR", dir)

	in := Credentials{
		APIBase:   "https://example.test",
		Token:     "abcd1234efgh5678",
		UserEmail: "alice@example.com",
		OrgSlug:   "acme",
	}
	if err := Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// File should exist with 0600 permissions.
	p := filepath.Join(dir, "credentials")
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm: got %o want 0600", perm)
	}

	out, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.Token != in.Token || out.UserEmail != in.UserEmail || out.APIBase != in.APIBase || out.OrgSlug != in.OrgSlug {
		t.Errorf("roundtrip mismatch: in=%+v out=%+v", in, out)
	}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RVS_CONFIG_DIR", dir)

	out, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !out.Empty() {
		t.Errorf("expected empty creds, got token %q", out.Token)
	}
	if out.APIBase != DefaultAPIBase {
		t.Errorf("expected default API base, got %q", out.APIBase)
	}
}

func TestLoadUsesEnvironmentTokenWithoutCredentialsFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RVS_CONFIG_DIR", dir)
	t.Setenv("RVS_TOKEN", "env-token")

	out, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.Token != "env-token" {
		t.Errorf("token: got %q", out.Token)
	}
	if out.TokenSource != "RVS_TOKEN" {
		t.Errorf("token source: got %q", out.TokenSource)
	}
	if out.APIBase != DefaultAPIBase {
		t.Errorf("api base: got %q", out.APIBase)
	}
}

func TestLoadEnvironmentOverridesCredentialsFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RVS_CONFIG_DIR", dir)

	if err := Save(Credentials{APIBase: "https://file.example", Token: "file-token"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	t.Setenv("RVS_TOKEN", "env-token")
	t.Setenv("RVS_API_BASE", "https://env.example")

	out, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.Token != "env-token" || out.TokenSource != "RVS_TOKEN" {
		t.Errorf("token override mismatch: %+v", out)
	}
	if out.APIBase != "https://env.example" || out.APIBaseSource != "RVS_API_BASE" {
		t.Errorf("api override mismatch: %+v", out)
	}
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RVS_CONFIG_DIR", dir)

	if err := Save(Credentials{Token: "abc"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	out, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !out.Empty() {
		t.Errorf("expected empty after clear, got %q", out.Token)
	}
	// Idempotent
	if err := Clear(); err != nil {
		t.Errorf("Clear idempotent: %v", err)
	}
}
