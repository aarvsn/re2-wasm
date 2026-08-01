// Package saves persists player save files in the browser's IndexedDB. Phase
// 1 wires the storage helper so that later phases can read/write RE2's
// save-slot bytes without re-implementing the IndexedDB plumbing.
//
// The package is divided in two: a pure-Go in-memory implementation used by
// host tests, and a JS-backed implementation used in the browser. Both
// implement engine.SaveStore.
package saves

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// SlotMax is the maximum number of save slots the original RE2 supports.
const SlotMax = 20

// MemStore is an in-memory SaveStore used by tests and as a fallback when
// IndexedDB is unavailable.
type MemStore struct {
	mu    sync.Mutex
	slots map[int][]byte
}

// NewMemStore returns an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{slots: make(map[int][]byte)}
}

// Load implements engine.SaveStore.
func (s *MemStore) Load(_ context.Context, slot int) ([]byte, error) {
	if err := checkSlot(slot); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.slots[slot]
	if !ok {
		return nil, ErrSlotEmpty{Slot: slot}
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out, nil
}

// Save implements engine.SaveStore.
func (s *MemStore) Save(_ context.Context, slot int, data []byte) error {
	if err := checkSlot(slot); err != nil {
		return err
	}
	if data == nil {
		return errors.New("saves: data is nil")
	}
	stored := make([]byte, len(data))
	copy(stored, data)
	s.mu.Lock()
	s.slots[slot] = stored
	s.mu.Unlock()
	return nil
}

// List implements engine.SaveStore.
func (s *MemStore) List(_ context.Context) ([]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int, 0, len(s.slots))
	for k := range s.slots {
		out = append(out, k)
	}
	return out, nil
}

// Export implements engine.SaveStore.
func (s *MemStore) Export(slot int) ([]byte, error) {
	if err := checkSlot(slot); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.slots[slot]
	if !ok {
		return nil, ErrSlotEmpty{Slot: slot}
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out, nil
}

// Import implements engine.SaveStore.
func (s *MemStore) Import(slot int, data []byte) error {
	return s.Save(context.Background(), slot, data)
}

// checkSlot validates that slot is in [0, SlotMax).
func checkSlot(slot int) error {
	if slot < 0 || slot >= SlotMax {
		return fmt.Errorf("saves: slot %d out of range [0,%d)", slot, SlotMax)
	}
	return nil
}

// ErrSlotEmpty is returned by Load/Export when the requested slot is unused.
type ErrSlotEmpty struct {
	Slot int
}

// Error implements error.
func (e ErrSlotEmpty) Error() string {
	return fmt.Sprintf("saves: slot %d is empty", e.Slot)
}
