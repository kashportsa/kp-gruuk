package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/kashportsa/kp-gruuk/internal/auth"
	"github.com/kashportsa/kp-gruuk/internal/config"
	"github.com/kashportsa/kp-gruuk/internal/server"
)

func main() {
	skipAuth := flag.Bool("skip-auth", false, "disable JWT validation (testing only)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg := config.LoadServerConfig()
	if *skipAuth {
		cfg.SkipAuth = true
	}

	srv := server.New(server.Config{
		Domain:     cfg.Domain,
		ListenAddr: cfg.ListenAddr,
		SkipAuth:   cfg.SkipAuth,
	}, logger)

	if !cfg.SkipAuth {
		if cfg.OktaIssuer == "" || cfg.OktaClientID == "" {
			fmt.Fprintln(os.Stderr, "OKTA_ISSUER and OKTA_CLIENT_ID must be set (or use --skip-auth)")
			os.Exit(1)
		}

		validator, err := auth.NewJWTValidator(cfg.OktaIssuer, cfg.OktaClientID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to initialize JWT validator: %v\n", err)
			os.Exit(1)
		}
		defer validator.Close()

		srv.ValidateToken = validator.Validate
		logger.Info("JWT validation enabled", "issuer", cfg.OktaIssuer)
	} else {
		logger.Warn("JWT validation DISABLED (skip-auth mode)")
	}

	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
