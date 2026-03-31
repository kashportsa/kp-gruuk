package tunnel

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// Message types exchanged over the WebSocket tunnel.
const (
	TypeHTTPRequest  = "http_request"
	TypeHTTPResponse = "http_response"
	TypePing         = "ping"
	TypePong         = "pong"
	TypeError        = "error"
	TypeConnected    = "connected"
)

// Envelope wraps every message sent through the tunnel.
type Envelope struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// HTTPRequestPayload is sent from server to client when a visitor makes an HTTP request.
type HTTPRequestPayload struct {
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Headers map[string][]string `json:"headers"`
	Body    string              `json:"body,omitempty"` // base64-encoded
}

// HTTPResponsePayload is sent from client to server with the local service's response.
type HTTPResponsePayload struct {
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body,omitempty"` // base64-encoded
}

// ConnectedPayload is sent from server to client after successful authentication.
type ConnectedPayload struct {
	Subdomain string `json:"subdomain"`
	PublicURL string `json:"public_url"`
}

// ErrorPayload carries error information.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewEnvelope creates an Envelope with a typed payload.
func NewEnvelope(msgType, requestID string, payload any) (*Envelope, error) {
	var raw json.RawMessage
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		raw = data
	}
	return &Envelope{
		Type:      msgType,
		RequestID: requestID,
		Payload:   raw,
	}, nil
}

// DecodePayload unmarshals the envelope payload into the given type.
func (e *Envelope) DecodePayload(v any) error {
	if e.Payload == nil {
		return fmt.Errorf("no payload")
	}
	return json.Unmarshal(e.Payload, v)
}

// EncodeBody base64-encodes raw bytes for transport in a payload.
func EncodeBody(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}

// DecodeBody decodes a base64-encoded body string.
func DecodeBody(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(encoded)
}
