package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/kashportsa/kp-gruuk/internal/tunnel"
	"nhooyr.io/websocket"
)

// Config holds server configuration.
type Config struct {
	Domain     string // e.g., "gk.kspt.dev"
	ListenAddr string // e.g., ":8080"
	SkipAuth   bool   // disable JWT validation (for testing)
}

// Server is the main KP-Gruuk server that handles both tunnel connections and visitor requests.
type Server struct {
	config   Config
	registry *Registry
	proxy    *Proxy
	logger   *slog.Logger

	// ValidateToken is called to validate a JWT token from tunnel clients.
	// It should return the email address from the token claims, or an error.
	// When SkipAuth is true, this is not called.
	ValidateToken func(token string) (email string, err error)
}

// New creates a new Server.
func New(cfg Config, logger *slog.Logger) *Server {
	registry := NewRegistry(logger)
	proxy := NewProxy(registry, 30*time.Second, logger)

	return &Server{
		config:   cfg,
		registry: registry,
		proxy:    proxy,
		logger:   logger,
	}
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	srv := &http.Server{
		Addr:              s.config.ListenAddr,
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.logger.Info("server starting", "addr", s.config.ListenAddr, "domain", s.config.Domain)
	return srv.ListenAndServe()
}

// ServeHTTP routes requests based on the Host header.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	subdomain := s.extractSubdomain(r.Host)

	if subdomain == "" {
		switch r.URL.Path {
		case "/_ws/connect":
			s.handleTunnelConnect(w, r)
		case "/_health":
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "ok")
		default:
			s.serveLanding(w)
		}
		return
	}

	s.proxy.ServeHTTP(w, r, subdomain)
}

func (s *Server) extractSubdomain(host string) string {
	// Strip port if present
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}

	domain := s.config.Domain
	if !strings.HasSuffix(host, "."+domain) {
		return ""
	}

	sub := strings.TrimSuffix(host, "."+domain)
	if sub == "" || strings.Contains(sub, ".") {
		return ""
	}
	return sub
}

func (s *Server) handleTunnelConnect(w http.ResponseWriter, r *http.Request) {
	var email string

	if s.config.SkipAuth {
		// In skip-auth mode, use a query param or header for the subdomain
		email = r.URL.Query().Get("email")
		if email == "" {
			email = r.Header.Get("X-Email")
		}
		if email == "" {
			http.Error(w, "email required (skip-auth mode)", http.StatusBadRequest)
			return
		}
	} else {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}

		if s.ValidateToken == nil {
			http.Error(w, "auth not configured", http.StatusInternalServerError)
			return
		}

		var err error
		email, err = s.ValidateToken(token)
		if err != nil {
			s.logger.Warn("token validation failed", "error", err)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
	}

	subdomain := EmailToSubdomain(email)

	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		s.logger.Error("websocket accept failed", "error", err)
		return
	}

	// Increase read limit for large request/response bodies
	ws.SetReadLimit(16 << 20) // 16MB

	mux := tunnel.NewMux(30 * time.Second)
	conn := &TunnelConn{
		Subdomain: subdomain,
		WS:        ws,
		Mux:       mux,
	}

	if err := s.registry.Register(subdomain, conn); err != nil {
		errPayload := &tunnel.ErrorPayload{
			Code:    "subdomain_taken",
			Message: fmt.Sprintf("subdomain %q is already in use", subdomain),
		}
		env, _ := tunnel.NewEnvelope(tunnel.TypeError, "", errPayload)
		data, _ := json.Marshal(env)
		ws.Write(r.Context(), websocket.MessageText, data)
		ws.Close(websocket.StatusPolicyViolation, "subdomain taken")
		mux.Close()
		return
	}

	defer func() {
		s.registry.Unregister(subdomain)
		mux.Close()
		ws.Close(websocket.StatusNormalClosure, "")
	}()

	// Send connected message
	publicURL := fmt.Sprintf("https://%s.%s", subdomain, s.config.Domain)
	connPayload := &tunnel.ConnectedPayload{
		Subdomain: subdomain,
		PublicURL: publicURL,
	}
	connEnv, _ := tunnel.NewEnvelope(tunnel.TypeConnected, "", connPayload)
	if err := conn.Send(r.Context(), connEnv); err != nil {
		s.logger.Error("failed to send connected message", "error", err)
		return
	}

	s.logger.Info("tunnel connected", "subdomain", subdomain, "url", publicURL)

	// Read pump: reads messages from the client
	s.readPump(r.Context(), conn)
}

func (s *Server) readPump(ctx context.Context, conn *TunnelConn) {
	for {
		_, data, err := conn.WS.Read(ctx)
		if err != nil {
			s.logger.Debug("tunnel read error", "subdomain", conn.Subdomain, "error", err)
			return
		}

		var env tunnel.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			s.logger.Warn("invalid message from client", "subdomain", conn.Subdomain, "error", err)
			continue
		}

		switch env.Type {
		case tunnel.TypeHTTPResponse:
			var resp tunnel.HTTPResponsePayload
			if err := env.DecodePayload(&resp); err != nil {
				s.logger.Warn("invalid response payload", "error", err)
				continue
			}
			conn.Mux.Resolve(env.RequestID, &resp)

		case tunnel.TypePong:
			// heartbeat response, no action needed

		default:
			s.logger.Warn("unexpected message type from client", "type", env.Type)
		}
	}
}

func (s *Server) serveLanding(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>KP-Gruuk</title></head>
<body style="font-family:system-ui;max-width:600px;margin:80px auto;text-align:center">
<h1>KP-Gruuk</h1>
<p>Internal tunnel service for Kashport.</p>
<p>Install: <code>curl -sSL https://raw.githubusercontent.com/kashportsa/kp-gruuk/main/install.sh | sh</code></p>
<p>Usage: <code>gruuk expose 8080</code></p>
</body></html>`)
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}

// EmailToSubdomain converts an email to a valid subdomain label.
func EmailToSubdomain(email string) string {
	prefix := strings.Split(email, "@")[0]
	prefix = strings.ToLower(prefix)

	// Replace non-alphanumeric chars with hyphens
	var b strings.Builder
	for _, c := range prefix {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else {
			b.WriteRune('-')
		}
	}
	result := b.String()

	// Collapse consecutive hyphens
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}

	// Trim leading/trailing hyphens
	result = strings.Trim(result, "-")

	return result
}
