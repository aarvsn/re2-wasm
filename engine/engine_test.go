package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aarvsn/re2-wasm/engine/clock"
)

// fakeRenderer is a host-test stand-in for engine.Renderer. It records every
// call so tests can assert ordering and counts.
type fakeRenderer struct {
	mu       sync.Mutex
	inits    int
	begins   int
	ends     int
	shutdown int
	clear    [4]float32
}

func (f *fakeRenderer) Init() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inits++
	return nil
}
func (f *fakeRenderer) SetClearColor(r, g, b, a float32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clear = [4]float32{r, g, b, a}
}
func (f *fakeRenderer) BeginFrame() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.begins++
	return nil
}
func (f *fakeRenderer) EndFrame() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ends++
	return nil
}
func (f *fakeRenderer) Shutdown() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shutdown++
	return nil
}

func TestNew_RequiresRenderer(t *testing.T) {
	_, err := New(Ports{})
	if err == nil {
		t.Fatal("expected error when Renderer is nil")
	}
}

func TestNew_DefaultClockInjected(t *testing.T) {
	e, err := New(Ports{Renderer: &fakeRenderer{}})
	if err != nil {
		t.Fatal(err)
	}
	if e.timer == nil {
		t.Fatal("timer not set")
	}
}

func TestEngine_Step_DrivesRenderer(t *testing.T) {
	r := &fakeRenderer{}
	e, err := New(Ports{Renderer: r, Clock: &clock.Fake{T: time.Unix(0, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Init(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < 5; i++ {
		if err := e.Step(ctx); err != nil {
			t.Fatalf("Step %d: %v", i, err)
		}
	}
	if r.begins != 5 || r.ends != 5 {
		t.Fatalf("begins=%d ends=%d, want 5/5", r.begins, r.ends)
	}
}

func TestEngine_SetClearColor_Forwards(t *testing.T) {
	r := &fakeRenderer{}
	e, _ := New(Ports{Renderer: r})
	e.SetClearColor(0.1, 0.2, 0.3, 0.4)
	if r.clear != [4]float32{0.1, 0.2, 0.3, 0.4} {
		t.Fatalf("clear = %v, want (0.1,0.2,0.3,0.4)", r.clear)
	}
}

func TestEngine_InitFails_OnRendererError(t *testing.T) {
	r := &errRenderer{}
	e, err := New(Ports{Renderer: r})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Init(); !errors.Is(err, errRendererInit) {
		t.Fatalf("Init err = %v, want errRendererInit", err)
	}
}

type errRenderer struct{}

var errRendererInit = errors.New("renderer init failed")

func (errRenderer) Init() error                      { return errRendererInit }
func (errRenderer) SetClearColor(_, _, _, _ float32) {}
func (errRenderer) BeginFrame() error                { return nil }
func (errRenderer) EndFrame() error                  { return nil }
func (errRenderer) Shutdown() error                  { return nil }

func TestEngine_ShutdownIsIdempotent(t *testing.T) {
	e, _ := New(Ports{Renderer: &fakeRenderer{}})
	e.Shutdown()
	e.Shutdown() // must not panic
}
