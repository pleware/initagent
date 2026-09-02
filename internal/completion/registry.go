package completion

import (
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
