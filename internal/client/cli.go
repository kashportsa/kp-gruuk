package client

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/kashportsa/kp-gruuk/internal/auth"
	"github.com/kashportsa/kp-gruuk/internal/config"
	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	serverURL string
	tokenFlag string
)

// NewRootCmd creates the root CLI command.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "gruuk",
		Short: "KP-Gruuk — Expose local services through gk.kspt.dev",
		Long:  "KP-Gruuk is an internal tunnel service for Kashport developers.\nExpose local services through authenticated tunnels on gk.kspt.dev.",
	}

	root.PersistentFlags().StringVar(&serverURL, "server-url", "", "override server URL (default: wss://gk.kspt.dev)")
	root.PersistentFlags().StringVar(&tokenFlag, "token", "", "use a specific access token (skip auth)")

	root.AddCommand(newExposeCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newLogoutCmd())
	root.AddCommand(newVersionCmd())

	return root
}

func newExposeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "expose <port>",
		Short: "Expose a local service through a gruuk tunnel",
		Args:  cobra.ExactArgs(1),
		RunE:  runExpose,
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		RunE:  runStatus,
	}
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear stored authentication tokens",
		RunE:  runLogout,
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("gruuk %s\n", version)
		},
	}
}

func runExpose(cmd *cobra.Command, args []string) error {
	port, err := strconv.Atoi(args[0])
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %s", args[0])
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.LoadClientConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	srvURL := serverURL
	if srvURL == "" {
		srvURL = cfg.ServerURL
	}

	token := tokenFlag
	if token == "" {
		token, err = getValidToken(cmd.Context(), cfg)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	localProxy := NewLocalProxy(port)
	client := NewTunnelClient(srvURL, token, localProxy, logger)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// Connect
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	fmt.Printf("\n  Tunnel active!\n")
	fmt.Printf("  %s -> http://localhost:%d\n\n", client.PublicURL, port)
	fmt.Printf("  Press Ctrl+C to stop.\n\n")

	// Run with reconnection
	errCh := make(chan error, 1)
	go func() {
		errCh <- runWithReconnect(ctx, client, srvURL, token, localProxy, port, logger)
	}()

	select {
	case <-sigCh:
		fmt.Println("\n  Shutting down...")
		cancel()
		client.Close()
		return nil
	case err := <-errCh:
		return err
	}
}

func runWithReconnect(ctx context.Context, client *TunnelClient, srvURL, token string, localProxy *LocalProxy, port int, logger *slog.Logger) error {
	attempt := 0
	maxBackoff := 30 * time.Second

	// Track the active client as a local pointer to avoid copying the struct
	// (copying a sync.Mutex after first use is undefined behavior in Go).
	current := client

	for {
		err := current.Run(ctx)
		if ctx.Err() != nil {
			return nil // context cancelled, clean shutdown
		}

		attempt++
		backoff := time.Duration(1<<min(attempt, 5)) * time.Second
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		// Add jitter (25%)
		jitter := time.Duration(float64(backoff) * 0.25)
		backoff = backoff - jitter/2 + time.Duration(time.Now().UnixNano()%int64(jitter+1))

		logger.Warn("tunnel disconnected, reconnecting...", "attempt", attempt, "backoff", backoff, "error", err)
		fmt.Printf("  Reconnecting... (attempt %d)\n", attempt)

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}

		// Create new client and reconnect
		newClient := NewTunnelClient(srvURL, token, localProxy, logger)
		if err := newClient.Connect(ctx); err != nil {
			logger.Warn("reconnect failed", "error", err)
			continue
		}

		current = newClient
		attempt = 0

		fmt.Printf("  Reconnected! %s -> http://localhost:%d\n\n", current.PublicURL, port)
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	store, err := auth.NewTokenStore()
	if err != nil {
		return err
	}

	token, err := store.Load()
	if err != nil {
		return err
	}

	if token == nil {
		fmt.Println("Not authenticated. Run 'gruuk expose <port>' to authenticate.")
		return nil
	}

	if token.IsExpired() {
		fmt.Println("Token expired. Run 'gruuk expose <port>' to re-authenticate.")
	} else {
		fmt.Printf("Authenticated. Token valid until %s\n", token.ExpiresAt.Format(time.RFC3339))
	}
	return nil
}

func runLogout(cmd *cobra.Command, args []string) error {
	store, err := auth.NewTokenStore()
	if err != nil {
		return err
	}

	if err := store.Clear(); err != nil {
		return err
	}

	fmt.Println("Logged out. Stored tokens cleared.")
	return nil
}

func getValidToken(ctx context.Context, cfg *config.ClientConfig) (string, error) {
	store, err := auth.NewTokenStore()
	if err != nil {
		return "", err
	}

	// Try existing token
	token, err := store.Load()
	if err != nil {
		return "", err
	}

	if token != nil && !token.IsExpired() {
		return token.AccessToken, nil
	}

	// Try refresh
	if token != nil && token.RefreshToken != "" && cfg.OktaIssuer != "" {
		refreshed, err := auth.RefreshAccessToken(cfg.OktaIssuer, cfg.OktaClientID, token.RefreshToken)
		if err == nil {
			if saveErr := store.Save(refreshed); saveErr != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to cache refreshed token: %v\n", saveErr)
			}
			return refreshed.AccessToken, nil
		}
	}

	// Run device flow
	if cfg.OktaIssuer == "" || cfg.OktaClientID == "" {
		return "", fmt.Errorf("Okta not configured. Set okta_issuer and okta_client_id in ~/.gruuk/config.json")
	}

	tokenResp, err := auth.RunDeviceFlow(ctx, auth.DeviceFlowConfig{
		Issuer:   cfg.OktaIssuer,
		ClientID: cfg.OktaClientID,
		Scopes:   []string{"openid", "email", "profile", "offline_access"},
	})
	if err != nil {
		return "", err
	}

	if saveErr := store.Save(tokenResp); saveErr != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to cache token: %v\n", saveErr)
	}
	return tokenResp.AccessToken, nil
}
