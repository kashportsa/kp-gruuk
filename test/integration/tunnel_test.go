package integration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kashportsa/kp-gruuk/internal/client"
	"github.com/kashportsa/kp-gruuk/internal/server"
)

func TestTunnelEndToEnd(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Start a local "app" server that the tunnel should proxy to
	localApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Local-App", "true")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "hello from local app: %s %s", r.Method, r.URL.Path)
	}))
	defer localApp.Close()

	// Extract local app port
	localPort := localApp.Listener.Addr().(*net.TCPAddr).Port

	// Start the gruuk server on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serverPort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	srv := server.New(server.Config{
		Domain:     "test.local",
		ListenAddr: fmt.Sprintf("127.0.0.1:%d", serverPort),
		SkipAuth:   true,
	}, logger)

	go srv.ListenAndServe()

	// Wait for server to start
	waitForServer(t, fmt.Sprintf("127.0.0.1:%d", serverPort))

	// Connect the tunnel client
	serverURL := fmt.Sprintf("ws://127.0.0.1:%d", serverPort)
	localProxy := client.NewLocalProxy(localPort)
	tunnelClient := client.NewTunnelClient(
		serverURL,
		"",
		localProxy,
		logger,
	)
	tunnelClient.SetEmail("test@kashport.com")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := tunnelClient.Connect(ctx); err != nil {
		t.Fatalf("tunnel connect: %v", err)
	}

	if tunnelClient.Subdomain != "test" {
		t.Fatalf("expected subdomain 'test', got %q", tunnelClient.Subdomain)
	}

	// Start the tunnel message loop in background
	go tunnelClient.Run(ctx)

	// Give the tunnel a moment to stabilize
	time.Sleep(100 * time.Millisecond)

	// Make a request through the tunnel
	httpClient := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/hello", serverPort), nil)
	req.Host = "test.test.local" // subdomain.domain

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	expected := "hello from local app: GET /api/hello"
	if string(body) != expected {
		t.Fatalf("expected %q, got %q", expected, string(body))
	}

	if resp.Header.Get("X-Local-App") != "true" {
		t.Fatal("expected X-Local-App header")
	}

	t.Log("tunnel end-to-end test passed")
}

func TestConcurrentRequests(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	localApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate some processing time
		time.Sleep(50 * time.Millisecond)
		fmt.Fprintf(w, "path: %s", r.URL.Path)
	}))
	defer localApp.Close()

	localPort := localApp.Listener.Addr().(*net.TCPAddr).Port

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serverPort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	srv := server.New(server.Config{
		Domain:     "test.local",
		ListenAddr: fmt.Sprintf("127.0.0.1:%d", serverPort),
		SkipAuth:   true,
	}, logger)

	go srv.ListenAndServe()
	waitForServer(t, fmt.Sprintf("127.0.0.1:%d", serverPort))

	serverURL := fmt.Sprintf("ws://127.0.0.1:%d", serverPort)
	localProxy := client.NewLocalProxy(localPort)
	tunnelClient := client.NewTunnelClient(
		serverURL,
		"",
		localProxy,
		logger,
	)
	tunnelClient.SetEmail("concurrent@kashport.com")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := tunnelClient.Connect(ctx); err != nil {
		t.Fatalf("tunnel connect: %v", err)
	}

	go tunnelClient.Run(ctx)
	time.Sleep(100 * time.Millisecond)

	// Send 10 concurrent requests
	const numRequests = 10
	var wg sync.WaitGroup
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			httpClient := &http.Client{Timeout: 10 * time.Second}
			req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/request/%d", serverPort, idx), nil)
			req.Host = "concurrent.test.local"

			resp, err := httpClient.Do(req)
			if err != nil {
				errors <- fmt.Errorf("request %d: %w", idx, err)
				return
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			expected := fmt.Sprintf("path: /request/%d", idx)
			if !strings.Contains(string(body), expected) {
				errors <- fmt.Errorf("request %d: expected %q, got %q", idx, expected, string(body))
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

func TestTunnelNotConnected(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serverPort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	srv := server.New(server.Config{
		Domain:     "test.local",
		ListenAddr: fmt.Sprintf("127.0.0.1:%d", serverPort),
		SkipAuth:   true,
	}, logger)

	go srv.ListenAndServe()
	waitForServer(t, fmt.Sprintf("127.0.0.1:%d", serverPort))

	// Request to a subdomain with no tunnel connected
	httpClient := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/", serverPort), nil)
	req.Host = "nobody.test.local"

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", resp.StatusCode)
	}
}

func waitForServer(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server at %s did not start in time", addr)
}
