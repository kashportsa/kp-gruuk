package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kashportsa/kp-gruuk/internal/config"
)

// StoredToken represents a persisted OAuth token with metadata.
type StoredToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	IDToken      string    `json:"id_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// IsExpired returns true if the access token has expired (with 1-minute buffer).
func (t *StoredToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt.Add(-1 * time.Minute))
}

// TokenStore manages token persistence at ~/.gruuk/token.json.
type TokenStore struct {
	path string
}

// NewTokenStore creates a new token store.
func NewTokenStore() (*TokenStore, error) {
	dir, err := config.GruukDir()
	if err != nil {
		return nil, err
	}
	return &TokenStore{
		path: filepath.Join(dir, "token.json"),
	}, nil
}

// Load reads the stored token from disk. Returns nil if not found.
func (ts *TokenStore) Load() (*StoredToken, error) {
	data, err := os.ReadFile(ts.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read token: %w", err)
	}

	var token StoredToken
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	return &token, nil
}

// Save writes a token to disk with secure permissions.
func (ts *TokenStore) Save(resp *TokenResponse) error {
	token := StoredToken{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		IDToken:      resp.IDToken,
		ExpiresAt:    time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second),
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}

	return os.WriteFile(ts.path, data, 0600)
}

// Clear removes the stored token.
func (ts *TokenStore) Clear() error {
	err := os.Remove(ts.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
