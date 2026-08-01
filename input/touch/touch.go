//go:build js && wasm

// Package touch maps browser touch events to the same Action set the
// keyboard / gamepad layers use. On-screen buttons overlay the canvas
// (drawn by the WASM runtime via the sprite batcher) and feed their
// state into the input.Manager.
//
// Phase 6 ships a minimal layout: a virtual D-pad on the left and three
// action buttons (Action / Aim / Fire) on the right. The layout is
// responsive and repositions itself for portrait vs. landscape.
package touch

import (
	"sync"
	"syscall/js"
)

// Button is one on-screen touch control.
type Button struct {
	ID       string
	X, Y     float32 // centre, in CSS pixels
	Radius   float32 // hit radius in CSS pixels
	Held     bool
}

// Layout is the set of on-screen buttons.
type Layout struct {
	mu      sync.Mutex
	buttons map[string]*Button
}

// NewLayout returns the default RE2 touch layout.
func NewLayout() *Layout {
	l := &Layout{buttons: make(map[string]*Button)}
	for _, id := range []string{"up", "down", "left", "right", "action", "aim", "fire"} {
		l.buttons[id] = &Button{ID: id, Radius: 40}
	}
	l.Reposition(800, 600)
	return l
}

// Reposition places the buttons for the given canvas size. The D-pad sits
// in the lower-left; action buttons sit in the lower-right.
func (l *Layout) Reposition(width, height float32) {
	l.mu.Lock()
	defer l.mu.Unlock()
	pad := float32(80)
	dpCX := pad
	dpCY := height - pad - 80
	spacing := float32(60)
	l.buttons["up"].X = dpCX
	l.buttons["up"].Y = dpCY - spacing
	l.buttons["down"].X = dpCX
	l.buttons["down"].Y = dpCY + spacing
	l.buttons["left"].X = dpCX - spacing
	l.buttons["left"].Y = dpCY
	l.buttons["right"].X = dpCX + spacing
	l.buttons["right"].Y = dpCY

	// Action buttons in the lower-right.
	aX := width - pad - 100
	aY := height - pad - 40
	l.buttons["action"].X = aX
	l.buttons["action"].Y = aY
	l.buttons["aim"].X = aX - 90
	l.buttons["aim"].Y = aY + 20
	l.buttons["fire"].X = aX - 40
	l.buttons["fire"].Y = aY - 70
}

// HitTest returns the button under the given (x, y) point, or nil.
func (l *Layout) HitTest(x, y float32) *Button {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, b := range l.buttons {
		dx := x - b.X
		dy := y - b.Y
		if dx*dx+dy*dy <= b.Radius*b.Radius {
			return b
		}
	}
	return nil
}

// SetHeld marks a button as held or released.
func (l *Layout) SetHeld(id string, held bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if b, ok := l.buttons[id]; ok {
		b.Held = held
	}
}

// All returns a snapshot of every button. The sprite batcher uses this to
// draw the on-screen controls.
func (l *Layout) All() []*Button {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]*Button, 0, len(l.buttons))
	for _, b := range l.buttons {
		out = append(out, b)
	}
	return out
}

// Attach installs touch listeners on the given canvas. Returns a release
// function that detaches them.
func (l *Layout) Attach(canvas js.Value) func() {
	if !canvas.Truthy() {
		return func() {}
	}
	onStart := js.FuncOf(func(this js.Value, args []js.Value) any {
		ev := args[0]
		rect := canvas.Call("getBoundingClientRect")
		for i := 0; i < ev.Get("changedTouches").Get("length").Int(); i++ {
			touch := ev.Get("changedTouches").Index(i)
			x := float32(touch.Get("clientX").Float() - rect.Get("left").Float())
			y := float32(touch.Get("clientY").Float() - rect.Get("top").Float())
			if b := l.HitTest(x, y); b != nil {
				l.SetHeld(b.ID, true)
			}
		}
		ev.Call("preventDefault")
		return nil
	})
	onEnd := js.FuncOf(func(this js.Value, args []js.Value) any {
		ev := args[0]
		for i := 0; i < ev.Get("changedTouches").Get("length").Int(); i++ {
			touch := ev.Get("changedTouches").Index(i)
			rect := canvas.Call("getBoundingClientRect")
			x := float32(touch.Get("clientX").Float() - rect.Get("left").Float())
			y := float32(touch.Get("clientY").Float() - rect.Get("top").Float())
			if b := l.HitTest(x, y); b != nil {
				l.SetHeld(b.ID, false)
			}
		}
		ev.Call("preventDefault")
		return nil
	})
	canvas.Call("addEventListener", "touchstart", onStart)
	canvas.Call("addEventListener", "touchend", onEnd)
	canvas.Call("addEventListener", "touchcancel", onEnd)
	return func() {
		canvas.Call("removeEventListener", "touchstart", onStart)
		canvas.Call("removeEventListener", "touchend", onEnd)
		canvas.Call("removeEventListener", "touchcancel", onEnd)
		onStart.Release()
		onEnd.Release()
	}
}
