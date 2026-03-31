package tunnel

import (
	"encoding/json"
	"testing"
)

func TestEnvelopeRoundtrip(t *testing.T) {
	req := &HTTPRequestPayload{
		Method:  "POST",
		Path:    "/api/users",
		Headers: map[string][]string{"Content-Type": {"application/json"}},
		Body:    EncodeBody([]byte(`{"name":"test"}`)),
	}

	env, err := NewEnvelope(TypeHTTPRequest, "req-123", req)
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.Type != TypeHTTPRequest {
		t.Fatalf("expected type %q, got %q", TypeHTTPRequest, decoded.Type)
	}
	if decoded.RequestID != "req-123" {
		t.Fatalf("expected request_id 'req-123', got %q", decoded.RequestID)
	}

	var decodedReq HTTPRequestPayload
	if err := decoded.DecodePayload(&decodedReq); err != nil {
		t.Fatal(err)
	}

	if decodedReq.Method != "POST" {
		t.Fatalf("expected method POST, got %q", decodedReq.Method)
	}

	body, err := DecodeBody(decodedReq.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"name":"test"}` {
		t.Fatalf("body mismatch: %q", string(body))
	}
}

func TestEncodeDecodeBody(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"empty", nil},
		{"text", []byte("hello world")},
		{"binary", []byte{0x00, 0xFF, 0x01, 0xFE}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeBody(tt.input)
			decoded, err := DecodeBody(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if len(tt.input) == 0 && len(decoded) == 0 {
				return // both nil/empty
			}
			if string(decoded) != string(tt.input) {
				t.Fatalf("mismatch: got %v, want %v", decoded, tt.input)
			}
		})
	}
}

func TestEnvelopeNoPayload(t *testing.T) {
	env, err := NewEnvelope(TypePing, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	if env.Type != TypePing {
		t.Fatalf("expected type %q, got %q", TypePing, env.Type)
	}

	if env.Payload != nil {
		t.Fatal("expected nil payload")
	}
}
