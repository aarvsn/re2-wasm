// Package cutscene is RE2's cutscene player. Cutscenes are non-interactive
// sequences (camera changes, scripted dialogue, FMVs) that play between
// gameplay segments. The player is a simple timeline: a list of Cue
// entries each with a start time and a kind; the engine advances the
// timeline every frame and fires callbacks.
//
// Phase 5 ships the timeline + cue types; Phase 6 wires real RE2 cutscene
// scripts (SCD/EDD formats) on top.
package cutscene

import (
	"fmt"
	"sort"
	"sync"
)

// CueKind enumerates the kinds of cue the timeline can fire.
type CueKind int

// Supported cue kinds.
const (
	CueKindCameraChange CueKind = iota
	CueKindDialogue
	CueKindSubtitle
	CueKindActorMove
	CueKindActorAnimate
	CueKindMusicChange
	CueKindSFX
	CueKindEnd
)

// Cue is one entry on the cutscene timeline.
type Cue struct {
	Time   float32 // seconds from cutscene start
	Kind   CueKind
	Target string // entity / camera / audio bank name, depends on Kind
	Param  string // free-form parameter (animation name, subtitle text, ...)
}

// Player is a cutscene timeline player. Create one with New, queue cues
// via Add, and call Step each frame from the engine.
type Player struct {
	mu     sync.Mutex
	cues   []Cue
	cursor int
	time   float32
	done   bool

	// OnCue is invoked for each cue whose time has just been reached.
	// Implementations switch cameras, play dialogue, etc.
	OnCue func(Cue)
}

// New returns an empty Player.
func New() *Player { return &Player{} }

// Add queues a cue. Cues may be added in any order; the Player sorts by
// time when the cutscene starts.
func (p *Player) Add(c Cue) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cues = append(p.cues, c)
}

// Start sorts the cue list by time and resets the cursor.
func (p *Player) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	sort.SliceStable(p.cues, func(i, j int) bool {
		return p.cues[i].Time < p.cues[j].Time
	})
	p.cursor = 0
	p.time = 0
	p.done = false
}

// Step advances the timeline by dt seconds and fires OnCue for every cue
// whose time has been reached. Returns true when the cutscene is over.
func (p *Player) Step(dt float32) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		return true
	}
	p.time += dt
	for p.cursor < len(p.cues) && p.cues[p.cursor].Time <= p.time {
		c := p.cues[p.cursor]
		p.cursor++
		if c.Kind == CueKindEnd {
			p.done = true
			if p.OnCue != nil {
				p.OnCue(c)
			}
			return true
		}
		if p.OnCue != nil {
			p.OnCue(c)
		}
	}
	return p.done
}

// Time returns the current playback time in seconds.
func (p *Player) Time() float32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.time
}

// Done reports whether the cutscene has finished.
func (p *Player) Done() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.done
}

// Reset rewinds the timeline to time 0. Does not clear the cue list.
func (p *Player) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cursor = 0
	p.time = 0
	p.done = false
}

// String returns a debug-friendly description of a cue.
func (c Cue) String() string {
	return fmt.Sprintf("cue{%s @%.2f target=%q param=%q}", c.Kind, c.Time, c.Target, c.Param)
}

// String returns the cue kind's name.
func (k CueKind) String() string {
	switch k {
	case CueKindCameraChange:
		return "camera_change"
	case CueKindDialogue:
		return "dialogue"
	case CueKindSubtitle:
		return "subtitle"
	case CueKindActorMove:
		return "actor_move"
	case CueKindActorAnimate:
		return "actor_animate"
	case CueKindMusicChange:
		return "music_change"
	case CueKindSFX:
		return "sfx"
	case CueKindEnd:
		return "end"
	default:
		return fmt.Sprintf("kind(%d)", int(k))
	}
}
