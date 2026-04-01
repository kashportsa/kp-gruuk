package server

import (
	"io"
	"log/slog"
	"sync"
	"testing"
)

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	return NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestRegistryRegisterAndLookup(t *testing.T) {
	r := newTestRegistry(t)
	conn := &TunnelConn{Subdomain: "alice"}

	if err := r.Register("alice", conn); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := r.Lookup("alice")
	if !ok {
		t.Fatal("Lookup: expected tunnel, got nothing")
	}
	if got != conn {
		t.Fatal("Lookup: returned wrong connection")
	}
}

func TestRegistryLookupMissing(t *testing.T) {
	r := newTestRegistry(t)
	_, ok := r.Lookup("nobody")
	if ok {
		t.Fatal("expected Lookup to return false for unknown subdomain")
	}
}

func TestRegistryDuplicateSubdomain(t *testing.T) {
	r := newTestRegistry(t)

	if err := r.Register("alice", &TunnelConn{Subdomain: "alice"}); err != nil {
		t.Fatalf("first Register: %v", err)
	}

	if err := r.Register("alice", &TunnelConn{Subdomain: "alice"}); err == nil {
		t.Fatal("expected error on duplicate subdomain, got nil")
	}
}

func TestRegistryUnregister(t *testing.T) {
	r := newTestRegistry(t)
	r.Register("alice", &TunnelConn{Subdomain: "alice"})

	r.Unregister("alice")

	_, ok := r.Lookup("alice")
	if ok {
		t.Fatal("expected Lookup to return false after Unregister")
	}
}

func TestRegistryUnregisterNonExistent(t *testing.T) {
	r := newTestRegistry(t)
	// Should not panic
	r.Unregister("nobody")
}

func TestRegistryConcurrent(t *testing.T) {
	r := newTestRegistry(t)
	const n = 50

	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			sub := "user"
			conn := &TunnelConn{Subdomain: sub}
			// Register may fail (duplicate), that's fine — we're testing for races
			_ = r.Register(sub+string(rune('a'+i)), conn)
		}(i)
	}
	wg.Wait()

	// All lookups should return consistent results
	for i := range n {
		sub := "user" + string(rune('a'+i))
		conn, ok := r.Lookup(sub)
		if ok && conn == nil {
			t.Errorf("Lookup(%q) returned ok but nil conn", sub)
		}
	}
}
