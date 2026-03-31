package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kashportsa/kp-gruuk/internal/tunnel"
)

// Proxy handles incoming HTTP visitor requests and forwards them through a tunnel.
type Proxy struct {
	registry *Registry
	timeout  time.Duration
	logger   *slog.Logger
}

// NewProxy creates a new visitor request proxy.
func NewProxy(registry *Registry, timeout time.Duration, logger *slog.Logger) *Proxy {
	return &Proxy{
		registry: registry,
		timeout:  timeout,
		logger:   logger,
	}
}

// ServeHTTP proxies a visitor request through the tunnel to the developer's local service.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request, subdomain string) {
	conn, ok := p.registry.Lookup(subdomain)
	if !ok {
		http.Error(w, "tunnel not connected", http.StatusBadGateway)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10MB limit
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	requestID := uuid.New().String()

	reqPayload := &tunnel.HTTPRequestPayload{
		Method:  r.Method,
		Path:    r.URL.RequestURI(),
		Headers: r.Header,
		Body:    tunnel.EncodeBody(body),
	}

	env, err := tunnel.NewEnvelope(tunnel.TypeHTTPRequest, requestID, reqPayload)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	respCh, err := conn.Mux.Register(requestID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), p.timeout)
	defer cancel()

	if err := conn.Send(ctx, env); err != nil {
		conn.Mux.Cancel(requestID)
		http.Error(w, "tunnel write error", http.StatusBadGateway)
		return
	}

	start := time.Now()

	select {
	case resp, ok := <-respCh:
		if !ok || resp == nil {
			http.Error(w, "tunnel closed", http.StatusBadGateway)
			return
		}
		p.writeResponse(w, resp)
		p.logger.Info("proxied request",
			"subdomain", subdomain,
			"method", r.Method,
			"path", r.URL.Path,
			"status", resp.StatusCode,
			"duration", time.Since(start).Round(time.Millisecond),
		)
	case <-ctx.Done():
		conn.Mux.Cancel(requestID)
		http.Error(w, "gateway timeout", http.StatusGatewayTimeout)
	}
}

func (p *Proxy) writeResponse(w http.ResponseWriter, resp *tunnel.HTTPResponsePayload) {
	for key, vals := range resp.Headers {
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}

	body, err := tunnel.DecodeBody(resp.Body)
	if err != nil {
		http.Error(w, "failed to decode response body", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(resp.StatusCode)
	if len(body) > 0 {
		w.Write(body)
	}
}
