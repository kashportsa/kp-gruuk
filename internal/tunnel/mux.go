package tunnel

import (
	"fmt"
	"sync"
	"time"
)

// Mux tracks in-flight HTTP requests through the tunnel, mapping request IDs to response channels.
type Mux struct {
	mu        sync.Mutex
	pending   map[string]*pendingRequest
	timeout   time.Duration
	done      chan struct{}
	closeOnce sync.Once
}

type pendingRequest struct {
	ch        chan *HTTPResponsePayload
	createdAt time.Time
}

// NewMux creates a new request multiplexer with the given timeout.
func NewMux(timeout time.Duration) *Mux {
	m := &Mux{
		pending: make(map[string]*pendingRequest),
		timeout: timeout,
		done:    make(chan struct{}),
	}
	go m.cleanupLoop()
	return m
}

// Register creates a response channel for the given request ID.
func (m *Mux) Register(requestID string) (<-chan *HTTPResponsePayload, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.pending[requestID]; exists {
		return nil, fmt.Errorf("request %s already registered", requestID)
	}

	ch := make(chan *HTTPResponsePayload, 1)
	m.pending[requestID] = &pendingRequest{
		ch:        ch,
		createdAt: time.Now(),
	}
	return ch, nil
}

// Resolve delivers a response for the given request ID and removes it from tracking.
func (m *Mux) Resolve(requestID string, resp *HTTPResponsePayload) bool {
	m.mu.Lock()
	pr, ok := m.pending[requestID]
	if ok {
		delete(m.pending, requestID)
	}
	m.mu.Unlock()

	if !ok {
		return false
	}

	pr.ch <- resp
	return true
}

// Cancel removes a pending request without delivering a response.
func (m *Mux) Cancel(requestID string) {
	m.mu.Lock()
	pr, ok := m.pending[requestID]
	if ok {
		delete(m.pending, requestID)
		close(pr.ch)
	}
	m.mu.Unlock()
}

// CancelAll closes all pending request channels.
func (m *Mux) CancelAll() {
	m.mu.Lock()
	for id, pr := range m.pending {
		close(pr.ch)
		delete(m.pending, id)
	}
	m.mu.Unlock()
}

// Close stops the cleanup goroutine and cancels all pending requests.
// Safe to call multiple times.
func (m *Mux) Close() {
	m.closeOnce.Do(func() { close(m.done) })
	m.CancelAll()
}

func (m *Mux) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.cleanExpired()
		}
	}
}

func (m *Mux) cleanExpired() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, pr := range m.pending {
		if now.Sub(pr.createdAt) > m.timeout {
			close(pr.ch)
			delete(m.pending, id)
		}
	}
}
