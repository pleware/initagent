package completion

import (
	"context"
	"testing"
)

// mockResolver is a test-only resolver.
type mockResolver struct {
	name string
}

func (m *mockResolver) Name() string {
	return m.name
}

func (m *mockResolver) Supports(mode LaunchMode) bool {
	return true
}

func (m *mockResolver) Watch(ctx context.Context, run RunContext) (<-chan Outcome, error) {
	out := make(chan Outcome, 1)
	close(out)
	return out, nil
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	mock := &mockResolver{name: "test"}

	r.Register(mock)

	got := r.Get("test")
	if got == nil {
		t.Fatal("expected resolver, got nil")
	}
	if got.Name() != "test" {
		t.Fatalf("name = %q, want %q", got.Name(), "test")
	}
}

func TestRegistry_RegisterDuplicatePanics(t *testing.T) {
	r := NewRegistry()
	mock := &mockResolver{name: "duplicate"}

	r.Register(mock)

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for duplicate registration, got none")
		}
	}()
	r.Register(mock)
}

func TestRegistry_GetNonExistent(t *testing.T) {
	r := NewRegistry()
	got := r.Get("nonexistent")
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestRegistry_All(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockResolver{name: "a"})
	r.Register(&mockResolver{name: "b"})

	all := r.All()
	if len(all) != 2 {
		t.Fatalf("len(all) = %d, want 2", len(all))
	}

	names := make(map[string]bool)
	for _, resolver := range all {
		names[resolver.Name()] = true
	}
	if !names["a"] || !names["b"] {
		t.Fatalf("missing expected resolvers, got: %v", names)
	}
}

func TestRegistry_Names(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockResolver{name: "x"})
	r.Register(&mockResolver{name: "y"})

	names := r.Names()
	if len(names) != 2 {
		t.Fatalf("len(names) = %d, want 2", len(names))
	}

	nameSet := make(map[string]bool)
	for _, name := range names {
		nameSet[name] = true
	}
	if !nameSet["x"] || !nameSet["y"] {
		t.Fatalf("missing expected names, got: %v", names)
	}
}

func TestDefaultRegistry(t *testing.T) {
	// Default registry should have built-in resolvers
	names := Default.Names()
	if len(names) == 0 {
		t.Fatal("default registry is empty (expected built-in resolvers)")
	}

	// Check for expected built-in resolvers
	process := Default.Get("process")
	if process == nil {
		t.Error("expected 'process' resolver in default registry")
	}

	file := Default.Get("file")
	if file == nil {
		t.Error("expected 'file' resolver in default registry")
	}
}
