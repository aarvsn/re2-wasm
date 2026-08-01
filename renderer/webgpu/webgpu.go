//go:build js && wasm

// Package webgpu is a forward-looking renderer backend scaffold. Phase 3
// ships only the API surface so Phase 6 can wire a real implementation
// without touching engine code; every method returns ErrNotImplemented.
//
// The package mirrors the WebGL2 backend's public shape (New / Init /
// BeginFrame / EndFrame / Shutdown / SetClearColor) so engine.Ports can
// hold either backend interchangeably.
package webgpu

import (
	"errors"
	"syscall/js"

	"github.com/aarvsn/re2-wasm/renderer/common"
)

// ErrNotImplemented is returned by every method in this scaffold. The
// error is exported so callers can detect it and fall back to WebGL2.
var ErrNotImplemented = errors.New("webgpu: not implemented (Phase 6)")

// Renderer is the WebGPU backend. The zero value is not usable; call New.
type Renderer struct {
	canvas  js.Value
	device  js.Value
	context js.Value
	clear   [4]float32
	ready   bool
	stats   common.Stats
}

// New returns a Renderer bound to the canvas with the given DOM id. The
// GPU device is requested lazily during Init.
func New(canvasID string) (*Renderer, error) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		return nil, errors.New("webgpu: document not available")
	}
	canvas := doc.Call("getElementById", canvasID)
	if !canvas.Truthy() {
		return nil, errors.New("webgpu: canvas not found: " + canvasID)
	}
	return &Renderer{canvas: canvas}, nil
}

// IsWebGPUAvailable returns true if the browser exposes navigator.gpu.
// Used by loader.js to decide whether to attempt the WebGPU path.
func IsWebGPUAvailable() bool {
	return js.Global().Get("navigator").Get("gpu").Truthy()
}

// Init would request a GPUDevice and configure the canvas context.
// Phase 6 implements this; for now it returns ErrNotImplemented.
func (r *Renderer) Init() error { return ErrNotImplemented }

// SetClearColor implements engine.Renderer (when wired).
func (r *Renderer) SetClearColor(rR, g, b, a float32) {
	r.clear = [4]float32{rR, g, b, a}
}

// BeginFrame implements engine.Renderer (when wired).
func (r *Renderer) BeginFrame() error { return ErrNotImplemented }

// EndFrame implements engine.Renderer (when wired).
func (r *Renderer) EndFrame() error { return ErrNotImplemented }

// Shutdown implements engine.Renderer (when wired).
func (r *Renderer) Shutdown() error { return nil }

// Stats returns the (currently empty) renderer statistics.
func (r *Renderer) Stats() common.Stats { return r.stats }
