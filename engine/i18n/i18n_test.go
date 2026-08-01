package i18n

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNew_HasEnglishDefaults(t *testing.T) {
	m := New()
	if m.Active() != English {
		t.Fatalf("Active = %q, want en", m.Active())
	}
	if got := m.Get("menu.pause.title"); got != "Pause" {
		t.Errorf("Get(menu.pause.title) = %q, want Pause", got)
	}
}

func TestGet_FallsBackToEnglish(t *testing.T) {
	m := New()
	m.LoadBundle(&Bundle{
		Locale:   Japanese,
		Messages: map[string]string{"menu.pause.title": "一時停止"},
	})
	if err := m.SetActive(Japanese); err != nil {
		t.Fatal(err)
	}
	if got := m.Get("menu.pause.title"); got != "一時停止" {
		t.Errorf("Get = %q, want 一時停止", got)
	}
	// Key missing from ja but present in en — should fall back.
	if got := m.Get("menu.pause.continue"); got != "Continue" {
		t.Errorf("Get = %q, want Continue (fallback)", got)
	}
}

func TestGet_FallsBackToKey(t *testing.T) {
	m := New()
	if got := m.Get("nonexistent.key"); got != "nonexistent.key" {
		t.Errorf("Get = %q, want the key itself", got)
	}
}

func TestGetf_Formats(t *testing.T) {
	m := New()
	m.LoadBundle(&Bundle{
		Locale:   English,
		Messages: map[string]string{"test.greeting": "Hello, %s!"},
	})
	got := m.Getf("test.greeting", "Leon")
	if got != "Hello, Leon!" {
		t.Errorf("Getf = %q, want 'Hello, Leon!'", got)
	}
}

func TestLoadJSON(t *testing.T) {
	m := New()
	data, _ := json.Marshal(map[string]string{
		"menu.pause.title": "Pause (custom)",
	})
	if err := m.LoadJSON(English, data); err != nil {
		t.Fatal(err)
	}
	if got := m.Get("menu.pause.title"); got != "Pause (custom)" {
		t.Errorf("Get = %q, want 'Pause (custom)'", got)
	}
}

func TestLoadJSON_RejectsBadJSON(t *testing.T) {
	m := New()
	err := m.LoadJSON(English, []byte("not json"))
	if err == nil || !strings.Contains(err.Error(), "parse json") {
		t.Fatalf("err = %v, want parse-json error", err)
	}
}

func TestSetActive_RejectsUnknownLocale(t *testing.T) {
	m := New()
	if err := m.SetActive(Locale("klingon")); err == nil {
		t.Fatal("SetActive returned nil, want error for unknown locale")
	}
}

func TestAvailable_IncludesLoadedLocales(t *testing.T) {
	m := New()
	m.LoadBundle(&Bundle{Locale: Japanese, Messages: map[string]string{}})
	avail := m.Available()
	found := false
	for _, l := range avail {
		if l == Japanese {
			found = true
		}
	}
	if !found {
		t.Errorf("Available = %v, want ja included", avail)
	}
}

func TestLoadBundle_MergesExistingKeys(t *testing.T) {
	m := New()
	m.LoadBundle(&Bundle{Locale: English, Messages: map[string]string{"a": "1"}})
	m.LoadBundle(&Bundle{Locale: English, Messages: map[string]string{"b": "2"}})
	if m.Get("a") != "1" {
		t.Error("key a lost after merge")
	}
	if m.Get("b") != "2" {
		t.Error("key b not added after merge")
	}
}
