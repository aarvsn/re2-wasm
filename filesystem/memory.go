// Package filesystem abstracts the user-provided game files. Phase 1 ships
// an in-memory store populated by the browser's drag-and-drop / file-picker
// handlers; Phase 2 will add a BIN/CUE parser and a virtual file tree.
//
// The package is intentionally engine-agnostic: it implements engine.FileSystem
// but does not import the engine package (the engine depends on us, not the
// other way around).
package filesystem

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
)

// MemoryFS is a simple in-memory file store keyed by lower-cased path. It is
// safe for concurrent use.
type MemoryFS struct {
	mu    sync.RWMutex
	files map[string][]byte
}

// New returns an empty MemoryFS.
func New() *MemoryFS {
	return &MemoryFS{files: make(map[string][]byte)}
}

// Mount stores payload under name. Existing entries are overwritten. Names
// are normalised (leading slashes stripped, backslashes converted, lower-cased)
// so that Windows-style paths from BIN/CUE parsers behave identically to
// Unix-style ones.
func (m *MemoryFS) Mount(name string, payload []byte) error {
	if name == "" {
		return errors.New("filesystem: name is required")
	}
	if payload == nil {
		return errors.New("filesystem: payload is nil")
	}
	key := normalise(name)
	m.mu.Lock()
	m.files[key] = payload
	m.mu.Unlock()
	return nil
}

// Has reports whether path exists in the store.
func (m *MemoryFS) Has(p string) bool {
	m.mu.RLock()
	_, ok := m.files[normalise(p)]
	m.mu.RUnlock()
	return ok
}

// Read returns the bytes of p. The slice is a defensive copy so callers can
// mutate it freely.
func (m *MemoryFS) Read(p string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.files[normalise(p)]
	if !ok {
		return nil, fmt.Errorf("filesystem: %q not found", p)
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out, nil
}

// List returns every mounted path, sorted alphabetically.
func (m *MemoryFS) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.files))
	for k := range m.files {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Remove unmounts a single path. Returns true if something was removed.
func (m *MemoryFS) Remove(p string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := normalise(p)
	if _, ok := m.files[key]; !ok {
		return false
	}
	delete(m.files, key)
	return true
}

// Size returns the total byte size of all mounted files.
func (m *MemoryFS) Size() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var n int64
	for _, b := range m.files {
		n += int64(len(b))
	}
	return n
}

// normalise lower-cases the path, strips leading slashes, and converts
// backslashes to forward slashes so that BIN/CUE / Windows paths behave the
// same as Unix ones.
func normalise(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "/")
	p = path.Clean(p)
	return strings.ToLower(p)
}
