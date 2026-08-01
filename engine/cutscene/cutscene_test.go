package cutscene

import "testing"

func TestPlayer_StartSortsByTime(t *testing.T) {
	p := New()
	p.Add(Cue{Time: 2.0, Kind: CueKindDialogue})
	p.Add(Cue{Time: 0.5, Kind: CueKindCameraChange})
	p.Add(Cue{Time: 1.0, Kind: CueKindSubtitle})
	p.Start()
	if p.cues[0].Time != 0.5 {
		t.Errorf("cues[0].Time = %v, want 0.5", p.cues[0].Time)
	}
	if p.cues[1].Time != 1.0 {
		t.Errorf("cues[1].Time = %v, want 1.0", p.cues[1].Time)
	}
	if p.cues[2].Time != 2.0 {
		t.Errorf("cues[2].Time = %v, want 2.0", p.cues[2].Time)
	}
}

func TestPlayer_StepFiresCuesInOrder(t *testing.T) {
	p := New()
	p.Add(Cue{Time: 0.5, Kind: CueKindCameraChange, Target: "cam1"})
	p.Add(Cue{Time: 1.0, Kind: CueKindSubtitle, Param: "Hello"})
	p.Add(Cue{Time: 1.5, Kind: CueKindEnd})
	p.Start()

	var fired []Cue
	p.OnCue = func(c Cue) { fired = append(fired, c) }

	// Step to 0.4s — no cue should fire.
	p.Step(0.4)
	if len(fired) != 0 {
		t.Errorf("fired at 0.4s = %v, want none", fired)
	}
	// Step to 0.6s — camera change fires.
	p.Step(0.2)
	if len(fired) != 1 || fired[0].Kind != CueKindCameraChange {
		t.Errorf("fired = %v, want camera_change", fired)
	}
	// Step to 1.1s — subtitle fires.
	p.Step(0.5)
	if len(fired) != 2 || fired[1].Kind != CueKindSubtitle {
		t.Errorf("fired = %v, want subtitle", fired)
	}
	// Step to 1.6s — end fires, player is done.
	done := p.Step(0.5)
	if !done {
		t.Error("Step returned false, want true after End cue")
	}
	if len(fired) != 3 || fired[2].Kind != CueKindEnd {
		t.Errorf("fired = %v, want end", fired)
	}
}

func TestPlayer_DoneAfterEnd(t *testing.T) {
	p := New()
	p.Add(Cue{Time: 0.5, Kind: CueKindEnd})
	p.Start()
	p.Step(0.6)
	if !p.Done() {
		t.Fatal("Done = false, want true")
	}
	// Further steps should be no-ops.
	done := p.Step(1.0)
	if !done {
		t.Fatal("Step after Done returned false")
	}
}

func TestPlayer_Reset(t *testing.T) {
	p := New()
	p.Add(Cue{Time: 0.5, Kind: CueKindEnd})
	p.Start()
	p.Step(0.6)
	if !p.Done() {
		t.Fatal("Done = false, want true")
	}
	p.Reset()
	if p.Done() {
		t.Fatal("Done = true after Reset")
	}
	if p.Time() != 0 {
		t.Errorf("Time = %v, want 0", p.Time())
	}
}

func TestCueKind_String(t *testing.T) {
	cases := []struct {
		kind CueKind
		want string
	}{
		{CueKindCameraChange, "camera_change"},
		{CueKindDialogue, "dialogue"},
		{CueKindSubtitle, "subtitle"},
		{CueKindActorMove, "actor_move"},
		{CueKindActorAnimate, "actor_animate"},
		{CueKindMusicChange, "music_change"},
		{CueKindSFX, "sfx"},
		{CueKindEnd, "end"},
		{CueKind(99), "kind(99)"},
	}
	for _, c := range cases {
		if got := c.kind.String(); got != c.want {
			t.Errorf("%v.String() = %q, want %q", c.kind, got, c.want)
		}
	}
}
