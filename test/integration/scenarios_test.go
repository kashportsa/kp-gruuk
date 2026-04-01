package integration

// Additional integration tests covering HTTP methods, headers, body forwarding,
// query strings, error scenarios, and multi-client behaviour.

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kashportsa/kp-gruuk/internal/client"
	"github.com/kashportsa/kp-gruuk/internal/server"
	"log/slog"
)

// tunnel sets up a gruuk server + connected tunnel client in one call.
// Returns the server port and an HTTP client pre-configured to send requests
// through the given subdomain.
func tunnel(t *testing.T, localApp *httptest.Server, email string, opts ...func(*server.Config)) (serverPort int, via *http.Client) {
	t.Helper()

	cfg := server.Config{
		Domain:     "test.local",
		ListenAddr: "127.0.0.1:0",
		SkipAuth:   true,
	}
	for _, o := range opts {
		o(&cfg)
	}

	// Pick a free port
	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		t.Fatal(err)
	}
	serverPort = ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	cfg.ListenAddr = fmt.Sprintf("127.0.0.1:%d", serverPort)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := server.New(cfg, logger)
	go srv.ListenAndServe()
	waitForServer(t, cfg.ListenAddr)

	// Extract local app port
	localAddr := localApp.Listener.Addr().String()
	var localPort int
	fmt.Sscanf(localAddr[strings.LastIndex(localAddr, ":")+1:], "%d", &localPort)

	proxy := client.NewLocalProxy(localPort)
	tc := client.NewTunnelClient(
		fmt.Sprintf("ws://127.0.0.1:%d", serverPort),
		"", proxy, logger,
	)
	tc.SetEmail(email)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	if err := tc.Connect(ctx); err != nil {
		t.Fatalf("tunnel connect: %v", err)
	}
	go tc.Run(ctx)
	time.Sleep(50 * time.Millisecond)

	subdomain := tc.Subdomain
	via = &http.Client{Timeout: 5 * time.Second}
	via.Transport = subdomain_transport(serverPort, subdomain)

	return serverPort, via
}

// subdomain_transport returns an http.Transport that injects the correct Host header.
func subdomain_transport(serverPort int, subdomain string) http.RoundTripper {
	return &subdomainRoundTripper{
		base:       http.DefaultTransport,
		serverPort: serverPort,
		subdomain:  subdomain,
	}
}

type subdomainRoundTripper struct {
	base       http.RoundTripper
	serverPort int
	subdomain  string
}

func (rt *subdomainRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Host = fmt.Sprintf("127.0.0.1:%d", rt.serverPort)
	req.URL.Scheme = "http"
	req.Host = fmt.Sprintf("%s.test.local", rt.subdomain)
	return rt.base.RoundTrip(req)
}

// --- Tests ---

func TestHTTPMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			localApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, r.Method)
			}))
			defer localApp.Close()

			_, via := tunnel(t, localApp, "method-test@kashport.com")

			req, _ := http.NewRequest(method, "http://placeholder/", nil)
			resp, err := via.Do(req)
			if err != nil {
				t.Fatalf("%s: %v", method, err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			if string(body) != method {
				t.Errorf("%s: expected method echoed back, got %q", method, string(body))
			}
		})
	}
}

func TestBodyForwardedRequestAndResponse(t *testing.T) {
	localApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "echo:%s", body)
	}))
	defer localApp.Close()

	_, via := tunnel(t, localApp, "body-test@kashport.com")

	req, _ := http.NewRequest("POST", "http://placeholder/", strings.NewReader("hello-body"))
	resp, err := via.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "echo:hello-body" {
		t.Fatalf("expected body echo, got %q", string(body))
	}
}

func TestQueryStringPreserved(t *testing.T) {
	localApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.URL.RawQuery)
	}))
	defer localApp.Close()

	_, via := tunnel(t, localApp, "query-test@kashport.com")

	resp, err := via.Get("http://placeholder/search?q=hello&page=3")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "q=hello&page=3" {
		t.Fatalf("query string not preserved, got %q", string(body))
	}
}

func TestRequestHeadersForwarded(t *testing.T) {
	localApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.Header.Get("X-Request-Id"))
	}))
	defer localApp.Close()

	_, via := tunnel(t, localApp, "req-header-test@kashport.com")

	req, _ := http.NewRequest("GET", "http://placeholder/", nil)
	req.Header.Set("X-Request-Id", "trace-123")
	resp, err := via.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "trace-123" {
		t.Fatalf("X-Request-Id not forwarded, got %q", string(body))
	}
}

func TestResponseHeadersForwarded(t *testing.T) {
	localApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Trace-Id", "resp-456")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer localApp.Close()

	_, via := tunnel(t, localApp, "resp-header-test@kashport.com")

	resp, err := via.Get("http://placeholder/")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Trace-Id"); got != "resp-456" {
		t.Errorf("X-Trace-Id not returned, got %q", got)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type not returned, got %q", ct)
	}
}

func TestStatusCodesPreserved(t *testing.T) {
	cases := []int{200, 201, 400, 401, 404, 500}
	for _, code := range cases {
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			localApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer localApp.Close()

			_, via := tunnel(t, localApp, fmt.Sprintf("status-%d@kashport.com", code))

			resp, err := via.Get("http://placeholder/")
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			resp.Body.Close()

			if resp.StatusCode != code {
				t.Errorf("expected %d, got %d", code, resp.StatusCode)
			}
		})
	}
}

