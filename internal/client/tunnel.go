package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/kashportsa/kp-gruuk/internal/tunnel"
	"nhooyr.io/websocket"
)

// TunnelClient manages the WebSocket connection to the gruuk server.
type TunnelClient struct {
	serverURL  string
	token      string
	email      string // used in skip-auth mode (sent as X-Email header)
	localProxy *LocalProxy
	logger     *slog.Logger
	ws         *websocket.Conn
	mu         sync.Mutex // serializes writes

	// Set after successful connection
	Subdomain string
	PublicURL string
}

// NewTunnelClient creates a new tunnel client.
func NewTunnelClient(serverURL, token string, localProxy *LocalProxy, logger *slog.Logger) *TunnelClient {
	return &TunnelClient{
		serverURL:  serverURL,
		token:      token,
		localProxy: localProxy,
		logger:     logger,
	}
}

// SetEmail sets the email for skip-auth mode (sent as X-Email header).
func (tc *TunnelClient) SetEmail(email string) {
	tc.email = email
}

// Connect dials the server and performs the initial handshake.
func (tc *TunnelClient) Connect(ctx context.Context) error {
	headers := http.Header{}
	if tc.token != "" {
		headers.Set("Authorization", "Bearer "+tc.token)
	}
	if tc.email != "" {
		headers.Set("X-Email", tc.email)
	}

	ws, _, err := websocket.Dial(ctx, tc.serverURL+"/_ws/connect", &websocket.DialOptions{
		HTTPHeader: headers,
	})
	if err != nil {
		return fmt.Errorf("dial server: %w", err)
	}

	ws.SetReadLimit(16 << 20) // 16MB
	tc.ws = ws

	// Wait for connected message
	_, data, err := ws.Read(ctx)
	if err != nil {
		ws.Close(websocket.StatusAbnormalClosure, "")
		return fmt.Errorf("read connected message: %w", err)
	}

	var env tunnel.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		ws.Close(websocket.StatusAbnormalClosure, "")
		return fmt.Errorf("unmarshal connected message: %w", err)
	}

	if env.Type == tunnel.TypeError {
		var errPayload tunnel.ErrorPayload
		env.DecodePayload(&errPayload)
		ws.Close(websocket.StatusNormalClosure, "")
		return fmt.Errorf("server error: %s - %s", errPayload.Code, errPayload.Message)
	}

	if env.Type != tunnel.TypeConnected {
		ws.Close(websocket.StatusAbnormalClosure, "")
		return fmt.Errorf("unexpected message type: %s", env.Type)
	}

	var connPayload tunnel.ConnectedPayload
	if err := env.DecodePayload(&connPayload); err != nil {
		ws.Close(websocket.StatusAbnormalClosure, "")
		return fmt.Errorf("decode connected payload: %w", err)
	}

	tc.Subdomain = connPayload.Subdomain
	tc.PublicURL = connPayload.PublicURL

	return nil
}

// Run starts the message loop, processing incoming requests until the context is cancelled or an error occurs.
func (tc *TunnelClient) Run(ctx context.Context) error {
	for {
		_, data, err := tc.ws.Read(ctx)
		if err != nil {
			return fmt.Errorf("tunnel read: %w", err)
		}

		var env tunnel.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			tc.logger.Warn("invalid message from server", "error", err)
			continue
		}

		switch env.Type {
		case tunnel.TypeHTTPRequest:
			go tc.handleRequest(ctx, &env)

		case tunnel.TypePing:
			pongEnv, _ := tunnel.NewEnvelope(tunnel.TypePong, "", nil)
			tc.send(ctx, pongEnv)

		case tunnel.TypeError:
			var errPayload tunnel.ErrorPayload
			env.DecodePayload(&errPayload)
			tc.logger.Error("server error", "code", errPayload.Code, "message", errPayload.Message)

		default:
			tc.logger.Warn("unexpected message type", "type", env.Type)
		}
	}
}

func (tc *TunnelClient) handleRequest(ctx context.Context, env *tunnel.Envelope) {
	var req tunnel.HTTPRequestPayload
	if err := env.DecodePayload(&req); err != nil {
		tc.logger.Warn("invalid request payload", "error", err)
		return
	}

	start := time.Now()
	resp := tc.localProxy.Forward(&req)
	duration := time.Since(start).Round(time.Millisecond)

	tc.logger.Info("request",
		"method", req.Method,
		"path", req.Path,
		"status", resp.StatusCode,
		"duration", duration,
	)

	respEnv, err := tunnel.NewEnvelope(tunnel.TypeHTTPResponse, env.RequestID, resp)
	if err != nil {
		tc.logger.Error("failed to create response envelope", "error", err)
		return
	}

	if err := tc.send(ctx, respEnv); err != nil {
		tc.logger.Error("failed to send response", "error", err)
	}
}

func (tc *TunnelClient) send(ctx context.Context, env *tunnel.Envelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.ws.Write(ctx, websocket.MessageText, data)
}

// Close gracefully closes the tunnel connection.
func (tc *TunnelClient) Close() error {
	if tc.ws != nil {
		return tc.ws.Close(websocket.StatusNormalClosure, "client shutting down")
	}
	return nil
}
