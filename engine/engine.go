// Package engine wires together the high-level game loop. It owns the
// renderer, input, audio, asset pipeline, and save subsystems, and drives
// them at a fixed simulation rate with an interpolated render rate.
//
// The Engine type is intentionally free of syscall/js usage so that it can be
// exercised by host-based unit tests with mock implementations of every port.
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aarvsn/re2-wasm/engine/clock"
)

// DefaultSimRate is RE2's original simulation frequency. The original game
// runs its logic at 30 Hz; we keep the same cadence so reverse-engineered
// frame timings from OpenBiohazard2 carry over unchanged.
const DefaultSimRate = 30

// DefaultSimStep is the duration of one simulation step at DefaultSimRate.
var DefaultSimStep = time.Second / time.Duration(DefaultSimRate)

// Ports collects every external dependency the engine needs. Each field is an
// interface defined in its own package; passing them in here keeps the engine
// testable and lets us swap a WebGL2 renderer for a WebGPU one without
// touching the loop.
type Ports struct {
	Renderer Renderer
	Audio    Audio
	Input    Input
	Assets   AssetSource
	Saves    SaveStore
	FS       FileSystem
	UI       UI
	Clock    clock.Source
}

// Renderer is the surface the engine draws to. The interface is small on
// purpose: Phase 1 only needs clear colour; later phases will extend it with
// texture, sprite, and model draw calls.
type Renderer interface {
	// Init performs one-time GPU setup (context creation, shader compile).
	Init() error
	// SetClearColor sets the RGBA clear colour in 0..1 range.
	SetClearColor(r, g, b, a float32)
	// BeginFrame clears the framebuffer and prepares for drawing.
	BeginFrame() error
	// EndFrame swaps / presents the framebuffer.
	EndFrame() error
	// Shutdown releases GPU resources.
	Shutdown() error
}

// Audio is the audio port. Stubbed for Phase 1; extended in Phase 4.
type Audio interface {
	Init() error
	Resume() error
	Suspend() error
	Shutdown() error
}

// Input is the input port. Phase 1 only needs polling; full state arrives in
// Phase 4.
type Input interface {
	Init() error
	Poll() error
	Shutdown() error
}

// AssetSource provides on-demand game asset bytes. Phase 2 plugs a real
// BIN/CUE + extracted-data implementation in here; it returns io.ReadCloser
// so callers can stream without buffering the whole asset.
type AssetSource interface {
	Open(ctx context.Context, name string) (io.ReadCloser, error)
}

// SaveStore persists player saves. Phase 4 wires this to IndexedDB.
type SaveStore interface {
	Load(ctx context.Context, slot int) ([]byte, error)
	Save(ctx context.Context, slot int, data []byte) error
	List(ctx context.Context) ([]int, error)
	Export(slot int) ([]byte, error)
	Import(slot int, data []byte) error
}

// FileSystem abstracts the user-provided game files (BIN/CUE, extracted data,
// drag-and-drop payloads). Phase 2 provides the real implementation.
type FileSystem interface {
	Mount(name string, payload []byte) error
	Has(path string) bool
	Read(path string) ([]byte, error)
}

// UI is the browser DOM overlay (loading bar, error toasts, menus). The
// engine treats it opaquely; the wasm package provides the JS-backed
// implementation.
type UI interface {
	SetLoading(progress float32, label string)
	HideLoading()
	ShowError(msg string)
	Shutdown()
}

// Engine drives the game. Create one with New, then call Run.
type Engine struct {
	ports    Ports
	timer    *clock.FrameTimer
	running  bool
	shutdown chan struct{}
}

// New constructs an Engine wired to the given ports. It does not start the
// loop; call Run for that.
func New(ports Ports) (*Engine, error) {
	if ports.Renderer == nil {
		return nil, errors.New("engine: Ports.Renderer is required")
	}
	src := ports.Clock
	if src == nil {
		src = clock.SystemClock{}
	}
	return &Engine{
		ports:    ports,
		timer:    clock.NewFrameTimer(src, DefaultSimStep),
		shutdown: make(chan struct{}),
	}, nil
}

