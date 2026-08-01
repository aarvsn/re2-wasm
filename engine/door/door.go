// Package door implements RE2's door transitions. When the player crosses
// a trigger, the screen fades to black, the engine loads the next room,
// and the screen fades back in. Door transitions also mask asset loading.
//
// The State machine in this package is decoupled from the renderer so it
// can be tested on the host. The engine drives the actual fade by reading
// State.Alpha each frame.
package door

import (
	"fmt"
	"sync"
)

// Phase is one stage of a door transition.
type Phase int

// Supported phases.
const (
	PhaseIdle Phase = iota
	PhaseFadeOut
	PhaseLoading
	PhaseFadeIn
)

// String returns the phase's name.
func (p Phase) String() string {
	switch p {
	case PhaseIdle:
		return "idle"
	case PhaseFadeOut:
		return "fade_out"
	case PhaseLoading:
		return "loading"
	case PhaseFadeIn:
		return "fade_in"
	default:
		return fmt.Sprintf("phase(%d)", int(p))
	}
}

// Transition is a single door-transition state machine.
type Transition struct {
	mu       sync.Mutex
	phase    Phase
	alpha    float32 // 0 = clear, 1 = fully black
	elapsed  float32 // seconds in current phase
	duration float32 // per-phase duration

	// OnLoad is invoked once when the transition enters PhaseLoading. It
	// should swap the active room; the transition stays in PhaseLoading
	// until OnLoad returns.
	OnLoad func(targetRoom string) error

	// TargetRoom is the room the transition is heading to.
	TargetRoom string
}

// New returns an idle Transition.
func New() *Transition { return &Transition{phase: PhaseIdle} }

// Phase returns the current phase.
func (t *Transition) Phase() Phase {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.phase
}

// Alpha returns the current fade alpha (0..1).
func (t *Transition) Alpha() float32 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.alpha
}

// Begin starts a transition to the given room. durationPerPhase is the
// fade-out and fade-in duration in seconds (typically 0.5).
func (t *Transition) Begin(targetRoom string, durationPerPhase float32) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.phase != PhaseIdle {
		return
	}
	t.phase = PhaseFadeOut
	t.alpha = 0
	t.elapsed = 0
	t.duration = durationPerPhase
	t.TargetRoom = targetRoom
}

// Step advances the transition by dt seconds. The engine calls this from
// its per-frame loop.
func (t *Transition) Step(dt float32) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch t.phase {
	case PhaseIdle, PhaseLoading:
		return nil
	case PhaseFadeOut:
		t.elapsed += dt
		t.alpha = clamp(t.elapsed/t.duration, 0, 1)
		if t.elapsed >= t.duration {
			t.phase = PhaseLoading
			t.elapsed = 0
		}
		return nil
	case PhaseFadeIn:
		t.elapsed += dt
		t.alpha = clamp(1-t.elapsed/t.duration, 0, 1)
		if t.elapsed >= t.duration {
			t.phase = PhaseIdle
			t.alpha = 0
		}
		return nil
	}
	return nil
}

// CompleteLoad is called by the engine once the new room has finished
// loading. It flips the transition from PhaseLoading to PhaseFadeIn. If
// OnLoad returns an error, the transition aborts back to PhaseIdle with
// alpha = 1 (so the screen stays black and the engine can show an error).
func (t *Transition) CompleteLoad() error {
	t.mu.Lock()
	phase := t.phase
	target := t.TargetRoom
	onLoad := t.OnLoad
	t.mu.Unlock()
	if phase != PhaseLoading {
		return nil
	}
	if onLoad != nil {
		if err := onLoad(target); err != nil {
			t.mu.Lock()
			t.phase = PhaseIdle
			t.alpha = 1
			t.mu.Unlock()
			return err
		}
	}
	t.mu.Lock()
	t.phase = PhaseFadeIn
	t.elapsed = 0
	t.mu.Unlock()
	return nil
}

// Active reports whether a transition is in progress (i.e. not PhaseIdle).
func (t *Transition) Active() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.phase != PhaseIdle
}

// clamp restricts v to [lo, hi].
func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
