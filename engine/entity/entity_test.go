package entity

import (
	"testing"
)

func TestWorld_SpawnAndGet(t *testing.T) {
	w := NewWorld()
	id := w.Spawn([3]float32{1, 2, 3})
	if id == 0 {
		t.Fatal("Spawn returned 0")
	}
	e := w.Get(id)
	if e == nil {
		t.Fatalf("Get(%d) = nil", id)
	}
	if e.Position != [3]float32{1, 2, 3} {
		t.Errorf("Position = %v, want (1,2,3)", e.Position)
	}
}

func TestWorld_Remove(t *testing.T) {
	w := NewWorld()
	id := w.Spawn([3]float32{0, 0, 0})
	if !w.Remove(id) {
		t.Fatal("Remove returned false")
	}
	if w.Get(id) != nil {
		t.Fatal("Get returned non-nil after Remove")
	}
	if w.Remove(id) {
		t.Fatal("Remove returned true on missing id")
	}
}

func TestWorld_Count(t *testing.T) {
	w := NewWorld()
	if w.Count() != 0 {
		t.Fatalf("Count = %d, want 0", w.Count())
	}
	w.Spawn([3]float32{})
	w.Spawn([3]float32{})
	if w.Count() != 2 {
		t.Fatalf("Count = %d, want 2", w.Count())
	}
}

func TestWorld_AllSnapshot(t *testing.T) {
	w := NewWorld()
	w.Spawn([3]float32{1, 0, 0})
	w.Spawn([3]float32{2, 0, 0})
	all := w.All()
	if len(all) != 2 {
		t.Fatalf("len(All) = %d, want 2", len(all))
	}
}

func TestWorld_TickCallsTickers(t *testing.T) {
	w := NewWorld()
	id := w.Spawn([3]float32{})
	e := w.Get(id)
	ticker := &fakeTicker{}
	e.AddComponent("ai", ticker)
	w.Tick(0.033)
	if ticker.ticks != 1 {
		t.Errorf("ticks = %d, want 1", ticker.ticks)
	}
	if ticker.dt != 0.033 {
		t.Errorf("dt = %v, want 0.033", ticker.dt)
	}
	if ticker.entity != e {
		t.Errorf("entity passed to Tick is wrong")
	}
}

type fakeTicker struct {
	ticks  int
	dt     float32
	entity *Entity
}

func (f *fakeTicker) Tick(e *Entity, dt float32) {
	f.ticks++
	f.dt = dt
	f.entity = e
}

func TestEntity_ComponentHelpers(t *testing.T) {
	e := &Entity{ID: 1, Components: make(map[string]any)}
	if e.HasComponent("health") {
		t.Fatal("HasComponent returned true before Add")
	}
	e.AddComponent("health", &Health{Current: 100, Max: 100})
	if !e.HasComponent("health") {
		t.Fatal("HasComponent returned false after Add")
	}
	c := e.Component("health")
	if c == nil {
		t.Fatal("Component returned nil")
	}
	h, ok := c.(*Health)
	if !ok {
		t.Fatalf("Component is %T, want *Health", c)
	}
	if h.Current != 100 {
		t.Errorf("Current = %d, want 100", h.Current)
	}
}

func TestEntity_MustComponent_Panics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on missing component")
		}
	}()
	e := &Entity{ID: 1}
	e.MustComponent("missing")
}

func TestHealth_DamageAndHeal(t *testing.T) {
	cases := []struct {
		name   string
		h      Health
		damage uint16
		want   uint16
		dead   bool
	}{
		{"no damage", Health{Current: 100, Max: 100}, 0, 100, false},
		{"partial", Health{Current: 100, Max: 100}, 30, 70, false},
		{"exact kill", Health{Current: 100, Max: 100}, 100, 0, true},
		{"overkill", Health{Current: 50, Max: 100}, 80, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := c.h
			h.Damage(c.damage)
			if h.Current != c.want {
				t.Errorf("Current = %d, want %d", h.Current, c.want)
			}
			if h.Dead != c.dead {
				t.Errorf("Dead = %v, want %v", h.Dead, c.dead)
			}
		})
	}
}

func TestHealth_Heal(t *testing.T) {
	h := Health{Current: 50, Max: 100, Dead: false}
	h.Heal(30)
	if h.Current != 80 {
		t.Errorf("Current = %d, want 80", h.Current)
	}
	h.Heal(100)
	if h.Current != 100 {
		t.Errorf("Current = %d, want 100 (clamped)", h.Current)
	}
}

func TestHealth_HealRevives(t *testing.T) {
	h := Health{Current: 0, Max: 100, Dead: true}
	h.Heal(20)
	if h.Dead {
		t.Error("Dead = true, want false after Heal")
	}
	if h.Current != 20 {
		t.Errorf("Current = %d, want 20", h.Current)
	}
}
