//go:build js && wasm

// Package ui is the browser DOM overlay: loading bar, error toasts, and the
// debug HUD. The engine consumes this via the engine.UI interface.
//
// Phase 1 ships a minimal implementation that toggles the loading bar and
// pushes error messages into a toast container. Later phases will add menus,
// inventory, and the in-game HUD.
package ui

import (
	"sync"
	"syscall/js"
)

// Overlay is the engine.UI implementation backed by DOM elements. It looks up
// elements lazily so it can be constructed before the DOM is ready.
type Overlay struct {
	mu sync.Mutex
}

// New returns an Overlay. The actual element lookups happen on each call so
// that the overlay keeps working even if loader.js rebuilds the DOM after the
// WASM module has booted.
func New() *Overlay { return &Overlay{} }

// SetLoading implements engine.UI. progress is 0..1; label is shown above the
// bar. Values outside [0,1] are clamped.
func (o *Overlay) SetLoading(progress float32, label string) {
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		return
	}
	screen := doc.Call("getElementById", "loading-screen")
	if !screen.Truthy() {
		return
	}
	screen.Get("style").Set("display", "flex")
	bar := doc.Call("getElementById", "loading-bar")
	if bar.Truthy() {
		bar.Get("style").Set("width", ftoa(progress*100)+"%")
	}
	lbl := doc.Call("getElementById", "loading-label")
	if lbl.Truthy() {
		lbl.Set("textContent", label)
	}
	pct := doc.Call("getElementById", "loading-percent")
	if pct.Truthy() {
		pct.Set("textContent", ftoa(progress*100)+"%")
	}
}

// HideLoading implements engine.UI.
func (o *Overlay) HideLoading() {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		return
	}
	screen := doc.Call("getElementById", "loading-screen")
	if screen.Truthy() {
		screen.Get("style").Set("display", "none")
	}
}

// ShowError implements engine.UI. The toast is shown for 6 seconds.
func (o *Overlay) ShowError(msg string) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		return
	}
	toast := doc.Call("getElementById", "error-toast")
	if !toast.Truthy() {
		// Fall back to console so the message is never lost.
		js.Global().Get("console").Call("error", msg)
		return
	}
	toast.Set("textContent", msg)
	toast.Get("classList").Call("add", "visible")
	// Auto-hide after 6s. We use setTimeout so we do not need a Go timer.
	js.Global().Call("setTimeout", js.FuncOf(func(this js.Value, args []js.Value) any {
		toast.Get("classList").Call("remove", "visible")
		return nil
	}), 6000)
}

// Shutdown implements engine.UI. There is nothing to release for Phase 1.
func (o *Overlay) Shutdown() {}

// ftoa formats a float32 as a string with up to 1 decimal place. We avoid
// strconv to keep the WASM binary smaller; the only consumers of this value
// are CSS width strings.
func ftoa(f float32) string {
	whole := int(f)
	frac := int((f - float32(whole)) * 10)
	if frac < 0 {
		frac = -frac
	}
	return itoa(whole) + "." + itoa(frac)
}

// itoa is a tiny int-to-string helper that avoids pulling in strconv for a
// handful of small numbers used by the loading bar.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
