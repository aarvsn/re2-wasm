// Package i18n is the engine's localisation layer. RE2 ships in English
// and Japanese at minimum; Phase 6 wires this package into the menu / UI
// code so every user-visible string passes through a translator.
//
// The implementation is intentionally tiny: a flat map of (locale, key)
// to translated string, with a fallback chain (requested -> English ->
// key itself). Real translation files (JSON) are loaded at boot.
package i18n

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Locale is a BCP-47 language tag, e.g. "en", "ja", "en-US".
type Locale string

// Default locales RE2 ships in.
const (
	English  Locale = "en"
	Japanese Locale = "ja"
)

// Bundle is a set of translations for one locale.
type Bundle struct {
	Locale Locale
	// Messages maps a key (e.g. "menu.pause.title") to its translation.
	Messages map[string]string
}

// Manager owns every loaded Bundle and the active locale.
type Manager struct {
	mu      sync.RWMutex
	active  Locale
	bundles map[Locale]*Bundle
}

// New returns a Manager pre-loaded with English fallbacks for the keys
// the engine uses at boot.
func New() *Manager {
	m := &Manager{
		active:  English,
		bundles: make(map[Locale]*Bundle),
	}
	m.LoadBundle(defaultEnglishBundle())
	return m
}

// LoadBundle registers a bundle. Existing bundles for the same locale are
// merged (new keys added, existing keys overwritten).
func (m *Manager) LoadBundle(b *Bundle) {
	if b == nil || b.Locale == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.bundles[b.Locale]; ok {
		for k, v := range b.Messages {
			existing.Messages[k] = v
		}
		return
	}
	m.bundles[b.Locale] = b
}

// LoadJSON parses a JSON object of {key: translation} and registers it
// under the given locale.
func (m *Manager) LoadJSON(loc Locale, data []byte) error {
	var msgs map[string]string
	if err := json.Unmarshal(data, &msgs); err != nil {
		return fmt.Errorf("i18n: parse json: %w", err)
	}
	m.LoadBundle(&Bundle{Locale: loc, Messages: msgs})
	return nil
}

// SetActive changes the active locale. Returns an error if no bundle is
// loaded for that locale (callers can still proceed; lookups will fall
// back to English).
func (m *Manager) SetActive(loc Locale) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.bundles[loc]; !ok {
		return fmt.Errorf("i18n: no bundle for locale %q", loc)
	}
	m.active = loc
	return nil
}

// Active returns the current locale.
func (m *Manager) Active() Locale {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// Available returns every locale with a loaded bundle.
func (m *Manager) Available() []Locale {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Locale, 0, len(m.bundles))
	for l := range m.bundles {
		out = append(out, l)
	}
	return out
}

// Get looks up key in the active locale, falling back to English and then
// to key itself. Never returns an empty string.
func (m *Manager) Get(key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if b, ok := m.bundles[m.active]; ok {
		if v, ok := b.Messages[key]; ok && v != "" {
			return v
		}
	}
	if m.active != English {
		if b, ok := m.bundles[English]; ok {
			if v, ok := b.Messages[key]; ok && v != "" {
				return v
			}
		}
	}
	return key
}

// Getf is like Get but formats the result with fmt.Sprintf.
func (m *Manager) Getf(key string, args ...any) string {
	return fmt.Sprintf(m.Get(key), args...)
}

// defaultEnglishBundle returns the keys the engine uses at boot. These
// double as documentation of every user-visible string in the codebase.
func defaultEnglishBundle() *Bundle {
	return &Bundle{
		Locale: English,
		Messages: map[string]string{
			"boot.loading":           "Loading…",
			"boot.ready":             "Ready",
			"boot.mounting":          "Mounting %s",
			"boot.mounted":           "Mounted %s (%d bytes)",
			"error.webgl2_missing":   "WebGL2 is required to run re2-wasm.",
			"error.audio_missing":    "Web Audio API is unavailable; the game will run silently.",
			"error.idb_missing":      "IndexedDB is unavailable; saves will not persist.",
			"menu.pause.title":       "Pause",
			"menu.pause.continue":    "Continue",
			"menu.pause.options":     "Options",
			"menu.pause.quit":        "Quit to Title",
			"menu.inventory.title":   "Inventory",
			"menu.inventory.use":     "Use",
			"menu.inventory.examine": "Examine",
			"menu.inventory.combine": "Combine",
			"menu.inventory.drop":    "Drop",
			"menu.map.title":         "Map",
			"controls.forward":       "Move Forward",
			"controls.backward":      "Move Backward",
			"controls.turn_left":     "Turn Left",
			"controls.turn_right":    "Turn Right",
			"controls.run":           "Run",
			"controls.aim":           "Aim",
			"controls.fire":          "Fire",
			"controls.interact":      "Interact",
			"controls.inventory":     "Inventory",
			"controls.map":           "Map",
			"controls.pause":         "Pause",
		},
	}
}
