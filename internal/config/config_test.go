package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadServerConfigDefaults(t *testing.T) {
	os.Unsetenv("DOMAIN")
	os.Unsetenv("LISTEN_ADDR")
	os.Unsetenv("OKTA_ISSUER")
	os.Unsetenv("OKTA_CLIENT_ID")
	os.Unsetenv("SKIP_AUTH")

	cfg := LoadServerConfig()

	if cfg.Domain != DefaultDomain {
		t.Errorf("Domain: got %q, want %q", cfg.Domain, DefaultDomain)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr: got %q, want %q", cfg.ListenAddr, ":8080")
	}
	if cfg.SkipAuth {
		t.Error("SkipAuth: expected false by default")
	}
	if cfg.OktaIssuer != "" || cfg.OktaClientID != "" {
		t.Error("expected empty Okta config when env vars not set")
	}
}

func TestLoadServerConfigFromEnv(t *testing.T) {
	t.Setenv("DOMAIN", "custom.example.com")
	t.Setenv("LISTEN_ADDR", ":9090")
	t.Setenv("OKTA_ISSUER", "https://example.okta.com/oauth2/default")
	t.Setenv("OKTA_CLIENT_ID", "0oa123")
	t.Setenv("SKIP_AUTH", "true")

	cfg := LoadServerConfig()

	if cfg.Domain != "custom.example.com" {
		t.Errorf("Domain: got %q, want %q", cfg.Domain, "custom.example.com")
	}
	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr: got %q, want %q", cfg.ListenAddr, ":9090")
	}
	if cfg.OktaIssuer != "https://example.okta.com/oauth2/default" {
		t.Errorf("OktaIssuer: got %q", cfg.OktaIssuer)
	}
	if cfg.OktaClientID != "0oa123" {
		t.Errorf("OktaClientID: got %q", cfg.OktaClientID)
	}
	if !cfg.SkipAuth {
		t.Error("SkipAuth: expected true")
	}
}

func TestLoadServerConfigSkipAuthOnlyOnExactTrue(t *testing.T) {
	for _, val := range []string{"1", "yes", "TRUE", "True", ""} {
		t.Setenv("SKIP_AUTH", val)
		cfg := LoadServerConfig()
		if cfg.SkipAuth {
			t.Errorf("SKIP_AUTH=%q should not enable skip-auth (only 'true' does)", val)
		}
	}
}

func TestLoadClientConfigNoFile(t *testing.T) {
	// Point GruukDir to a temp location with no config file
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cfg, err := LoadClientConfig()
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.ServerURL != DefaultServerURL {
		t.Errorf("ServerURL: got %q, want %q", cfg.ServerURL, DefaultServerURL)
	}
}

func TestSaveAndLoadClientConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	original := &ClientConfig{
		ServerURL:    "wss://custom.example.com",
		OktaIssuer:   "https://issuer.okta.com",
		OktaClientID: "0oa456",
	}

	if err := SaveClientConfig(original); err != nil {
		t.Fatalf("SaveClientConfig: %v", err)
	}

	loaded, err := LoadClientConfig()
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if loaded.ServerURL != original.ServerURL {
		t.Errorf("ServerURL: got %q, want %q", loaded.ServerURL, original.ServerURL)
	}
	if loaded.OktaIssuer != original.OktaIssuer {
		t.Errorf("OktaIssuer: got %q, want %q", loaded.OktaIssuer, original.OktaIssuer)
	}
	if loaded.OktaClientID != original.OktaClientID {
		t.Errorf("OktaClientID: got %q, want %q", loaded.OktaClientID, original.OktaClientID)
	}
}

func TestLoadClientConfigDefaultsEmptyServerURL(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write config with empty server_url
	gruukDir := filepath.Join(dir, ".gruuk")
	os.MkdirAll(gruukDir, 0700)
	os.WriteFile(filepath.Join(gruukDir, "config.json"), []byte(`{"server_url":""}`), 0600)

	cfg, err := LoadClientConfig()
	if err != nil {
		t.Fatalf("LoadClientConfig: %v", err)
	}
	if cfg.ServerURL != DefaultServerURL {
		t.Errorf("expected default ServerURL when empty, got %q", cfg.ServerURL)
	}
}
