package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	DefaultDomain      = "gk.kspt.dev"
	DefaultServerURL   = "wss://gk.kspt.dev"
	DefaultOktaIssuer  = "https://kashport.okta.com/oauth2/default"
	DefaultOktaClientID = "0oa221wiphrYZiPe51d8"
)

// ServerConfig holds server-side configuration loaded from environment variables.
type ServerConfig struct {
	OktaIssuer   string // OKTA_ISSUER
	OktaClientID string // OKTA_CLIENT_ID
	Domain       string // DOMAIN (default: gk.kspt.dev)
	ListenAddr   string // LISTEN_ADDR (default: :8080)
	SkipAuth     bool   // SKIP_AUTH (for testing)
}

// LoadServerConfig loads server configuration from environment variables.
func LoadServerConfig() ServerConfig {
	cfg := ServerConfig{
		OktaIssuer:   os.Getenv("OKTA_ISSUER"),
		OktaClientID: os.Getenv("OKTA_CLIENT_ID"),
		Domain:       os.Getenv("DOMAIN"),
		ListenAddr:   os.Getenv("LISTEN_ADDR"),
		SkipAuth:     os.Getenv("SKIP_AUTH") == "true",
	}
	if cfg.Domain == "" {
		cfg.Domain = DefaultDomain
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	return cfg
}

// ClientConfig holds client-side configuration.
type ClientConfig struct {
	ServerURL    string `json:"server_url"`
	OktaIssuer   string `json:"okta_issuer"`
	OktaClientID string `json:"okta_client_id"`
}

// GruukDir returns the path to ~/.gruuk, creating it if necessary.
func GruukDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".gruuk")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// LoadClientConfig loads client config from ~/.gruuk/config.json.
func LoadClientConfig() (*ClientConfig, error) {
	dir, err := GruukDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &ClientConfig{
			ServerURL:    DefaultServerURL,
			OktaIssuer:   DefaultOktaIssuer,
			OktaClientID: DefaultOktaClientID,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	var cfg ClientConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.ServerURL == "" {
		cfg.ServerURL = DefaultServerURL
	}
	if cfg.OktaIssuer == "" {
		cfg.OktaIssuer = DefaultOktaIssuer
	}
	if cfg.OktaClientID == "" {
		cfg.OktaClientID = DefaultOktaClientID
	}
	return &cfg, nil
}

// SaveClientConfig writes client config to ~/.gruuk/config.json.
func SaveClientConfig(cfg *ClientConfig) error {
	dir, err := GruukDir()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0600)
}
