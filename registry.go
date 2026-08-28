package core

import (
	"sort"
	"sync"
)

// Registry stores game factories by name and is safe for concurrent use.
type Registry struct {
	mu    sync.RWMutex
	games map[string]Factory
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{games: make(map[string]Factory)}
}

// Register adds a game factory under the given name.
func (r *Registry) Register(name string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.games[name] = factory
}

// Get returns a fresh instance of the named game.
func (r *Registry) Get(name string) (Game, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.games[name]
	if !ok {
		return nil, false
	}
	return factory(), true
}

// Has reports whether a game is registered under the given name.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.games[name]
	return ok
}

// Names returns the registered game names in sorted order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.games))
	for name := range r.games {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