func TestLocalServiceDown(t *testing.T) {
	// Use a server that immediately closes so the port is free when the tunnel tries to forward
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // nothing listening on this port

	// Create a fake localApp just to satisfy the tunnel helper signature
	fakeApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer fakeApp.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ln2, _ := net.Listen("tcp", "127.0.0.1:0")
	serverPort := ln2.Addr().(*net.TCPAddr).Port
	ln2.Close()

	srv := server.New(server.Config{
		Domain:     "test.local",
		ListenAddr: fmt.Sprintf("127.0.0.1:%d", serverPort),
		SkipAuth:   true,
	}, logger)
	go srv.ListenAndServe()
	waitForServer(t, fmt.Sprintf("127.0.0.1:%d", serverPort))

	// Connect tunnel client pointing at the closed port
	proxy := client.NewLocalProxy(port)
	tc := client.NewTunnelClient(
		fmt.Sprintf("ws://127.0.0.1:%d", serverPort),
		"", proxy, logger,
	)
	tc.SetEmail("down@kashport.com")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tc.Connect(ctx)
	go tc.Run(ctx)
	time.Sleep(50 * time.Millisecond)

	httpClient := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/", serverPort), nil)
	req.Host = "down.test.local"
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502 when local service is down, got %d", resp.StatusCode)
	}
}

func TestProxyTimeout(t *testing.T) {
	localApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond) // longer than proxy timeout
		fmt.Fprint(w, "too slow")
	}))
	defer localApp.Close()

	withShortTimeout := func(cfg *server.Config) {
		cfg.ProxyTimeout = 100 * time.Millisecond
	}

	_, via := tunnel(t, localApp, "timeout-test@kashport.com", withShortTimeout)

	resp, err := via.Get("http://placeholder/slow")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", resp.StatusCode)
	}
}

func TestSubdomainConflict(t *testing.T) {
	localApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer localApp.Close()

	localAddr := localApp.Listener.Addr().String()
	var localPort int
	fmt.Sscanf(localAddr[strings.LastIndex(localAddr, ":")+1:], "%d", &localPort)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	serverPort := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	srv := server.New(server.Config{
		Domain:     "test.local",
		ListenAddr: fmt.Sprintf("127.0.0.1:%d", serverPort),
		SkipAuth:   true,
	}, logger)
	go srv.ListenAndServe()
	waitForServer(t, fmt.Sprintf("127.0.0.1:%d", serverPort))

	connectClient := func() error {
		proxy := client.NewLocalProxy(localPort)
		tc := client.NewTunnelClient(
			fmt.Sprintf("ws://127.0.0.1:%d", serverPort),
			"", proxy, logger,
		)
		tc.SetEmail("conflict@kashport.com")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return tc.Connect(ctx)
	}

	// First connection should succeed
	if err := connectClient(); err != nil {
		t.Fatalf("first connect: %v", err)
	}

	// Second connection with same email should fail with subdomain_taken
	err := connectClient()
	if err == nil {
		t.Fatal("expected error for duplicate subdomain, got nil")
	}
	if !strings.Contains(err.Error(), "subdomain_taken") {
		t.Fatalf("expected subdomain_taken error, got %q", err.Error())
	}
}

func TestMultipleSimultaneousClients(t *testing.T) {
	const numClients = 5

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	serverPort := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	srv := server.New(server.Config{
		Domain:     "test.local",
		ListenAddr: fmt.Sprintf("127.0.0.1:%d", serverPort),
		SkipAuth:   true,
	}, logger)
	go srv.ListenAndServe()
	waitForServer(t, fmt.Sprintf("127.0.0.1:%d", serverPort))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errors := make(chan error, numClients)

	for i := range numClients {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			localApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, "client-%d", i)
			}))
			defer localApp.Close()

			localAddr := localApp.Listener.Addr().String()
			var localPort int
			fmt.Sscanf(localAddr[strings.LastIndex(localAddr, ":")+1:], "%d", &localPort)

			proxy := client.NewLocalProxy(localPort)
			tc := client.NewTunnelClient(
				fmt.Sprintf("ws://127.0.0.1:%d", serverPort),
				"", proxy, logger,
			)
			tc.SetEmail(fmt.Sprintf("client%d@kashport.com", i))

			if err := tc.Connect(ctx); err != nil {
				errors <- fmt.Errorf("client %d connect: %w", i, err)
				return
			}
			go tc.Run(ctx)
			time.Sleep(50 * time.Millisecond)

			// Make a request through this client's tunnel
			httpClient := &http.Client{Timeout: 5 * time.Second}
			req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/", serverPort), nil)
			req.Host = fmt.Sprintf("%s.test.local", tc.Subdomain)

			resp, err := httpClient.Do(req)
			if err != nil {
				errors <- fmt.Errorf("client %d request: %w", i, err)
				return
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			expected := fmt.Sprintf("client-%d", i)
			if string(body) != expected {
				errors <- fmt.Errorf("client %d: expected %q, got %q", i, expected, string(body))
			}
		}(i)
	}

	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}

func TestPathPreserved(t *testing.T) {
	localApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.URL.Path)
	}))
	defer localApp.Close()

	_, via := tunnel(t, localApp, "path-test@kashport.com")

	resp, err := via.Get("http://placeholder/api/v1/users/42")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "/api/v1/users/42" {
		t.Fatalf("path not preserved, got %q", string(body))
	}
}

func TestLargeResponseBody(t *testing.T) {
	const size = 512 * 1024 // 512KB
	payload := strings.Repeat("x", size)

	localApp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, payload)
	}))
	defer localApp.Close()

	_, via := tunnel(t, localApp, "large-resp@kashport.com")

	resp, err := via.Get("http://placeholder/large")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if len(body) != size {
		t.Fatalf("expected %d bytes, got %d", size, len(body))
	}
}
