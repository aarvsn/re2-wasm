// Package perf is the engine's performance instrumentation layer. It owns
// a per-section timer that the engine taps into from the rAF loop; the
// debug HUD reads the resulting counters once per second.
//
// The implementation is allocation-free in the steady state so the
// instrumentation itself does not perturb the measurements.
package perf

import (
	"sync"
	"time"
)

// Section is a named region of the frame (e.g. "render", "sim", "audio").
type Section int

// Known sections. Reorder freely; do not reuse indices across versions.
const (
	SectionFrame Section = iota
	SectionSim
	SectionRender
	SectionAudio
	SectionInput
	SectionAssets
	sectionCount
)

// Counter tracks per-section timing statistics.
type Counter struct {
	LastMS float32
	AvgMS  float32
	MaxMS  float32
	count  uint64
}

// Count returns the number of times End has been called for this section.
func (c Counter) Count() uint64 { return c.count }

// Tracker is the engine-wide performance tracker.
type Tracker struct {
	mu       sync.Mutex
	starts   [sectionCount]time.Time
	counters [sectionCount]Counter
}

// New returns a Tracker ready to record.
func New() *Tracker { return &Tracker{} }

// Begin records the start of a section. Pair with End on the same section.
func (t *Tracker) Begin(s Section) {
	t.mu.Lock()
	t.starts[s] = time.Now()
	t.mu.Unlock()
}

// End records the end of a section and updates its counter.
func (t *Tracker) End(s Section) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	dur := now.Sub(t.starts[s])
	ms := float32(dur.Seconds() * 1000)
	c := &t.counters[s]
	c.LastMS = ms
	c.count++
	if ms > c.MaxMS {
		c.MaxMS = ms
	}
	// Exponential moving average with α=0.1; fast to react, slow to settle.
	if c.AvgMS == 0 {
		c.AvgMS = ms
	} else {
		c.AvgMS = c.AvgMS*0.9 + ms*0.1
	}
}

// Counter returns a snapshot of the given section's counter.
func (t *Tracker) Counter(s Section) Counter {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.counters[s]
}

// Reset clears every counter. Used when the HUD rebuilds its display.
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := range t.counters {
		t.counters[i] = Counter{}
	}
}

// String returns the section's name for HUD display.
func (s Section) String() string {
	switch s {
	case SectionFrame:
		return "frame"
	case SectionSim:
		return "sim"
	case SectionRender:
		return "render"
	case SectionAudio:
		return "audio"
	case SectionInput:
		return "input"
	case SectionAssets:
		return "assets"
	default:
		return "unknown"
	}
}
