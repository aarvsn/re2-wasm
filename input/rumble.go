//go:build js && wasm

// Package input (rumble.go) wraps the Gamepad API's vibrationActuator.
// Not every browser / gamepad supports it; the Rumble function returns
// false when unavailable so callers can fall back silently.
package input

import "syscall/js"

// RumbleSupported reports whether the first connected gamepad exposes a
// vibrationActuator. RE2 uses rumble for gunshots and damage feedback.
func RumbleSupported() bool {
	pads := js.Global().Get("navigator").Get("getGamepads").Invoke()
	if !pads.Truthy() {
		return false
	}
	for i := 0; i < pads.Length(); i++ {
		p := pads.Index(i)
		if p.Truthy() && p.Get("vibrationActuator").Truthy() {
			return true
		}
	}
	return false
}

// Rumble plays a vibration effect on the first gamepad that supports it.
// duration is in milliseconds; strongMagnitude and weakMagnitude are in
// [0, 1]. Returns true if an effect was actually played.
func Rumble(durationMs int, strongMagnitude, weakMagnitude float32) bool {
	pads := js.Global().Get("navigator").Get("getGamepads").Invoke()
	if !pads.Truthy() {
		return false
	}
	for i := 0; i < pads.Length(); i++ {
		p := pads.Index(i)
		if !p.Truthy() {
			continue
		}
		act := p.Get("vibrationActuator")
		if !act.Truthy() {
			continue
		}
		params := js.Global().Get("Object").New()
		params.Set("duration", durationMs)
		params.Set("strongMagnitude", float64(strongMagnitude))
		params.Set("weakMagnitude", float64(weakMagnitude))
		act.Call("playEffect", "dual-rumble", params)
		return true
	}
	return false
}
