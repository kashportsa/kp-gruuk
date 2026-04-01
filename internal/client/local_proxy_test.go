package client

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kashportsa/kp-gruuk/internal/tunnel"
)

func TestLocalProxyGET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "hello")
	}))
	defer srv.Close()
	proxy := localProxyFor(t, srv)

	resp := proxy.Forward(&tunnel.HTTPRequestPayload{
		Method: "GET",
		Path:   "/",
	})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := tunnel.DecodeBody(resp.Body)
	if string(body) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(body))
	}
}

func TestLocalProxyPOSTWithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "received: %s", body)
	}))
	defer srv.Close()
	proxy := localProxyFor(t, srv)

	resp := proxy.Forward(&tunnel.HTTPRequestPayload{
		Method: "POST",
		Path:   "/submit",
		Body:   tunnel.EncodeBody([]byte("payload=data")),
	})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := tunnel.DecodeBody(resp.Body)
	if !strings.Contains(string(body), "payload=data") {
		t.Fatalf("body not forwarded, got %q", string(body))
	}
}

func TestLocalProxyQueryStringPreserved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.URL.RawQuery)
	}))
	defer srv.Close()
	proxy := localProxyFor(t, srv)

	resp := proxy.Forward(&tunnel.HTTPRequestPayload{
		Method: "GET",
		Path:   "/search?q=gruuk&page=2",
	})

	body, _ := tunnel.DecodeBody(resp.Body)
	if string(body) != "q=gruuk&page=2" {
		t.Fatalf("query string not preserved, got %q", string(body))
	}
}

func TestLocalProxyRequestHeadersForwarded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Got-Header", r.Header.Get("X-Custom"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	proxy := localProxyFor(t, srv)

	resp := proxy.Forward(&tunnel.HTTPRequestPayload{
		Method: "GET",
		Path:   "/",
		Headers: http.Header{
			"X-Custom": {"my-value"},
		},
	})

	if got := resp.Headers["X-Got-Header"]; len(got) == 0 || got[0] != "my-value" {
		t.Fatalf("request header not forwarded, got %v", got)
	}
}

func TestLocalProxyResponseHeadersReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Response-ID", "abc123")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()
	proxy := localProxyFor(t, srv)

	resp := proxy.Forward(&tunnel.HTTPRequestPayload{Method: "GET", Path: "/"})

	if got := resp.Headers["X-Response-Id"]; len(got) == 0 || got[0] != "abc123" {
		t.Fatalf("X-Response-ID not returned, got %v", resp.Headers)
	}
	if ct := resp.Headers["Content-Type"]; len(ct) == 0 || ct[0] != "application/json" {
		t.Fatalf("Content-Type not returned, got %v", resp.Headers)
	}
}

func TestLocalProxyStatusCodes(t *testing.T) {
	cases := []int{200, 201, 204, 400, 401, 403, 404, 500, 503}
	for _, code := range cases {
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer srv.Close()
			proxy := localProxyFor(t, srv)

			resp := proxy.Forward(&tunnel.HTTPRequestPayload{Method: "GET", Path: "/"})
			if resp.StatusCode != code {
				t.Fatalf("expected %d, got %d", code, resp.StatusCode)
			}
		})
	}
}

func TestLocalProxyServiceUnavailable(t *testing.T) {
	// Point at a port with nothing listening
	proxy := NewLocalProxy(19999)

	resp := proxy.Forward(&tunnel.HTTPRequestPayload{Method: "GET", Path: "/"})

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502 when service is down, got %d", resp.StatusCode)
	}
}

func TestLocalProxyEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "len=%d", len(body))
	}))
	defer srv.Close()
	proxy := localProxyFor(t, srv)

	resp := proxy.Forward(&tunnel.HTTPRequestPayload{Method: "GET", Path: "/"})

	body, _ := tunnel.DecodeBody(resp.Body)
	if string(body) != "len=0" {
		t.Fatalf("expected empty body, got %q", string(body))
	}
}

func TestLocalProxyHTTPMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, r.Method)
			}))
			defer srv.Close()
			proxy := localProxyFor(t, srv)

			resp := proxy.Forward(&tunnel.HTTPRequestPayload{Method: method, Path: "/"})
			body, _ := tunnel.DecodeBody(resp.Body)
			if string(body) != method {
				t.Fatalf("expected method %q echoed back, got %q", method, string(body))
			}
		})
	}
}

func TestLocalProxyLargeBody(t *testing.T) {
	const size = 1 << 20 // 1MB

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "received=%d", len(body))
	}))
	defer srv.Close()
	proxy := localProxyFor(t, srv)

	largeBody := make([]byte, size)
	for i := range largeBody {
		largeBody[i] = 'x'
	}

	resp := proxy.Forward(&tunnel.HTTPRequestPayload{
		Method: "POST",
		Path:   "/upload",
		Body:   tunnel.EncodeBody(largeBody),
	})

	body, _ := tunnel.DecodeBody(resp.Body)
	if string(body) != fmt.Sprintf("received=%d", size) {
		t.Fatalf("large body not forwarded correctly, got %q", string(body))
	}
}

// localProxyFor extracts the port from a test server and returns a LocalProxy.
func localProxyFor(t *testing.T, srv *httptest.Server) *LocalProxy {
	t.Helper()
	addr := srv.Listener.Addr().String()
	var port int
	fmt.Sscanf(addr[strings.LastIndex(addr, ":")+1:], "%d", &port)
	return NewLocalProxy(port)
}
