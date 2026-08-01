// Package player implements RE2's player controller. The controller reads
// the active action set from the input package and applies movement /
// rotation to the player entity. The math matches the original game's
// tank-style controls: forward/backward moves along the facing direction,
// left/right rotates the player.
//
// The controller is decoupled from input so it can be tested on the host
// with a synthetic action set.
package player

import (
	"math"

	"github.com/aarvsn/re2-wasm/engine/entity"
)

// Speeds are RE2's per-second movement values, derived from the original
// engine's frame-scaled constants (which assume 30 Hz).
const (
	WalkSpeed    float32 = 80.0 // units per second
	RunSpeed     float32 = 160.0
	TurnSpeed    float32 = 2.5 // radians per second
	RunTurnSpeed float32 = 1.8
)

// ActionSet is the set of actions currently held. The engine's input
// layer fills this from input.Binder.ActiveActions.
type ActionSet struct {
	MoveForward  bool
	MoveBackward bool
	TurnLeft     bool
	TurnRight    bool
	Run          bool
}

// Controller ticks one player entity. Construct with New.
type Controller struct {
	EntityID entity.ID
}

// New returns a Controller for the given entity.
func New(id entity.ID) *Controller { return &Controller{EntityID: id} }

// Step advances the player by dt seconds given the held action set. The
// world's entity table is read for the current position/rotation; if the
// entity is missing the call is a no-op.
func (c *Controller) Step(w *entity.World, as ActionSet, dt float32) {
	e := w.Get(c.EntityID)
	if e == nil {
		return
	}
	speed := WalkSpeed
	turn := TurnSpeed
	if as.Run {
		speed = RunSpeed
		turn = RunTurnSpeed
	}
	// Rotation is independent of movement so the player can turn in place.
	if as.TurnLeft {
		e.Rotation += turn * dt
	}
	if as.TurnRight {
		e.Rotation -= turn * dt
	}
	// Movement is along the facing direction (tank controls).
	if as.MoveForward {
		e.Position[0] -= sin32(e.Rotation) * speed * dt
		e.Position[2] -= cos32(e.Rotation) * speed * dt
	}
	if as.MoveBackward {
		e.Position[0] += sin32(e.Rotation) * speed * dt
		e.Position[2] += cos32(e.Rotation) * speed * dt
	}
}

// sin32 / cos32 are float32 wrappers around math.Sin / math.Cos.
func sin32(x float32) float32 {
	return float32(math.Sin(float64(x)))
}
func cos32(x float32) float32 {
	return float32(math.Cos(float64(x)))
}
