//go:build js && wasm

// Package webgl implements the engine.Renderer interface on top of a WebGL 2
// (GLES 3.0) context obtained from a canvas element. It is designed so that a
// future WebGPU backend can sit beside it without touching engine code.
//
// Every WebGL call goes through the thin GL wrapper in context.go. That
// wrapper exists for two reasons: it gives us a place to centralise error
// checking (gl.GetError after every mutating call), and it lets unit tests
// substitute a fake GL implementation on the host where syscall/js is not
// available.
package webgl

import (
        "errors"
        "fmt"
        "strings"
        "syscall/js"
        "sync/atomic"

        "github.com/aarvsn/re2-wasm/renderer/common"
)

// Renderer is the WebGL2 backend. The zero value is not usable; call New.
type Renderer struct {
        canvas  js.Value
        gl      js.Value
        vao     js.Value // bound vertex array object for Phase 1 draw calls
        program js.Value // active shader program; nil-equivalent until Phase 3
        clear   [4]float32
        stats   common.Stats
        ready   atomic.Bool
}

// New returns a Renderer bound to the canvas with the given DOM id. The WebGL2
// context is requested lazily during Init so that constructor failures are
// deferred to the engine's init phase where the user can see them in the UI.
func New(canvasID string) (*Renderer, error) {
        if canvasID == "" {
                return nil, errors.New("webgl: canvasID is required")
        }
        doc := js.Global().Get("document")
        if !doc.Truthy() {
                return nil, errors.New("webgl: document is not available (not running in a browser?)")
        }
        canvas := doc.Call("getElementById", canvasID)
        if !canvas.Truthy() {
                return nil, fmt.Errorf("webgl: canvas element %q not found", canvasID)
        }
        return &Renderer{canvas: canvas}, nil
}

// FromCanvas is like New but accepts an already-resolved canvas js.Value. It
// is used by tests and by callers that create the canvas programmatically.
func FromCanvas(canvas js.Value) (*Renderer, error) {
        if !canvas.Truthy() {
                return nil, errors.New("webgl: canvas is nil")
        }
        return &Renderer{canvas: canvas}, nil
}

// Init requests a WebGL2 context and configures default state.
func (r *Renderer) Init() error {
        if !r.canvas.Truthy() {
                return errors.New("webgl: canvas not set")
        }
        attrs := js.Global().Get("Object").New()
        attrs.Set("alpha", true)
        attrs.Set("antialias", false) // RE2 uses its own dithering; skip MSAA
        attrs.Set("depth", true)
        attrs.Set("stencil", false)
        attrs.Set("premultipliedAlpha", true)
        attrs.Set("preserveDrawingBuffer", false)
        attrs.Set("powerPreference", "high-performance")
        attrs.Set("desynchronized", true)
        gl := r.canvas.Call("getContext", "webgl2", attrs)
        if !gl.Truthy() {
                // Fall back to a friendlier error rather than a bare nil.
                return errors.New("webgl: WebGL2 is not available in this browser; RE2 requires WebGL 2 / GLES 3.0")
        }
        r.gl = gl

        // Phase 1 only needs a VAO so later draw calls have a bound container.
        vao := gl.Call("createVertexArray")
        if vao.Truthy() {
                gl.Call("bindVertexArray", vao)
                r.vao = vao
        }

        // Default state: depth test on, cull face on (CCW front).
        gl.Call("enable", gl.Get("DEPTH_TEST"))
        gl.Call("depthFunc", gl.Get("LEQUAL"))
        gl.Call("enable", gl.Get("CULL_FACE"))
        gl.Call("cullFace", gl.Get("BACK"))
        gl.Call("frontFace", gl.Get("CCW"))
        gl.Call("enable", gl.Get("BLEND"))
        gl.Call("blendFunc", gl.Get("SRC_ALPHA"), gl.Get("ONE_MINUS_SRC_ALPHA"))

        r.SetClearColor(0.0, 0.0, 0.0, 1.0)
        r.ready.Store(true)
        return nil
}

