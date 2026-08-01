//go:build js && wasm

// state.go contains the gamepad-polling portion of the input Manager. It is
// split out of input.go so that the keyboard/mouse listener plumbing stays
// readable.
package input

import "syscall/js"

// PollGamepad queries the browser's Gamepad API and refreshes the
// GamepadAxes / GamepadButtons fields on the current state. It must be called
// on a path that already holds no other locks; it takes m.mu itself.
//
// The engine calls this from its per-frame Poll() step. We keep it separate
// so that Phase 4 can extend it (multiple gamepads, dead-zones, rumble)
// without bloating input.go.
func (m *Manager) PollGamepad() {
	if !js.Global().Get("navigator").Truthy() {
		return
	}
	gpFn := js.Global().Get("navigator").Get("getGamepads")
	if !gpFn.Truthy() {
		return
	}
	pads := gpFn.Invoke()
	if !pads.Truthy() || pads.Length() == 0 {
		return
	}
	// Pick the first connected gamepad. Phase 4 will add multi-pad support.
	var pad js.Value
	for i := 0; i < pads.Length(); i++ {
		p := pads.Index(i)
		if p.Truthy() {
			pad = p
			break
		}
	}
	if !pad.Truthy() {
		return
	}
	axesJS := pad.Get("axes")
	buttonsJS := pad.Get("buttons")
	axes := make([]float32, axesJS.Length())
	for i := 0; i < axesJS.Length(); i++ {
		axes[i] = float32(axesJS.Index(i).Float())
	}
	buttons := make([]bool, buttonsJS.Length())
	for i := 0; i < buttonsJS.Length(); i++ {
		b := buttonsJS.Index(i)
		buttons[i] = b.Get("pressed").Bool()
	}
	m.mu.Lock()
	m.current.GamepadAxes = axes
	m.current.GamepadButtons = buttons
	m.mu.Unlock()
}
