package menu

import "testing"

func TestManager_PushPop(t *testing.T) {
	m := New()
	if m.Active() != nil {
		t.Fatal("Active() should be nil on a fresh Manager")
	}
	if m.Mode() != ModeNone {
		t.Fatalf("Mode = %v, want ModeNone", m.Mode())
	}
	m.Push(ModePause, PauseMenu())
	if m.Mode() != ModePause {
		t.Fatalf("Mode = %v, want ModePause", m.Mode())
	}
	p := m.Active()
	if p == nil || p.Title != "Pause" {
		t.Fatalf("Active = %+v, want Pause page", p)
	}
	if !m.Pop() {
		t.Fatal("Pop returned false")
	}
	if m.Active() != nil {
		t.Fatal("Active() should be nil after Pop empties the stack")
	}
}

func TestManager_CloseAll(t *testing.T) {
	m := New()
	m.Push(ModePause, PauseMenu())
	m.Push(ModeInventory, InventoryMenu())
	m.CloseAll()
	if m.Mode() != ModeNone {
		t.Fatalf("Mode = %v, want ModeNone", m.Mode())
	}
	if m.Active() != nil {
		t.Fatal("Active() should be nil after CloseAll")
	}
}

func TestManager_Move_Wraps(t *testing.T) {
	m := New()
	m.Push(ModePause, PauseMenu()) // 3 items
	m.Move(1)                      // 0 -> 1
	if m.Active().Selected != 1 {
		t.Errorf("Selected = %d, want 1", m.Active().Selected)
	}
	m.Move(1) // 1 -> 2
	if m.Active().Selected != 2 {
		t.Errorf("Selected = %d, want 2", m.Active().Selected)
	}
	m.Move(1) // 2 -> 0 (wrap)
	if m.Active().Selected != 0 {
		t.Errorf("Selected = %d, want 0 (wrap)", m.Active().Selected)
	}
	m.Move(-1) // 0 -> 2 (wrap backwards)
	if m.Active().Selected != 2 {
		t.Errorf("Selected = %d, want 2 (wrap back)", m.Active().Selected)
	}
}

func TestManager_Move_SkipsDisabled(t *testing.T) {
	m := New()
	m.Push(ModeInventory, Page{
		Title: "Test",
		Items: []Item{
			{Label: "A"},
			{Label: "B", Disabled: true},
			{Label: "C"},
		},
	})
	m.Move(1) // 0 -> 2 (skips 1)
	if m.Active().Selected != 2 {
		t.Errorf("Selected = %d, want 2 (skip disabled)", m.Active().Selected)
	}
}

func TestManager_Confirm(t *testing.T) {
	called := false
	m := New()
	m.Push(ModePause, Page{
		Title: "Test",
		Items: []Item{
			{Label: "A", OnSelect: func() { called = true }},
			{Label: "B", Disabled: true},
		},
	})
	if !m.Confirm() {
		t.Fatal("Confirm returned false")
	}
	if !called {
		t.Fatal("OnSelect was not called")
	}
	// Force-select the disabled item (bypass Move's skip logic) and
	// confirm Confirm refuses to invoke it.
	m.Active().Selected = 1
	if m.Confirm() {
		t.Fatal("Confirm returned true on disabled item")
	}
}

func TestManager_PushClampsSelected(t *testing.T) {
	m := New()
	m.Push(ModePause, Page{
		Title:    "Test",
		Items:    []Item{{Label: "A"}, {Label: "B"}},
		Selected: 99, // out of range
	})
	if m.Active().Selected != 0 {
		t.Errorf("Selected = %d, want 0 (clamped)", m.Active().Selected)
	}
}

func TestMode_String(t *testing.T) {
	cases := []struct {
		mode Mode
		want string
	}{
		{ModeNone, "none"},
		{ModePause, "pause"},
		{ModeInventory, "inventory"},
		{ModeMap, "map"},
		{ModeFile, "file"},
		{ModeOptions, "options"},
		{Mode(99), "mode(99)"},
	}
	for _, c := range cases {
		if got := c.mode.String(); got != c.want {
			t.Errorf("%v.String() = %q, want %q", c.mode, got, c.want)
		}
	}
}