// Timer returns the engine's fixed-step frame timer. Tests use it to drive
// deterministic simulation steps.
func (e *Engine) Timer() *clock.FrameTimer { return e.timer }

// Run blocks until ctx is cancelled or Shutdown is called. It is the main
// entry point invoked by host-side tests; the WASM runtime uses Init + Step
// instead so the loop can be driven by requestAnimationFrame.
func (e *Engine) Run(ctx context.Context) error {
	if err := e.Init(); err != nil {
		return err
	}
	if err := e.loop(ctx); err != nil {
		return fmt.Errorf("engine: loop: %w", err)
	}
	return e.shutdownPorts()
}

// Init performs one-time setup of every port. It is split out of Run so the
// WASM runtime can call it synchronously before kicking off the rAF loop,
// surfacing port-initialisation errors to the user immediately.
func (e *Engine) Init() error {
	if err := e.initPorts(); err != nil {
		return fmt.Errorf("engine: init: %w", err)
	}
	return nil
}

func (e *Engine) initPorts() error {
	if err := e.ports.Renderer.Init(); err != nil {
		return fmt.Errorf("renderer: %w", err)
	}
	if e.ports.Audio != nil {
		if err := e.ports.Audio.Init(); err != nil {
			return fmt.Errorf("audio: %w", err)
		}
	}
	if e.ports.Input != nil {
		if err := e.ports.Input.Init(); err != nil {
			return fmt.Errorf("input: %w", err)
		}
	}
	return nil
}

func (e *Engine) shutdownPorts() error {
	var errs []error
	if e.ports.Input != nil {
		if err := e.ports.Input.Shutdown(); err != nil {
			errs = append(errs, err)
		}
	}
	if e.ports.Audio != nil {
		if err := e.ports.Audio.Shutdown(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := e.ports.Renderer.Shutdown(); err != nil {
		errs = append(errs, err)
	}
	if e.ports.UI != nil {
		e.ports.UI.Shutdown()
	}
	return errors.Join(errs...)
}

// loop is split out so that tests can drive a single iteration via Step().
func (e *Engine) loop(ctx context.Context) error {
	e.running = true
	defer func() { e.running = false }()

	// Phase 1: render a clear colour at the browser rAF cadence. The
	// fixed-step accumulator is exercised but does not yet drive any
	// simulation work; that arrives in Phase 2 onward.
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		select {
		case <-e.shutdown:
			return nil
		default:
		}

		_ = e.timer.Tick()

		if err := e.ports.Renderer.BeginFrame(); err != nil {
			return err
		}
		// Future phases: draw world, sprites, models, UI here.
		if err := e.ports.Renderer.EndFrame(); err != nil {
			return err
		}

		// In the browser the rAF callback drives this loop externally
		// (see wasm/loop.go); when running headless (tests) we yield so
		// the loop does not busy-spin.
		select {
		case <-ctx.Done():
			return nil
		case <-e.shutdown:
			return nil
		case <-time.After(time.Millisecond):
		}
	}
}

// Step drives a single iteration of the loop without sleeping. It is intended
// for tests and for the browser's requestAnimationFrame callback (which calls
// Step instead of Run).
func (e *Engine) Step(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_ = e.timer.Tick()
	if err := e.ports.Renderer.BeginFrame(); err != nil {
		return err
	}
	if err := e.ports.Renderer.EndFrame(); err != nil {
		return err
	}
	return nil
}

// Shutdown signals the loop to stop. It is safe to call from any goroutine.
func (e *Engine) Shutdown() {
	select {
	case <-e.shutdown:
	default:
		close(e.shutdown)
	}
}

// SetClearColor forwards to the renderer. Useful for the Phase 1 smoke test
// that pulses the clear colour to prove the rendering pipeline is live.
func (e *Engine) SetClearColor(r, g, b, a float32) {
	if e.ports.Renderer != nil {
		e.ports.Renderer.SetClearColor(r, g, b, a)
	}
}
