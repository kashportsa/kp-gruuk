package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// DeviceFlowConfig holds configuration for the Okta Device Authorization Flow.
type DeviceFlowConfig struct {
	Issuer   string // e.g., "https://kashport.okta.com/oauth2/default"
	ClientID string
	Scopes   []string // e.g., ["openid", "email", "profile"]
}

// TokenResponse represents a successful token exchange response.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

type deviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	Interval                int    `json:"interval"`
	ExpiresIn               int    `json:"expires_in"`
}

type tokenErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// RunDeviceFlow performs the Okta Device Authorization Flow.
// It prints instructions to stdout and polls for the token.
func RunDeviceFlow(ctx context.Context, cfg DeviceFlowConfig) (*TokenResponse, error) {
	scopes := strings.Join(cfg.Scopes, " ")
	if scopes == "" {
		scopes = "openid email profile"
	}

	// Step 1: Request device authorization
	deviceAuthURL := cfg.Issuer + "/v1/device/authorize"
	data := url.Values{
		"client_id": {cfg.ClientID},
		"scope":     {scopes},
	}

	resp, err := http.PostForm(deviceAuthURL, data)
	if err != nil {
		return nil, fmt.Errorf("device authorize request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device authorize failed (%d): %s", resp.StatusCode, body)
	}

	var authResp deviceAuthResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		return nil, fmt.Errorf("parse device auth response: %w", err)
	}

	// Step 2: Display instructions
	fmt.Println()
	fmt.Println("  To authenticate, open your browser at:")
	fmt.Printf("  %s\n\n", authResp.VerificationURIComplete)
	fmt.Printf("  Or go to %s and enter code: %s\n\n", authResp.VerificationURI, authResp.UserCode)
	fmt.Println("  Waiting for authentication...")

	// Try to open browser automatically
	openBrowser(authResp.VerificationURIComplete)

	// Step 3: Poll for token
	interval := time.Duration(authResp.Interval) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}

	tokenURL := cfg.Issuer + "/v1/token"
	deadline := time.Now().Add(time.Duration(authResp.ExpiresIn) * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}

		tokenResp, err := pollToken(tokenURL, cfg.ClientID, authResp.DeviceCode)
		if err != nil {
			return nil, err
		}
		if tokenResp != nil {
			fmt.Println("  Authenticated successfully!")
			fmt.Println()
			return tokenResp, nil
		}
	}

	return nil, fmt.Errorf("device authorization expired")
}

func pollToken(tokenURL, clientID, deviceCode string) (*TokenResponse, error) {
	data := url.Values{
		"client_id":   {clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		var tokenResp TokenResponse
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			return nil, fmt.Errorf("parse token response: %w", err)
		}
		return &tokenResp, nil
	}

	var errResp tokenErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return nil, fmt.Errorf("token request failed (%d): %s", resp.StatusCode, body)
	}

	switch errResp.Error {
	case "authorization_pending":
		return nil, nil // keep polling
	case "slow_down":
		time.Sleep(5 * time.Second) // back off and keep polling
		return nil, nil
	case "expired_token":
		return nil, fmt.Errorf("device code expired, please try again")
	case "access_denied":
		return nil, fmt.Errorf("access denied by user")
	default:
		return nil, fmt.Errorf("token error: %s - %s", errResp.Error, errResp.ErrorDescription)
	}
}

// RefreshAccessToken uses a refresh token to get a new access token.
func RefreshAccessToken(issuer, clientID, refreshToken string) (*TokenResponse, error) {
	tokenURL := issuer + "/v1/token"
	data := url.Values{
		"client_id":     {clientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return nil, fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh failed (%d): %s", resp.StatusCode, body)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse refresh response: %w", err)
	}
	return &tokenResp, nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	cmd.Start()
}