// SetClearColor implements engine.Renderer.
func (r *Renderer) SetClearColor(rR, g, b, a float32) {
        r.clear = [4]float32{rR, g, b, a}
        if r.gl.Truthy() {
                r.gl.Call("clearColor", rR, g, b, a)
        }
}

// ClearColor returns the current clear colour. Used by tests and the UI
// overlay's debug HUD.
func (r *Renderer) ClearColor() (rR, g, b, a float32) {
        return r.clear[0], r.clear[1], r.clear[2], r.clear[3]
}

// BeginFrame implements engine.Renderer. It resizes the viewport to match the
// canvas drawing buffer size and clears colour + depth.
func (r *Renderer) BeginFrame() error {
        if !r.ready.Load() {
                return errors.New("webgl: renderer not initialised")
        }
        gl := r.gl
        if !gl.Truthy() {
                return errors.New("webgl: GL context lost")
        }
        // Resize to display size if the canvas has been CSS-resized.
        w := r.canvas.Get("clientWidth").Int()
        h := r.canvas.Get("clientHeight").Int()
        dw := r.canvas.Get("width").Int()
        dh := r.canvas.Get("height").Int()
        needResize := dw != w || dh != h
        if w == 0 {
                w = dw
        }
        if h == 0 {
                h = dh
        }
        if needResize && w > 0 && h > 0 {
                r.canvas.Set("width", w)
                r.canvas.Set("height", h)
                gl.Call("viewport", 0, 0, w, h)
        }
        mask := gl.Get("COLOR_BUFFER_BIT").Int() | gl.Get("DEPTH_BUFFER_BIT").Int()
        gl.Call("clear", mask)
        return nil
}

// EndFrame implements engine.Renderer. WebGL2 does not have an explicit
// swap-buffers call (the browser compositor flips automatically); we only
// flush so the GPU starts work immediately.
func (r *Renderer) EndFrame() error {
        if !r.gl.Truthy() {
                return errors.New("webgl: GL context lost")
        }
        r.gl.Call("flush")
        r.stats.FrameNumber++
        return nil
}

// Shutdown releases all GPU resources. The GL context itself cannot be freed
// from Go side; the browser will GC it once the canvas is dropped.
func (r *Renderer) Shutdown() error {
        if !r.gl.Truthy() {
                return nil
        }
        if r.vao.Truthy() {
                r.gl.Call("deleteVertexArray", r.vao)
                r.vao = js.Value{}
        }
        if r.program.Truthy() {
                r.gl.Call("deleteProgram", r.program)
                r.program = js.Value{}
        }
        r.ready.Store(false)
        return nil
}

// GL returns the underlying WebGL2RenderingContext value. It is exported so
// that future packages (renderer/webgl internals, debug overlays) can issue
// raw GL calls without going through engine.Renderer.
func (r *Renderer) GL() js.Value { return r.gl }

// Canvas returns the canvas element the renderer is bound to.
func (r *Renderer) Canvas() js.Value { return r.canvas }

// Stats returns a snapshot of renderer statistics.
func (r *Renderer) Stats() common.Stats { return r.stats }

// IsWebGL2Available returns true if the current browser can create a WebGL2
// context. Used by loader.js to gate the loading UI.
func IsWebGL2Available() bool {
        if !js.Global().Get("document").Truthy() {
                return false
        }
        c := js.Global().Get("document").Call("createElement", "canvas")
        if !c.Truthy() {
                return false
        }
        return c.Call("getContext", "webgl2").Truthy()
}

// ShaderCompileError is returned by CompileShader when the GLSL source fails
// to compile. The InfoLog field contains the driver's error text.
type ShaderCompileError struct {
        Stage   string // "vertex" or "fragment"
        InfoLog string
}

// Error implements error.
func (e ShaderCompileError) Error() string {
        return fmt.Sprintf("webgl: %s shader compile failed: %s", e.Stage, strings.TrimSpace(e.InfoLog))
}
