package core

import (
	"encoding/json"
	"sync"
	"time"
)

// Event is a domain event emitted by a game. CorrelationID links an event back
// to the command that caused it (empty for spontaneous events).
type Event struct {
	Type          string          `json:"type"`
	Game          string          `json:"game"`
	At            time.Time       `json:"at"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

// Command is an external instruction for a game.
type Command struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// CommandSpec describes a supported command for discovery.
type CommandSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema,omitempty"`
}

// Emitter publishes domain events.
type Emitter interface {
	Emit(Event)
}

// EmitterFunc adapts a function to the Emitter interface.
type EmitterFunc func(Event)

// Emit calls the underlying function.
func (f EmitterFunc) Emit(ev Event) { f(ev) }

// EmitterSetter is implemented by games that can receive an Emitter.
type EmitterSetter interface {
	SetEmitter(Emitter)
}

// Commandable is implemented by games that accept external commands.
type Commandable interface {
	HandleCommand(Command) error
	Commands() []CommandSpec
}

// MustJSON marshals v into json.RawMessage, dropping any error. It is a
// convenience for building event and command payloads from known-safe values.
func MustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// Bus fans events out to subscribers without blocking producers.
type Bus struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
}

// NewBus returns an empty event bus.
func NewBus() *Bus {
	return &Bus{subs: make(map[chan Event]struct{})}
}

// Emit sends the event to every subscriber. Subscribers with full buffers are
// skipped rather than blocking the producer.
func (b *Bus) Emit(ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// Subscribe returns a buffered channel that receives emitted events.
func (b *Bus) Subscribe() chan Event {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscription returned by Subscribe.
func (b *Bus) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	delete(b.subs, ch)
	b.mu.Unlock()
}
