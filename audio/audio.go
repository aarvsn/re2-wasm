//go:build js && wasm

// Package audio is the browser Web Audio API port. Phase 1 wires up an
// AudioContext with the proper browser unlock dance (resume on first user
// gesture) but does not yet play any sounds. Phase 4 will add positional
// sources, music streams, and SFX banks.
//
// Every syscall/js call lives in this package; the engine consumes the
// Audio interface declared in engine/engine.go.
package audio

import (
	"errors"
	"sync"
	"syscall/js"
)

// Manager wraps a browser AudioContext. It is safe for concurrent use.
type Manager struct {
	mu      sync.Mutex
	ctx     js.Value
	master  js.Value
	suspended bool
}

// New constructs a Manager. The AudioContext itself is created lazily in Init
// so that browser autoplay policies (which require a user gesture) are
// respected: the caller must call Resume after the first user interaction.
func New() *Manager { return &Manager{} }

// Init creates the AudioContext and a master gain node. It is safe to call
// even outside a browser (the call becomes a no-op).
func (m *Manager) Init() error {
	if !js.Global().Get("window").Truthy() {
		return nil
	}
	ctor := js.Global().Get("AudioContext")
	if !ctor.Truthy() {
		// Older WebKit prefixes.
		ctor = js.Global().Get("webkitAudioContext")
	}
	if !ctor.Truthy() {
		return errors.New("audio: Web Audio API is not available in this browser")
	}
	ctx := ctor.New()
	master := ctx.Call("createGain")
	master.Call("connect", ctx.Get("destination"))
	master.Get("gain").Set("value", 1.0)
	m.mu.Lock()
	m.ctx = ctx
	m.master = master
	m.suspended = ctx.Get("state").String() == "suspended"
	m.mu.Unlock()
	return nil
}

// Resume un-suspends the AudioContext. Browsers start the context in the
// "suspended" state until a user gesture occurs; loader.js calls this from
// its first click/keydown listener.
func (m *Manager) Resume() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.ctx.Truthy() {
		return nil
	}
	if !m.suspended && m.ctx.Get("state").String() == "running" {
		return nil
	}
	p := m.ctx.Call("resume")
	// Promise: ignore rejection; we will retry on next gesture.
	if p.Truthy() && p.Get("then").Truthy() {
		p.Call("then", js.FuncOf(func(this js.Value, args []js.Value) any {
			m.mu.Lock()
			m.suspended = false
			m.mu.Unlock()
			return nil
		}))
	}
	return nil
}

// Suspend pauses the AudioContext. Used when the tab is backgrounded.
func (m *Manager) Suspend() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.ctx.Truthy() {
		return nil
	}
	m.ctx.Call("suspend")
	m.suspended = true
	return nil
}

// Shutdown closes the AudioContext and releases the master gain.
func (m *Manager) Shutdown() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.ctx.Truthy() {
		return nil
	}
	m.ctx.Call("close")
	m.ctx = js.Value{}
	m.master = js.Value{}
	return nil
}

// SetMasterVolume scales every playing sound. v is clamped to [0, 1].
func (m *Manager) SetMasterVolume(v float32) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.master.Truthy() {
		m.master.Get("gain").Set("value", v)
	}
}

// Context returns the underlying AudioContext. Exposed for Phase 4 so that
// positional sources can attach to the listener without going through the
// Manager for every node creation.
func (m *Manager) Context() js.Value {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ctx
}
