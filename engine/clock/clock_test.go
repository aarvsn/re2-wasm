package clock

import (
	"testing"
	"time"
)

func TestFakeAdvance(t *testing.T) {
	f := &Fake{T: time.Unix(0, 0)}
	if got := f.Now(); !got.Equal(time.Unix(0, 0)) {
		t.Fatalf("Now = %v, want epoch", got)
	}
	got := f.Advance(2 * time.Second)
	if want := time.Unix(0, 2_000_000_000); !got.Equal(want) {
		t.Fatalf("Advance = %v, want %v", got, want)
	}
}

func TestFrameTimer_TickAccumulates(t *testing.T) {
	f := &Fake{T: time.Unix(0, 0)}
	const step = 33 * time.Millisecond // ~30 Hz, rounded
	tr := NewFrameTimer(f, step)

	// First Tick must return 0 (no history yet).
	if got := tr.Tick(); got != 0 {
		t.Fatalf("first Tick = %d, want 0", got)
	}

	cases := []struct {
		adv  time.Duration
		want int // expected steps this tick
	}{
		{0, 0},
		{10 * time.Millisecond, 0},
		{33 * time.Millisecond, 1},
		{66 * time.Millisecond, 2}, // 2 full steps in one frame
		{33 * time.Millisecond, 1},
		{200 * time.Millisecond, 5}, // capped at 5 per frame
	}

	for i, c := range cases {
		f.Advance(c.adv)
		got := tr.Tick()
		if got != c.want {
			t.Fatalf("case %d: Tick = %d, want %d (adv=%v)", i, got, c.want, c.adv)
		}
	}
}

func TestFrameTimer_Reset(t *testing.T) {
	f := &Fake{T: time.Unix(0, 0)}
	tr := NewFrameTimer(f, 16*time.Millisecond)
	// Prime the timer so subsequent advances can accumulate.
	tr.Tick()
	f.Advance(100 * time.Millisecond)
	if n := tr.Tick(); n == 0 {
		t.Fatal("expected steps to accumulate before Reset")
	}
	if tr.TotalSteps() == 0 {
		t.Fatal("expected TotalSteps > 0 before Reset")
	}
	tr.Reset()
	if tr.TotalSteps() != 0 {
		t.Fatalf("TotalSteps after Reset = %d, want 0", tr.TotalSteps())
	}
	if got := tr.Tick(); got != 0 {
		t.Fatalf("Tick after Reset = %d, want 0", got)
	}
}

func TestFrameTimer_AlphaInRange(t *testing.T) {
	f := &Fake{T: time.Unix(0, 0)}
	tr := NewFrameTimer(f, 100*time.Millisecond)
	tr.Tick()
	f.Advance(150 * time.Millisecond)
	tr.Tick()
	// After 50 ms extra in the accumulator, alpha should be 0.5.
	if a := tr.Alpha(); a < 0.49 || a > 0.51 {
		t.Fatalf("Alpha = %v, want ~0.5", a)
	}
}

func TestNewFrameTimer_PanicsOnZeroStep(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on zero step")
		}
	}()
	_ = NewFrameTimer(&Fake{}, 0)
}
