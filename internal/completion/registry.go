package completion

import (
	"context"
	"fmt"
	"sync"
)

// Registry holds all registered completion resolvers.
// It is safe for concurrent use.
type Registry struct {
	mu        sync.RWMutex
	resolvers map[string]Resolver
}

// NewRegistry creates an empty resolver registry.
func NewRegistry() *Registry {
	return &Registry{
		resolvers: make(map[string]Resolver),
	}
}

// Register adds a resolver to the registry.
// If a resolver with the same name already exists, Register panics.
func (r *Registry) Register(resolver Resolver) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := resolver.Name()
	if _, exists := r.resolvers[name]; exists {
		panic(fmt.Sprintf("completion: resolver %q already registered", name))
	}
	r.resolvers[name] = resolver
}

// Get retrieves a resolver by name.
// Returns nil if not found.
func (r *Registry) Get(name string) Resolver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.resolvers[name]
}

// All returns a copy of all registered resolvers.
func (r *Registry) All() []Resolver {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Resolver, 0, len(r.resolvers))
	for _, resolver := range r.resolvers {
		result = append(result, resolver)
	}
	return result
}

// Names returns the names of all registered resolvers.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.resolvers))
	for name := range r.resolvers {
		names = append(names, name)
	}
	return names
}

// Resolve runs the resolvers that support run.LaunchMode and returns the
// first completion Outcome. A resolver whose Watch cannot work on this run
// (missing pid, sentinel dir, or exec result) reports an error and is
// dropped, matching 12's "narrowed by worker capability" rule.
//
// This is the Milestone 0 shape: one resolvable signal per run, so the
// resolvers are tried sequentially and the first outcome wins. Fan-in
// arbitration over concurrent watchers ("first high trust wins") lands when
// a second resolver can fire on the same run.
func (r *Registry) Resolve(ctx context.Context, run RunContext) (Outcome, error) {
	for _, resolver := range r.All() {
		if !resolver.Supports(run.LaunchMode) {
			continue
		}
		ch, err := resolver.Watch(ctx, run)
		if err != nil {
			continue
		}
		select {
		case outcome, ok := <-ch:
			if ok && outcome.Done {
				return outcome, nil
			}
		case <-ctx.Done():
			return Outcome{}, ctx.Err()
		}
	}
	return Outcome{}, fmt.Errorf("completion: no resolver reported completion for run %s", run.RunID)
}

// Default is the global resolver registry.
// Implementations register themselves via init().
var Default = NewRegistry()

// Register adds a resolver to the default registry.
func Register(resolver Resolver) {
	Default.Register(resolver)
}

// Get retrieves a resolver from the default registry.
func Get(name string) Resolver {
	return Default.Get(name)
}
