// Package clock provides a deterministic, frame-rate-independent wall clock
// for the engine. It is decoupled from syscall/js so it can be unit-tested on
// the host. The browser entry point injects a real time source; tests inject
// a fake one.
package clock

import "time"

// Source is anything that can report the current monotonic time.
type Source interface {
	// Now returns the current time. Implementations MUST be monotonic.
	Now() time.Time
}

// SystemClock is a Source backed by time.Now().
type SystemClock struct{}

// Now implements Source.
func (SystemClock) Now() time.Time { return time.Now() }

// Fake is a manually-advanceable Source intended for unit tests.
type Fake struct {
	T time.Time
}

// Now implements Source.
func (f *Fake) Now() time.Time { return f.T }

// Advance moves the fake clock forward by d and returns the new time.
func (f *Fake) Advance(d time.Duration) time.Time {
	f.T = f.T.Add(d)
	return f.T
}

// FrameTimer converts wall-clock deltas into a fixed-step accumulator used by
// the engine's update loop. RE2 runs its simulation at 30 Hz; we decouple the
// render rate (browser rAF, ~60 Hz) from the simulation rate by stepping the
// sim a whole number of times per frame.
type FrameTimer struct {
	source    Source
	simStep   time.Duration
	acc       time.Duration
	last      time.Time
	started   bool
	totalStep int
}

// NewFrameTimer returns a FrameTimer that produces simulation steps of simStep.
// A simStep of 0 panics.
func NewFrameTimer(source Source, simStep time.Duration) *FrameTimer {
	if simStep <= 0 {
		panic("clock: simStep must be positive")
	}
	return &FrameTimer{source: source, simStep: simStep}
}

// Tick should be called once per render frame. It returns the number of
// fixed-step simulation updates that should run this frame. The first call
// after construction (or after Reset) always returns 0 to avoid a huge
// "catch-up" spike after load.
func (t *FrameTimer) Tick() int {
	now := t.source.Now()
	if !t.started {
		t.last = now
		t.started = true
		return 0
	}
	delta := now.Sub(t.last)
	t.last = now
	if delta < 0 {
		// Clock went backwards; ignore.
		return 0
	}
	// Clamp to avoid spiral-of-death after a tab was backgrounded.
	if delta > 250*time.Millisecond {
		delta = 250 * time.Millisecond
	}
	t.acc += delta
	steps := 0
	for t.acc >= t.simStep {
		t.acc -= t.simStep
		steps++
		t.totalStep++
		if steps >= 5 {
			// Cap per-frame catch-up so the tab can recover quickly.
			t.acc = 0
			break
		}
	}
	return steps
}

// Reset clears the accumulator and re-arms the timer so the next Tick returns
// 0. Useful after a long pause (e.g. asset loading).
func (t *FrameTimer) Reset() {
	t.started = false
	t.acc = 0
	t.totalStep = 0
}

// Alpha returns the fractional simulation step (0..1) for interpolation
// between the previous and current simulation state. Multiply motion by this
// value when interpolating renderable transforms.
func (t *FrameTimer) Alpha() float32 {
	return float32(t.acc) / float32(t.simStep)
}

// TotalSteps returns the cumulative number of simulation steps performed since
// the last Reset. Useful for profiling and deterministic replays.
func (t *FrameTimer) TotalSteps() int { return t.totalStep }
