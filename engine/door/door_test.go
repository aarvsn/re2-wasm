package door

import (
	"errors"
	"testing"
)

func TestTransition_BeginSetsFadeOut(t *testing.T) {
	tr := New()
	tr.Begin("RPD_MAIN", 0.5)
	if tr.Phase() != PhaseFadeOut {
		t.Errorf("Phase = %v, want FadeOut", tr.Phase())
	}
	if tr.TargetRoom != "RPD_MAIN" {
		t.Errorf("TargetRoom = %q, want RPD_MAIN", tr.TargetRoom)
	}
}

func TestTransition_FadeOutReachesBlack(t *testing.T) {
	tr := New()
	tr.Begin("X", 0.5)
	tr.Step(0.5)
	if tr.Phase() != PhaseLoading {
		t.Errorf("Phase = %v, want Loading", tr.Phase())
	}
	if tr.Alpha() != 1.0 {
		t.Errorf("Alpha = %v, want 1.0", tr.Alpha())
	}
}

func TestTransition_CompleteLoadFlipsToFadeIn(t *testing.T) {
	tr := New()
	tr.Begin("X", 0.5)
	tr.Step(0.5) // -> Loading
	if err := tr.CompleteLoad(); err != nil {
		t.Fatal(err)
	}
	if tr.Phase() != PhaseFadeIn {
		t.Errorf("Phase = %v, want FadeIn", tr.Phase())
	}
}

func TestTransition_CompleteLoadCallsOnLoad(t *testing.T) {
	tr := New()
	called := false
	tr.OnLoad = func(room string) error {
		called = true
		if room != "X" {
			t.Errorf("OnLoad room = %q, want X", room)
		}
		return nil
	}
	tr.Begin("X", 0.5)
	tr.Step(0.5) // -> Loading
	if err := tr.CompleteLoad(); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("OnLoad was not called")
	}
}

func TestTransition_CompleteLoadAbortsOnError(t *testing.T) {
	tr := New()
	tr.OnLoad = func(room string) error {
		return errors.New("missing asset")
	}
	tr.Begin("X", 0.5)
	tr.Step(0.5) // -> Loading
	err := tr.CompleteLoad()
	if err == nil {
		t.Fatal("CompleteLoad returned nil, want error")
	}
	if tr.Phase() != PhaseIdle {
		t.Errorf("Phase = %v, want Idle (aborted)", tr.Phase())
	}
	if tr.Alpha() != 1.0 {
		t.Errorf("Alpha = %v, want 1.0 (stays black)", tr.Alpha())
	}
}

func TestTransition_FadeInReturnsToIdle(t *testing.T) {
	tr := New()
	tr.Begin("X", 0.5)
	tr.Step(0.5)      // -> Loading
	tr.CompleteLoad() // -> FadeIn
	tr.Step(0.5)      // -> Idle
	if tr.Phase() != PhaseIdle {
		t.Errorf("Phase = %v, want Idle", tr.Phase())
	}
	if tr.Alpha() != 0 {
		t.Errorf("Alpha = %v, want 0", tr.Alpha())
	}
}

func TestTransition_ActiveFlag(t *testing.T) {
	tr := New()
	if tr.Active() {
		t.Error("Active = true on fresh transition")
	}
	tr.Begin("X", 0.5)
	if !tr.Active() {
		t.Error("Active = false after Begin")
	}
	tr.Step(0.5)
	tr.CompleteLoad()
	tr.Step(0.5) // -> Idle
	if tr.Active() {
		t.Error("Active = true after transition completed")
	}
}

func TestTransition_BeginIsNoOpIfAlreadyActive(t *testing.T) {
	tr := New()
	tr.Begin("X", 0.5)
	tr.Begin("Y", 1.0) // should be ignored
	if tr.TargetRoom != "X" {
		t.Errorf("TargetRoom = %q, want X (Begin should not override)", tr.TargetRoom)
	}
}

func TestPhase_String(t *testing.T) {
	cases := []struct {
		phase Phase
		want  string
	}{
		{PhaseIdle, "idle"},
		{PhaseFadeOut, "fade_out"},
		{PhaseLoading, "loading"},
		{PhaseFadeIn, "fade_in"},
		{Phase(99), "phase(99)"},
	}
	for _, c := range cases {
		if got := c.phase.String(); got != c.want {
			t.Errorf("%v.String() = %q, want %q", c.phase, got, c.want)
		}
	}
}

func TestClamp(t *testing.T) {
	cases := []struct{ v, lo, hi, want float32 }{
		{0.5, 0, 1, 0.5},
		{-1, 0, 1, 0},
		{2, 0, 1, 1},
		{0, 0, 1, 0},
		{1, 0, 1, 1},
	}
	for _, c := range cases {
		if got := clamp(c.v, c.lo, c.hi); got != c.want {
			t.Errorf("clamp(%v,%v,%v) = %v, want %v", c.v, c.lo, c.hi, got, c.want)
		}
	}
}
