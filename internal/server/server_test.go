package server

import "testing"

func TestEmailToSubdomain(t *testing.T) {
	tests := []struct {
		email    string
		expected string
	}{
		{"juan@kashport.com", "juan"},
		{"juan.rodriguez@kashport.com", "juan-rodriguez"},
		{"Juan_Test@kashport.com", "juan-test"},
		{"user.name.long@kashport.com", "user-name-long"},
		{"UPPER@kashport.com", "upper"},
		{"a..b@kashport.com", "a-b"},
		{"test123@kashport.com", "test123"},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			got := EmailToSubdomain(tt.email)
			if got != tt.expected {
				t.Fatalf("EmailToSubdomain(%q) = %q, want %q", tt.email, got, tt.expected)
			}
		})
	}
}
