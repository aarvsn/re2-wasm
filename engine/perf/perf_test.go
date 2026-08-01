package perf

import (
	"testing"
	"time"
)

func TestTracker_BeginEndUpdatesCounter(t *testing.T) {
	tr := New()
	tr.Begin(SectionRender)
	time.Sleep(2 * time.Millisecond)
	tr.End(SectionRender)
	c := tr.Counter(SectionRender)
	if c.LastMS < 1.0 {
		t.Errorf("LastMS = %v, want >= 1.0", c.LastMS)
	}
	if c.Count() != 1 {
		t.Errorf("count = %d, want 1", c.Count())
	}
}

func TestTracker_AverageIsExponentialMoving(t *testing.T) {
	tr := New()
	// Three measurements: 1ms, 2ms, 3ms (simulated via direct calls).
	// The EMA with α=0.1 should be: 1, 1*0.9+2*0.1=1.1, 1.1*0.9+3*0.1=1.29
	// We can't precisely sleep for 1ms reliably, so we sleep for 2ms
	// to ensure a non-zero duration on all operating systems (e.g., Windows).
	for i := 0; i < 5; i++ {
		tr.Begin(SectionSim)
		time.Sleep(2 * time.Millisecond)
		tr.End(SectionSim)
	}
	c := tr.Counter(SectionSim)
	if c.Count() != 5 {
		t.Errorf("count = %d, want 5", c.Count())
	}
	if c.AvgMS <= 0 {
		t.Errorf("AvgMS = %v, want > 0", c.AvgMS)
	}
}

func TestTracker_MaxMSIncreases(t *testing.T) {
	tr := New()
	tr.Begin(SectionRender)
	tr.End(SectionRender)
	firstMax := tr.Counter(SectionRender).MaxMS
	tr.Begin(SectionRender)
	time.Sleep(5 * time.Millisecond)
	tr.End(SectionRender)
	secondMax := tr.Counter(SectionRender).MaxMS
	if secondMax <= firstMax {
		t.Errorf("MaxMS did not increase: %v -> %v", firstMax, secondMax)
	}
}

func TestTracker_ResetClears(t *testing.T) {
	tr := New()
	tr.Begin(SectionRender)
	tr.End(SectionRender)
	tr.Reset()
	c := tr.Counter(SectionRender)
	if c.Count() != 0 || c.LastMS != 0 || c.MaxMS != 0 {
		t.Errorf("after Reset: %+v, want zero", c)
	}
}

func TestSection_String(t *testing.T) {
	cases := []struct {
		s    Section
		want string
	}{
		{SectionFrame, "frame"},
		{SectionSim, "sim"},
		{SectionRender, "render"},
		{SectionAudio, "audio"},
		{SectionInput, "input"},
		{SectionAssets, "assets"},
		{Section(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("%v.String() = %q, want %q", c.s, got, c.want)
		}
	}
}
