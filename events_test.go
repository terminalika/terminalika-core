package core

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBusEmitsToSubscribers(t *testing.T) {
	b := NewBus()
	sub := b.Subscribe()
	defer b.Unsubscribe(sub)

	ev := Event{Type: "snake.moved", Game: "snake", At: time.Now()}
	b.Emit(ev)

	got := <-sub
	if got.Type != ev.Type || got.Game != ev.Game {
		t.Fatalf("got %+v, want type/game %s/%s", got, ev.Type, ev.Game)
	}
}

func TestBusUnsubscribeStopsDelivery(t *testing.T) {
	b := NewBus()
	sub := b.Subscribe()
	b.Unsubscribe(sub)

	b.Emit(Event{Type: "x", Game: "g", At: time.Now()})

	select {
	case ev := <-sub:
		t.Fatalf("received event after unsubscribe: %+v", ev)
	default:
	}
}

func TestBusSkipsSlowSubscribers(t *testing.T) {
	b := NewBus()

	// A subscriber that never reads: its buffer fills and events get dropped
	// without blocking the producer.
	sub := b.Subscribe()
	defer b.Unsubscribe(sub)

	for i := 0; i < 200; i++ {
		b.Emit(Event{Type: "x", Game: "g", At: time.Now()})
	}
	// If Emit blocked, this test would deadlock; reaching here is success.
}

func TestEmitterFuncAdaptsFunction(t *testing.T) {
	var got Event
	EmitterFunc(func(ev Event) { got = ev }).Emit(Event{Type: "a", Game: "g", At: time.Now()})

	if got.Type != "a" {
		t.Fatalf("got type %q, want a", got.Type)
	}
}

func TestMustJSON(t *testing.T) {
	raw := MustJSON(map[string]int{"x": 1})
	var m map[string]int
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["x"] != 1 {
		t.Fatalf("m = %v, want x=1", m)
	}
}
