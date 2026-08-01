//go:build js && wasm

// Package input (bindings.go) implements configurable key/mouse/gamepad
// bindings to high-level game Actions. The default layout mirrors RE2's
// PS1 controls; users can remap any binding at runtime.
package input

import (
	"sync"
)

// Binding maps a physical input (key code, mouse button, or gamepad
// button index) to a high-level Action. Only one binding per action is
// active at a time; rebinding overwrites the previous mapping.
type Binding struct {
	Key         Key   // KeyboardEvent.code, or "" if not bound
	MouseButton uint8 // 0=L, 1=M, 2=R; 0xFF if not bound
	GamepadBtn  int   // index into the gamepad's buttons array; -1 if not
}

// defaultBindings is RE2's PS1 layout translated to a modern keyboard.
var defaultBindings = map[Action]Binding{
	ActionMoveForward:  {Key: "KeyW"},
	ActionMoveBackward: {Key: "KeyS"},
	ActionTurnLeft:     {Key: "KeyA"},
	ActionTurnRight:    {Key: "KeyD"},
	ActionInteract:     {Key: "KeyF"},
	ActionInventory:    {Key: "KeyI"},
	ActionMap:          {Key: "KeyM"},
	ActionAim:          {MouseButton: 2}, // right-click
	ActionFire:         {MouseButton: 0}, // left-click
	ActionRun:          {Key: "ShiftLeft"},
	ActionCancel:       {Key: "Escape"},
	ActionPause:        {Key: "KeyP"},
	ActionMenu:         {Key: "Enter"},
}

// Binder owns the per-action binding map. It is safe for concurrent use.
type Binder struct {
	mu       sync.RWMutex
	bindings map[Action]Binding
}

// NewBinder returns a Binder populated with the default RE2 layout.
func NewBinder() *Binder {
	b := &Binder{bindings: make(map[Action]Binding)}
	for a, bnd := range defaultBindings {
		b.bindings[a] = bnd
	}
	return b
}

// Get returns the binding for an action. Returns the zero Binding (which
// matches nothing) if the action has no mapping.
func (b *Binder) Get(a Action) Binding {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.bindings[a]
}

// Set replaces the binding for an action. Pass a zero Binding to clear.
func (b *Binder) Set(a Action, binding Binding) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bindings[a] = binding
}

// ActionFor returns the Action mapped to the given physical input, or
// ActionNone if no binding matches. Used by the engine's input poller.
func (b *Binder) ActionFor(k Key, mouseBtn uint8, gamepadBtn int) Action {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for a, bnd := range b.bindings {
		if bnd.Key != "" && bnd.Key == k {
			return a
		}
		if bnd.MouseButton != 0xFF && bnd.MouseButton == mouseBtn && mouseBtn != 0xFF {
			return a
		}
		if bnd.GamepadBtn >= 0 && bnd.GamepadBtn == gamepadBtn {
			return a
		}
	}
	return ActionNone
}

// ActiveActions returns the set of actions currently held, given a
// snapshot of the input state. This is the main entry point the engine
// uses per simulation step.
func (b *Binder) ActiveActions(s State) map[Action]struct{} {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make(map[Action]struct{}, len(b.bindings))
	for a, bnd := range b.bindings {
		if bnd.Key != "" {
			if _, ok := s.KeysDown[bnd.Key]; ok {
				out[a] = struct{}{}
				continue
			}
		}
		if bnd.MouseButton != 0xFF {
			if s.MouseButtons&(1<<bnd.MouseButton) != 0 {
				out[a] = struct{}{}
				continue
			}
		}
		if bnd.GamepadBtn >= 0 && bnd.GamepadBtn < len(s.GamepadButtons) {
			if s.GamepadButtons[bnd.GamepadBtn] {
				out[a] = struct{}{}
				continue
			}
		}
	}
	return out
}

// Reset restores the default RE2 layout.
func (b *Binder) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bindings = make(map[Action]Binding)
	for a, bnd := range defaultBindings {
		b.bindings[a] = bnd
	}
}
