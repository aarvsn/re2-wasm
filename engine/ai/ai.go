// Package ai implements RE2's enemy AI as a small finite-state machine.
// Each enemy type (zombie, licker, dog, crow, ...) has its own FSM; the
// engine ticks every enemy once per simulation step.
//
// The package is decoupled from input and rendering so the same FSMs run
// in host tests and in the browser.
package ai

import (
	"math"

	"github.com/aarvsn/re2-wasm/engine/entity"
)

// State is one FSM node.
type State int

// Supported states (a subset; RE2 has more but these cover the common
// zombie behaviour).
const (
	StateIdle State = iota
	StatePatrol
	StateAlert
	StateChase
	StateAttack
	StateStagger
	StateDead
)

// String returns the state's name for debugging.
func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StatePatrol:
		return "patrol"
	case StateAlert:
		return "alert"
	case StateChase:
		return "chase"
	case StateAttack:
		return "attack"
	case StateStagger:
		return "stagger"
	case StateDead:
		return "dead"
	default:
		return "unknown"
	}
}

// Enemy is the AI component attached to an entity. The entity's position
// and rotation are read/written by the controller; the AI's state lives
// here.
type Enemy struct {
	Kind        string // "zombie", "licker", ...
	State       State
	Target      entity.ID // player entity, or 0 if no target
	Speed       float32
	AttackRange float32
	DetectRange float32

	// State timers, in seconds. Counts down each tick; at 0 the FSM
	// re-evaluates.
	stateTimer float32

	// Patrol route; the enemy walks between PatrolA and PatrolB.
	PatrolA, PatrolB [3]float32
	patrolToB        bool
}

// New returns a zombie enemy with RE2-default stats.
func New(kind string) *Enemy {
	return &Enemy{
		Kind:        kind,
		State:       StateIdle,
		Speed:       30.0,
		AttackRange: 30.0,
		DetectRange: 400.0,
	}
}

// Tick advances the FSM by dt seconds. The world is read for the player's
// position; the enemy's entity is mutated to move it.
func (e *Enemy) Tick(self *entity.Entity, w *entity.World, dt float32) {
	if e.State == StateDead {
		return
	}
	e.stateTimer -= dt
	if e.stateTimer > 0 {
		// Continue current behaviour without re-evaluating.
		e.act(self, w, dt)
		return
	}

	// Re-evaluate.
	player := w.Get(e.Target)
	if player != nil {
		dist := distance(self.Position, player.Position)
		switch {
		case dist <= e.AttackRange && e.State != StateAttack:
			e.setState(StateAttack, 0.5)
		case dist <= e.DetectRange && e.State != StateChase:
			e.setState(StateChase, 0)
		case dist > e.DetectRange && (e.State == StateChase || e.State == StateAlert):
			e.setState(StatePatrol, 0)
		}
	}
	e.act(self, w, dt)
}

// setState transitions to s and resets the state timer.
func (e *Enemy) setState(s State, timer float32) {
	e.State = s
	e.stateTimer = timer
}

// act performs the per-frame movement for the current state.
func (e *Enemy) act(self *entity.Entity, w *entity.World, dt float32) {
	player := w.Get(e.Target)
	switch e.State {
	case StateIdle:
		// stand still
	case StatePatrol:
		target := e.PatrolA
		if e.patrolToB {
			target = e.PatrolB
		}
		moveTowards(self, target, e.Speed, dt)
		if distance(self.Position, target) < 5 {
			e.patrolToB = !e.patrolToB
		}
	case StateChase:
		if player != nil {
			moveTowards(self, player.Position, e.Speed*1.5, dt)
		}
	case StateAttack:
		// Attack animation would play here; for Phase 5 we just stop.
	case StateStagger:
		// No movement; timer expires and FSM re-evaluates.
	case StateDead:
		// No movement.
	}
}

// setStateField exposes setState for tests in the same package.
func (e *Enemy) setTestState(s State, timer float32) { e.setState(s, timer) }

// distance returns the Euclidean distance between two 3-vectors.
func distance(a, b [3]float32) float32 {
	dx := a[0] - b[0]
	dy := a[1] - b[1]
	dz := a[2] - b[2]
	return float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
}

// moveTowards moves self toward target at the given speed. If self is
// already within speed*dt of target, it snaps to target.
func moveTowards(self *entity.Entity, target [3]float32, speed, dt float32) {
	dx := target[0] - self.Position[0]
	dz := target[2] - self.Position[2]
	dist := float32(math.Sqrt(float64(dx*dx + dz*dz)))
	step := speed * dt
	if dist <= step {
		self.Position[0] = target[0]
		self.Position[2] = target[2]
		return
	}
	self.Position[0] += dx / dist * step
	self.Position[2] += dz / dist * step
	// Face the direction of motion.
	if dx != 0 || dz != 0 {
		self.Rotation = float32(math.Atan2(float64(-dx), float64(-dz)))
	}
}
