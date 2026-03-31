package client

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kashportsa/kp-gruuk/internal/tunnel"
)

// LocalProxy forwards tunnel requests to a local service.
type LocalProxy struct {
	targetPort int
	client     *http.Client
}

// NewLocalProxy creates a proxy that forwards to localhost on the given port.
func NewLocalProxy(port int) *LocalProxy {
	return &LocalProxy{
		targetPort: port,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Forward takes an HTTP request payload from the tunnel and forwards it to the local service,
// returning the response payload.
func (lp *LocalProxy) Forward(req *tunnel.HTTPRequestPayload) *tunnel.HTTPResponsePayload {
	body, err := tunnel.DecodeBody(req.Body)
	if err != nil {
		return errorResponse(502, "failed to decode request body")
	}

	targetURL := fmt.Sprintf("http://localhost:%d%s", lp.targetPort, req.Path)

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = io.NopCloser(newBytesReader(body))
	}

	httpReq, err := http.NewRequest(req.Method, targetURL, bodyReader)
	if err != nil {
		return errorResponse(502, "failed to create request")
	}

	for key, vals := range req.Headers {
		for _, v := range vals {
			httpReq.Header.Add(key, v)
		}
	}
	// Override host to match the original request
	httpReq.Host = httpReq.Header.Get("Host")

	resp, err := lp.client.Do(httpReq)
	if err != nil {
		return errorResponse(502, fmt.Sprintf("local service unavailable: %v", err))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB limit
	if err != nil {
		return errorResponse(502, "failed to read local response")
	}

	return &tunnel.HTTPResponsePayload{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       tunnel.EncodeBody(respBody),
	}
}

func errorResponse(status int, msg string) *tunnel.HTTPResponsePayload {
	return &tunnel.HTTPResponsePayload{
		StatusCode: status,
		Headers:    map[string][]string{"Content-Type": {"text/plain"}},
		Body:       tunnel.EncodeBody([]byte(msg)),
	}
}

type bytesReader struct {
	data []byte
	pos  int
}

func newBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data}
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
