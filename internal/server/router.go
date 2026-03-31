package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/kashportsa/kp-gruuk/internal/tunnel"
	"nhooyr.io/websocket"
)

// TunnelConn represents an active tunnel connection from a CLI client.
type TunnelConn struct {
	Subdomain string
	WS        *websocket.Conn
	Mux       *tunnel.Mux
	mu        sync.Mutex // serializes writes to WS
}

// Send writes an envelope to the WebSocket connection.
func (tc *TunnelConn) Send(ctx context.Context, env *tunnel.Envelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.WS.Write(ctx, websocket.MessageText, data)
}

// Registry maps subdomains to active tunnel connections.
type Registry struct {
	mu      sync.RWMutex
	tunnels map[string]*TunnelConn
	logger  *slog.Logger
}

// NewRegistry creates a new tunnel registry.
func NewRegistry(logger *slog.Logger) *Registry {
	return &Registry{
		tunnels: make(map[string]*TunnelConn),
		logger:  logger,
	}
}

// Register adds a tunnel connection for the given subdomain. Returns error if already taken.
func (r *Registry) Register(subdomain string, conn *TunnelConn) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tunnels[subdomain]; exists {
		return fmt.Errorf("subdomain %q is already in use", subdomain)
	}

	r.tunnels[subdomain] = conn
	r.logger.Info("tunnel registered", "subdomain", subdomain)
	return nil
}

// Unregister removes a tunnel connection.
func (r *Registry) Unregister(subdomain string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tunnels, subdomain)
	r.logger.Info("tunnel unregistered", "subdomain", subdomain)
}

// Lookup finds a tunnel connection by subdomain.
func (r *Registry) Lookup(subdomain string) (*TunnelConn, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conn, ok := r.tunnels[subdomain]
	return conn, ok
}
