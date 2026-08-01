// Package menu implements RE2's menu state machine. The original game
// has several full-screen overlays (Pause, Inventory, Map, File viewer,
// Options) that stack on top of gameplay. Each one is a Mode in this
// package; the Manager handles transitions, input routing, and rendering
// hints.
//
// The Manager is intentionally pure: it does not call syscall/js. The
// WASM runtime renders the active menu based on the Manager's exposed
// state.
package menu

import (
	"fmt"
)

// Mode is the active menu overlay.
type Mode int

// Supported modes.
const (
	ModeNone Mode = iota
	ModePause
	ModeInventory
	ModeMap
	ModeFile
	ModeOptions
)

// String returns the mode's name for debugging / rendering.
func (m Mode) String() string {
	switch m {
	case ModeNone:
		return "none"
	case ModePause:
		return "pause"
	case ModeInventory:
		return "inventory"
	case ModeMap:
		return "map"
	case ModeFile:
		return "file"
	case ModeOptions:
		return "options"
	default:
		return fmt.Sprintf("mode(%d)", int(m))
	}
}

// Item is one entry in a menu list.
type Item struct {
	Label string
	// Disabled items are greyed out; e.g. "Use" when nothing is selected.
	Disabled bool
	// OnSelect is invoked when the user picks this item. May be nil.
	OnSelect func()
}

// Page is a single menu page. It has a title, a list of items, and the
// index of the currently-highlighted item.
type Page struct {
	Title    string
	Items    []Item
	Selected int
}

// Manager owns the menu stack. Push to open a menu, Pop to close it, and
// Active to read the topmost page.
type Manager struct {
	stack []Page
	mode  Mode
}

// New returns an empty Manager.
func New() *Manager { return &Manager{mode: ModeNone} }

// Active returns the topmost page, or nil if the stack is empty.
func (m *Manager) Active() *Page {
	if len(m.stack) == 0 {
		return nil
	}
	return &m.stack[len(m.stack)-1]
}

// Mode returns the active menu mode.
func (m *Manager) Mode() Mode { return m.mode }

// Push opens a new menu page and switches to the given mode.
func (m *Manager) Push(mode Mode, page Page) {
	if page.Selected < 0 || (len(page.Items) > 0 && page.Selected >= len(page.Items)) {
		page.Selected = 0
	}
	m.stack = append(m.stack, page)
	m.mode = mode
}

// Pop closes the topmost menu. Returns true if something was popped.
func (m *Manager) Pop() bool {
	if len(m.stack) == 0 {
		return false
	}
	m.stack = m.stack[:len(m.stack)-1]
	if len(m.stack) == 0 {
		m.mode = ModeNone
	}
	return true
}

// CloseAll clears the entire stack.
func (m *Manager) CloseAll() {
	m.stack = nil
	m.mode = ModeNone
}

// Move changes the selected item by delta (typically -1 for up, +1 for
// down). Wraps around. No-op if there are no items or no items are
// enabled.
func (m *Manager) Move(delta int) {
	p := m.Active()
	if p == nil || len(p.Items) == 0 {
		return
	}
	n := len(p.Items)
	// Find the next enabled item, wrapping at most n times.
	for i := 0; i < n; i++ {
		p.Selected = (p.Selected + delta + n) % n
		if !p.Items[p.Selected].Disabled {
			return
		}
	}
}

// Confirm invokes the OnSelect callback of the currently-selected item.
// Returns true if an action was invoked.
func (m *Manager) Confirm() bool {
	p := m.Active()
	if p == nil || len(p.Items) == 0 {
		return false
	}
	item := &p.Items[p.Selected]
	if item.Disabled || item.OnSelect == nil {
		return false
	}
	item.OnSelect()
	return true
}

// PauseMenu returns the standard pause-menu page.
func PauseMenu() Page {
	return Page{
		Title: "Pause",
		Items: []Item{
			{Label: "Continue"},
			{Label: "Options"},
			{Label: "Quit to Title"},
		},
	}
}

// InventoryMenu returns a placeholder inventory page. Phase 5 wires real
// item data via the engine's inventory component.
func InventoryMenu() Page {
	return Page{
		Title: "Inventory",
		Items: []Item{
			{Label: "Use"},
			{Label: "Examine"},
			{Label: "Combine"},
			{Label: "Drop", Disabled: true},
		},
	}
}

// MapMenu returns a placeholder map page. The full map UI is Phase 5+.
func MapMenu() Page {
	return Page{
		Title: "Map",
		Items: []Item{
			{Label: "Floor 1F"},
			{Label: "Floor 2F"},
			{Label: "Floor B1"},
		},
	}
}
