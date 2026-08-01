// Package entity is the engine's entity system. RE2 is built around
// discrete actors (player, zombies, items, triggers) that tick once per
// simulation step. The system is intentionally tiny: each Entity has a
// unique ID, a Position, a set of Components keyed by string, and a Tick
// method the World calls every step.
//
// The design is "data-oriented, Go-flavoured": components are stored as
// any-typed slots in a map, and systems query by component key. This
// avoids the deep inheritance hierarchies that hurt OpenBiohazard2's
// readability.
package entity

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// ID is a unique entity identifier. The zero ID is reserved for "no entity".
type ID uint64

// World owns every active Entity. It is safe for concurrent use; the
// engine ticks entities single-threaded but tests may read positions from
// other goroutines.
type World struct {
	mu       sync.RWMutex
	next     uint64
	entities map[ID]*Entity
}

// NewWorld returns an empty World.
func NewWorld() *World {
	return &World{entities: make(map[ID]*Entity)}
}

// Entity is one actor in the world.
type Entity struct {
	ID       ID
	Position [3]float32
	// Rotation is yaw in radians; RE2's original engine uses only Y
	// rotation because characters are billboarded sprites.
	Rotation float32
	// Components is an open map so systems can stash arbitrary state
	// (health, AI, inventory link, ...).
	Components map[string]any
}

// Spawn creates a new Entity at the given position and returns its ID.
func (w *World) Spawn(pos [3]float32) ID {
	id := ID(atomic.AddUint64(&w.next, 1))
	e := &Entity{
		ID:         id,
		Position:   pos,
		Components: make(map[string]any),
	}
	w.mu.Lock()
	w.entities[id] = e
	w.mu.Unlock()
	return id
}

// Get returns the Entity with the given ID, or nil if it does not exist.
func (w *World) Get(id ID) *Entity {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.entities[id]
}

// Remove deletes the Entity with the given ID. Returns true if something
// was removed.
func (w *World) Remove(id ID) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.entities[id]; !ok {
		return false
	}
	delete(w.entities, id)
	return true
}

// All returns a snapshot of every entity. The slice is safe to mutate;
// callers must not modify the Entity pointers themselves concurrently
// with Tick.
func (w *World) All() []*Entity {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]*Entity, 0, len(w.entities))
	for _, e := range w.entities {
		out = append(out, e)
	}
	return out
}

// Count returns the number of live entities.
func (w *World) Count() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.entities)
}

// Tickler is implemented by any component that wants per-step updates.
// The Ticker's Tick runs once per simulation step; dt is in seconds.
type Ticker interface {
	Tick(e *Entity, dt float32)
}

// Tick advances every component that implements Ticker by dt seconds.
// The engine calls this from the fixed-step loop.
func (w *World) Tick(dt float32) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, e := range w.entities {
		for _, c := range e.Components {
			if t, ok := c.(Ticker); ok {
				t.Tick(e, dt)
			}
		}
	}
}

// AddComponent stores c under the given key. Existing values are
// overwritten.
func (e *Entity) AddComponent(key string, c any) {
	if e.Components == nil {
		e.Components = make(map[string]any)
	}
	e.Components[key] = c
}

// Component retrieves the component stored under key. Returns nil if no
// such component exists.
func (e *Entity) Component(key string) any {
	if e.Components == nil {
		return nil
	}
	return e.Components[key]
}

// HasComponent reports whether the entity has a component stored under key.
func (e *Entity) HasComponent(key string) bool {
	if e.Components == nil {
		return false
	}
	_, ok := e.Components[key]
	return ok
}

// MustComponent is like Component but panics if the component is missing.
// Use it in system code that assumes a component was added at spawn time.
func (e *Entity) MustComponent(key string) any {
	c := e.Component(key)
	if c == nil {
		panic(fmt.Sprintf("entity %d: missing component %q", e.ID, key))
	}
	return c
}

// Health is a common component storing hit points.
type Health struct {
	Current uint16
	Max     uint16
	Dead    bool
}

// Damage reduces Current by n, marking Dead if Current drops to 0.
func (h *Health) Damage(n uint16) {
	if n >= h.Current {
		h.Current = 0
		h.Dead = true
	} else {
		h.Current -= n
	}
}

// Heal increases Current up to Max.
func (h *Health) Heal(n uint16) {
	h.Current += n
	if h.Current > h.Max {
		h.Current = h.Max
	}
	if h.Current > 0 {
		h.Dead = false
	}
}
