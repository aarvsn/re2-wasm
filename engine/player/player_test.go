package player

import (
	"math"
	"testing"

	"github.com/aarvsn/re2-wasm/engine/entity"
)

func TestStep_NoOpWhenEntityMissing(t *testing.T) {
	w := entity.NewWorld()
	c := New(999) // not spawned
	c.Step(w, ActionSet{MoveForward: true}, 0.033)
	// Should not panic.
}

func TestStep_ForwardMovesAlongFacing(t *testing.T) {
	w := entity.NewWorld()
	id := w.Spawn([3]float32{0, 0, 0})
	e := w.Get(id)
	e.Rotation = 0 // facing -Z
	c := New(id)
	c.Step(w, ActionSet{MoveForward: true}, 1.0)
	// Rotation = 0 means forward is -Z, so X should be 0 and Z should be -WalkSpeed.
	if math.Abs(float64(e.Position[0])) > 1e-5 {
		t.Errorf("X = %v, want 0", e.Position[0])
	}
	if math.Abs(float64(e.Position[2]-(-WalkSpeed))) > 1e-3 {
		t.Errorf("Z = %v, want %v", e.Position[2], -WalkSpeed)
	}
}

func TestStep_BackwardIsInverseOfForward(t *testing.T) {
	w := entity.NewWorld()
	id := w.Spawn([3]float32{0, 0, 0})
	e := w.Get(id)
	e.Rotation = 0
	c := New(id)
	c.Step(w, ActionSet{MoveBackward: true}, 1.0)
	if math.Abs(float64(e.Position[2]-WalkSpeed)) > 1e-3 {
		t.Errorf("Z = %v, want %v", e.Position[2], WalkSpeed)
	}
}

func TestStep_TurnLeftIncrementsRotation(t *testing.T) {
	w := entity.NewWorld()
	id := w.Spawn([3]float32{})
	e := w.Get(id)
	c := New(id)
	c.Step(w, ActionSet{TurnLeft: true}, 1.0)
	if math.Abs(float64(e.Rotation-TurnSpeed)) > 1e-5 {
		t.Errorf("Rotation = %v, want %v", e.Rotation, TurnSpeed)
	}
}

func TestStep_TurnRightDecrementsRotation(t *testing.T) {
	w := entity.NewWorld()
	id := w.Spawn([3]float32{})
	e := w.Get(id)
	c := New(id)
	c.Step(w, ActionSet{TurnRight: true}, 1.0)
	if math.Abs(float64(e.Rotation-(-TurnSpeed))) > 1e-5 {
		t.Errorf("Rotation = %v, want %v", e.Rotation, -TurnSpeed)
	}
}

func TestStep_RunIsFaster(t *testing.T) {
	w := entity.NewWorld()
	id := w.Spawn([3]float32{})
	walk := w.Get(id)
	cWalk := New(id)
	cWalk.Step(w, ActionSet{MoveForward: true}, 1.0)
	walkZ := walk.Position[2]

	id2 := w.Spawn([3]float32{})
	run := w.Get(id2)
	cRun := New(id2)
	cRun.Step(w, ActionSet{MoveForward: true, Run: true}, 1.0)
	runZ := run.Position[2]

	if math.Abs(float64(runZ)) <= math.Abs(float64(walkZ)) {
		t.Errorf("runZ=%v walkZ=%v, run should be further", runZ, walkZ)
	}
}

func TestStep_Rotation90Degrees(t *testing.T) {
	w := entity.NewWorld()
	id := w.Spawn([3]float32{})
	e := w.Get(id)
	e.Rotation = float32(math.Pi / 2) // facing -X
	c := New(id)
	c.Step(w, ActionSet{MoveForward: true}, 1.0)
	// Forward at 90° rotation: X = -WalkSpeed, Z = 0
	if math.Abs(float64(e.Position[0]-(-WalkSpeed))) > 1e-3 {
		t.Errorf("X = %v, want %v", e.Position[0], -WalkSpeed)
	}
	if math.Abs(float64(e.Position[2])) > 1e-3 {
		t.Errorf("Z = %v, want 0", e.Position[2])
	}
}
