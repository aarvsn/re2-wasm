package ai

import (
	"testing"

	"github.com/aarvsn/re2-wasm/engine/entity"
)

func TestTick_DeadIsNoOp(t *testing.T) {
	e := New("zombie")
	e.State = StateDead
	w := entity.NewWorld()
	id := w.Spawn([3]float32{0, 0, 0})
	self := w.Get(id)
	startPos := self.Position
	e.Tick(self, w, 0.033)
	if self.Position != startPos {
		t.Errorf("dead enemy moved: %v -> %v", startPos, self.Position)
	}
}

func TestTick_IdleDoesNotMove(t *testing.T) {
	e := New("zombie")
	w := entity.NewWorld()
	id := w.Spawn([3]float32{0, 0, 0})
	self := w.Get(id)
	e.Tick(self, w, 0.033)
	if self.Position != [3]float32{0, 0, 0} {
		t.Errorf("idle enemy moved: %v", self.Position)
	}
}

func TestTick_PlayerOutOfRangeStaysIdle(t *testing.T) {
	e := New("zombie")
	w := entity.NewWorld()
	playerID := w.Spawn([3]float32{1000, 0, 0}) // far away
	enemyID := w.Spawn([3]float32{0, 0, 0})
	e.Target = playerID
	self := w.Get(enemyID)
	e.Tick(self, w, 0.033)
	if e.State != StateIdle {
		t.Errorf("State = %v, want Idle", e.State)
	}
}

func TestTick_PlayerInRangeTransitionsToChase(t *testing.T) {
	e := New("zombie")
	e.DetectRange = 500
	w := entity.NewWorld()
	playerID := w.Spawn([3]float32{100, 0, 0}) // within range
	enemyID := w.Spawn([3]float32{0, 0, 0})
	e.Target = playerID
	self := w.Get(enemyID)
	e.Tick(self, w, 0.033)
	if e.State != StateChase {
		t.Errorf("State = %v, want Chase", e.State)
	}
}

func TestTick_ChaseMovesTowardPlayer(t *testing.T) {
	e := New("zombie")
	e.DetectRange = 500
	e.setTestState(StateChase, 0)
	w := entity.NewWorld()
	playerID := w.Spawn([3]float32{100, 0, 0})
	enemyID := w.Spawn([3]float32{0, 0, 0})
	e.Target = playerID
	self := w.Get(enemyID)
	// dt=1 so the enemy moves Speed*1.5 = 45 units toward the player.
	e.Tick(self, w, 1.0)
	if self.Position[0] < 40 || self.Position[0] > 50 {
		t.Errorf("X = %v, want ~45 (moved toward player)", self.Position[0])
	}
}

func TestTick_PlayerInAttackRangeTransitionsToAttack(t *testing.T) {
	e := New("zombie")
	e.AttackRange = 50
	e.DetectRange = 500
	w := entity.NewWorld()
	playerID := w.Spawn([3]float32{30, 0, 0}) // within attack range
	enemyID := w.Spawn([3]float32{0, 0, 0})
	e.Target = playerID
	self := w.Get(enemyID)
	e.Tick(self, w, 0.033)
	if e.State != StateAttack {
		t.Errorf("State = %v, want Attack", e.State)
	}
}

func TestTick_AttackDoesNotMove(t *testing.T) {
	e := New("zombie")
	e.setTestState(StateAttack, 0.5)
	w := entity.NewWorld()
	playerID := w.Spawn([3]float32{30, 0, 0})
	enemyID := w.Spawn([3]float32{0, 0, 0})
	e.Target = playerID
	self := w.Get(enemyID)
	startPos := self.Position
	e.Tick(self, w, 0.033)
	if self.Position != startPos {
		t.Errorf("attacking enemy moved: %v -> %v", startPos, self.Position)
	}
}

func TestTick_PatrolBouncesBetweenWaypoints(t *testing.T) {
	e := New("zombie")
	e.setTestState(StatePatrol, 0)
	e.PatrolA = [3]float32{0, 0, 0}
	e.PatrolB = [3]float32{100, 0, 0}
	w := entity.NewWorld()
	id := w.Spawn([3]float32{0, 0, 0})
	self := w.Get(id)
	// Tick several times with large dt so we reach PatrolB.
	for i := 0; i < 5; i++ {
		e.Tick(self, w, 1.0)
	}
	if self.Position[0] < 90 {
		t.Errorf("after patrol, X = %v, want near 100", self.Position[0])
	}
}

func TestMoveTowards_SnapsWhenClose(t *testing.T) {
	e := &entity.Entity{Position: [3]float32{95, 0, 0}}
	moveTowards(e, [3]float32{100, 0, 0}, 30, 1.0) // step=30, dist=5
	if e.Position[0] != 100 {
		t.Errorf("X = %v, want 100 (snap)", e.Position[0])
	}
}

func TestMoveTowards_FacesDirection(t *testing.T) {
	e := &entity.Entity{Position: [3]float32{0, 0, 0}}
	moveTowards(e, [3]float32{0, 0, 100}, 30, 1.0) // moving +Z
	// Atan2(-dx, -dz) = Atan2(0, -100) = π. Facing -Z (forward at yaw=π
	// in RE2's convention moves toward +Z). Allow either ±π.
	if e.Rotation < 3.0 && e.Rotation > -3.0 {
		// closer to 0 than to ±π — probably wrong
		t.Errorf("Rotation = %v, want near ±π", e.Rotation)
	}
}

func TestDistance(t *testing.T) {
	cases := []struct {
		a, b [3]float32
		want float32
	}{
		{[3]float32{0, 0, 0}, [3]float32{0, 0, 0}, 0},
		{[3]float32{3, 4, 0}, [3]float32{0, 0, 0}, 5},
		{[3]float32{0, 0, 0}, [3]float32{0, 0, 5}, 5},
	}
	for _, c := range cases {
		if got := distance(c.a, c.b); got != c.want {
			t.Errorf("distance(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestState_String(t *testing.T) {
	cases := []struct {
		s    State
		want string
	}{
		{StateIdle, "idle"},
		{StatePatrol, "patrol"},
		{StateAlert, "alert"},
		{StateChase, "chase"},
		{StateAttack, "attack"},
		{StateStagger, "stagger"},
		{StateDead, "dead"},
		{State(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("%v.String() = %q, want %q", c.s, got, c.want)
		}
	}
}
