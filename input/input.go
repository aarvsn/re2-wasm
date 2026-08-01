//go:build js && wasm

// Package input collects keyboard, mouse, and gamepad events from the browser
// and exposes them as a polled state snapshot. Phase 1 wires the listeners but
// does not yet surface button presses to the engine; Phase 4 will complete the
// mapping to RE2's in-game actions.
//
// All syscall/js calls live in this package; the engine consumes the
// package's Go-native types and stays testable.
package input

import (
	"sync"
	"syscall/js"
)

// Key identifies a logical key. We use the browser's KeyboardEvent.code
// string (e.g. "KeyW", "ArrowUp", "Space") rather than the deprecated
// keyCode so that layouts work correctly on non-US keyboards.
type Key string

// Action is a high-level game action. Phase 4 will fill these in; for now the
// set is intentionally minimal so we can wire the pipeline end to end.
type Action int

// Supported actions.
const (
	ActionNone Action = iota
	ActionMoveForward
	ActionMoveBackward
	ActionTurnLeft
	ActionTurnRight
	ActionInteract
	ActionInventory
	ActionMap
	ActionAim
	ActionFire
	ActionRun
	ActionCancel
	ActionPause
	ActionMenu
)

// State is a point-in-time snapshot of all input devices. The engine reads
// one copy per simulation step.
type State struct {
	// KeysDown is the set of KeyboardEvent.code values currently held.
	KeysDown map[Key]struct{}

	// MouseX / MouseY are the cursor's position in CSS pixels relative to
	// the canvas top-left.
	MouseX, MouseY float32

	// MouseDX / MouseDY are the per-frame delta in pixels (used with
	// pointer lock for camera control).
	MouseDX, MouseDY float32

	// MouseButtons is a bitmask of currently-pressed mouse buttons (0=L,
	// 1=M, 2=R).
	MouseButtons uint8

	// GamepadAxes is the axis array of the first connected gamepad.
	GamepadAxes []float32

	// GamepadButtons is the button state of the first connected gamepad.
	// Each entry is true while the button is held.
	GamepadButtons []bool
}

// Manager owns the browser event listeners and a thread-safe snapshot of the
// latest input State. Call Init to attach, Poll to copy the current state,
// and Shutdown to detach.
type Manager struct {
	mu       sync.Mutex
	current  State
	polled   State
	listener map[string]js.Func
	canvas   js.Value
	alive    bool
}

// New returns a Manager bound to the given canvas. The canvas is required so
// mouse coordinates can be reported relative to it.
func New(canvas js.Value) *Manager {
	return &Manager{
		canvas:   canvas,
		listener: make(map[string]js.Func),
		current: State{
			KeysDown: make(map[Key]struct{}),
		},
	}
}

// Init attaches all browser event listeners. It must be called before Poll.
func (m *Manager) Init() error {
	if !js.Global().Get("document").Truthy() {
		return nil // host tests: silently no-op
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.alive {
		return nil
	}
	m.alive = true
	m.attach("keydown", m.onKeyDown)
	m.attach("keyup", m.onKeyUp)
	m.attach("mousemove", m.onMouseMove)
	m.attach("mousedown", m.onMouseDown)
	m.attach("mouseup", m.onMouseUp)
	m.attach("contextmenu", m.onContextMenu) // suppress right-click menu
	return nil
}

// attach wraps fn as a js.Func and registers it on window for the given event.
func (m *Manager) attach(event string, fn func(js.Value)) {
	f := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(args[0])
		}
		return nil
	})
	m.listener[event] = f
	js.Global().Call("addEventListener", event, f)
}

// Shutdown detaches every listener previously attached by Init.
func (m *Manager) Shutdown() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.alive {
		return nil
	}
	for event, f := range m.listener {
		js.Global().Call("removeEventListener", event, f)
		f.Release()
	}
	m.listener = make(map[string]js.Func)
	m.alive = false
	return nil
}

// Poll copies the current input State into out and resets per-frame deltas.
// The engine calls this once per simulation step.
func (m *Manager) Poll() error {
	// Refresh gamepad state before copying. The browser API is cheap and
	// non-blocking.
	m.PollGamepad()

	m.mu.Lock()
	defer m.mu.Unlock()
	// Copy current state.
	out := State{
		MouseX:         m.current.MouseX,
		MouseY:         m.current.MouseY,
		MouseDX:        m.current.MouseDX,
		MouseDY:        m.current.MouseDY,
		MouseButtons:   m.current.MouseButtons,
		GamepadAxes:    append([]float32(nil), m.current.GamepadAxes...),
		GamepadButtons: append([]bool(nil), m.current.GamepadButtons...),
		KeysDown:       make(map[Key]struct{}, len(m.current.KeysDown)),
	}
	for k := range m.current.KeysDown {
		out.KeysDown[k] = struct{}{}
	}
	// Per-frame deltas are consumed; reset them.
	m.current.MouseDX = 0
	m.current.MouseDY = 0
	m.polled = out
	return nil
}

// Snapshot returns the most recent state captured by Poll. The engine reads
// this during simulation.
func (m *Manager) Snapshot() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.polled
}

// onKeyDown marks a key as held.
func (m *Manager) onKeyDown(ev js.Value) {
	code := ev.Get("code").String()
	if code == "" {
		return
	}
	m.mu.Lock()
	m.current.KeysDown[Key(code)] = struct{}{}
	m.mu.Unlock()
}

// onKeyUp clears a key.
func (m *Manager) onKeyUp(ev js.Value) {
	code := ev.Get("code").String()
	if code == "" {
		return
	}
	m.mu.Lock()
	delete(m.current.KeysDown, Key(code))
	m.mu.Unlock()
}

// onMouseMove updates cursor position and accumulates deltas.
func (m *Manager) onMouseMove(ev js.Value) {
	rect := js.Value{}
	if m.canvas.Truthy() {
		rect = m.canvas.Call("getBoundingClientRect")
	}
	x := ev.Get("clientX").Float()
	y := ev.Get("clientY").Float()
	if rect.Truthy() {
		x -= rect.Get("left").Float()
		y -= rect.Get("top").Float()
	}
	dx := ev.Get("movementX").Float()
	dy := ev.Get("movementY").Float()
	m.mu.Lock()
	m.current.MouseX = float32(x)
	m.current.MouseY = float32(y)
	m.current.MouseDX += float32(dx)
	m.current.MouseDY += float32(dy)
	m.mu.Unlock()
}

// onMouseDown sets the bit for the pressed mouse button.
func (m *Manager) onMouseDown(ev js.Value) {
	btn := ev.Get("button").Int()
	if btn < 0 || btn > 7 {
		return
	}
	m.mu.Lock()
	m.current.MouseButtons |= 1 << uint(btn)
	m.mu.Unlock()
}

// onMouseUp clears the bit for the released mouse button.
func (m *Manager) onMouseUp(ev js.Value) {
	btn := ev.Get("button").Int()
	if btn < 0 || btn > 7 {
		return
	}
	m.mu.Lock()
	m.current.MouseButtons &^= 1 << uint(btn)
	m.mu.Unlock()
}

// onContextMenu calls preventDefault so right-click on the canvas does not
// pop the browser's context menu (we use right-click for aim in Phase 4).
func (m *Manager) onContextMenu(ev js.Value) {
	ev.Call("preventDefault")
}

// IsKeyDown returns true if the given key is currently held. Convenience
// helper used by Phase 1 smoke tests; the engine itself uses Snapshot().
func (m *Manager) IsKeyDown(k Key) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.current.KeysDown[k]
	return ok
}
