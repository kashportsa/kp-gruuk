package auth

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *TokenStore {
	t.Helper()
	return &TokenStore{path: filepath.Join(t.TempDir(), "token.json")}
}

func TestTokenStoreLoadNonExistent(t *testing.T) {
	ts := newTestStore(t)

	tok, err := ts.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if tok != nil {
		t.Fatal("expected nil token for non-existent file")
	}
}

func TestTokenStoreSaveAndLoad(t *testing.T) {
	ts := newTestStore(t)

	resp := &TokenResponse{
		AccessToken:  "access123",
		RefreshToken: "refresh456",
		IDToken:      "id789",
		ExpiresIn:    3600,
	}

	if err := ts.Save(resp); err != nil {
		t.Fatalf("Save: %v", err)
	}

	tok, err := ts.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tok == nil {
		t.Fatal("expected non-nil token after Save")
	}
	if tok.AccessToken != resp.AccessToken {
		t.Errorf("AccessToken: got %q, want %q", tok.AccessToken, resp.AccessToken)
	}
	if tok.RefreshToken != resp.RefreshToken {
		t.Errorf("RefreshToken: got %q, want %q", tok.RefreshToken, resp.RefreshToken)
	}
	if tok.IDToken != resp.IDToken {
		t.Errorf("IDToken: got %q, want %q", tok.IDToken, resp.IDToken)
	}
}

func TestTokenStoreExpiresAt(t *testing.T) {
	ts := newTestStore(t)

	before := time.Now()
	resp := &TokenResponse{
		AccessToken: "token",
		ExpiresIn:   3600,
	}
	ts.Save(resp)
	after := time.Now()

	tok, _ := ts.Load()
	// ExpiresAt should be ~1 hour from now, within the before/after window
	low := before.Add(3599 * time.Second)
	high := after.Add(3601 * time.Second)
	if tok.ExpiresAt.Before(low) || tok.ExpiresAt.After(high) {
		t.Errorf("ExpiresAt %v outside expected range [%v, %v]", tok.ExpiresAt, low, high)
	}
}

func TestTokenIsExpired(t *testing.T) {
	cases := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"expired 10 minutes ago", time.Now().Add(-10 * time.Minute), true},
		{"expires in 30 seconds (within buffer)", time.Now().Add(30 * time.Second), true},
		{"expires in 5 minutes", time.Now().Add(5 * time.Minute), false},
		{"expires in 1 hour", time.Now().Add(time.Hour), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := &StoredToken{ExpiresAt: tc.expiresAt}
			if got := tok.IsExpired(); got != tc.want {
				t.Errorf("IsExpired() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTokenStoreClear(t *testing.T) {
	ts := newTestStore(t)

	resp := &TokenResponse{AccessToken: "token", ExpiresIn: 3600}
	ts.Save(resp)

	if err := ts.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	tok, err := ts.Load()
	if err != nil {
		t.Fatalf("Load after Clear: %v", err)
	}
	if tok != nil {
		t.Fatal("expected nil token after Clear")
	}
}

func TestTokenStoreClearNonExistent(t *testing.T) {
	ts := newTestStore(t)
	// Should not error when file doesn't exist
	if err := ts.Clear(); err != nil {
		t.Fatalf("Clear on non-existent file: %v", err)
	}
}
