package filesystem

import (
	"strings"
	"testing"
)

func TestMemoryFS_MountAndRead(t *testing.T) {
	cases := []struct {
		name    string
		mountAs string
		readAs  string
		payload []byte
	}{
		{"simple", "stage1/room1.tim", "stage1/room1.tim", []byte{1, 2, 3}},
		{"backslash", "STAGE1\\ROOM1.TIM", "stage1/room1.tim", []byte{9}},
		{"leading_slash", "/stage1/room1.tim", "stage1/room1.tim", []byte{0}},
		{"upper", "STAGE1/ROOM1.TIM", "stage1/room1.tim", []byte{4, 5, 6, 7}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := New()
			if err := m.Mount(c.mountAs, c.payload); err != nil {
				t.Fatalf("Mount: %v", err)
			}
			if !m.Has(c.readAs) {
				t.Fatalf("Has(%q) = false, want true", c.readAs)
			}
			got, err := m.Read(c.readAs)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if string(got) != string(c.payload) {
				t.Fatalf("Read = %v, want %v", got, c.payload)
			}
		})
	}
}

func TestMemoryFS_MountErrors(t *testing.T) {
	m := New()
	cases := []struct {
		name string
		path string
		data []byte
	}{
		{"empty_name", "", []byte{1}},
		{"nil_data", "x", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := m.Mount(c.path, c.data)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestMemoryFS_ReadNotFound(t *testing.T) {
	m := New()
	_, err := m.Read("nope")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want a not-found error", err)
	}
}

func TestMemoryFS_ReadReturnsDefensiveCopy(t *testing.T) {
	m := New()
	orig := []byte{1, 2, 3}
	_ = m.Mount("a", orig)
	got, _ := m.Read("a")
	got[0] = 99
	got2, _ := m.Read("a")
	if got2[0] != 1 {
		t.Fatalf("mutation leaked into store: got2[0] = %d", got2[0])
	}
}

func TestMemoryFS_ListSorted(t *testing.T) {
	m := New()
	_ = m.Mount("b", []byte{1})
	_ = m.Mount("a", []byte{1})
	_ = m.Mount("c", []byte{1})
	got := m.List()
	want := []string{"a", "b", "c"}
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("List = %v, want %v", got, want)
	}
}

func TestMemoryFS_Remove(t *testing.T) {
	m := New()
	_ = m.Mount("a", []byte{1})
	if !m.Remove("a") {
		t.Fatal("Remove returned false on existing key")
	}
	if m.Has("a") {
		t.Fatal("Has returned true after Remove")
	}
	if m.Remove("a") {
		t.Fatal("Remove returned true on missing key")
	}
}

func TestMemoryFS_Size(t *testing.T) {
	m := New()
	_ = m.Mount("a", make([]byte, 100))
	_ = m.Mount("b", make([]byte, 200))
	if got, want := m.Size(), int64(300); got != want {
		t.Fatalf("Size = %d, want %d", got, want)
	}
}

func TestNormalise(t *testing.T) {
	cases := []struct{ in, want string }{
		{"STAGE1\\ROOM.TIM", "stage1/room.tim"},
		{"/stage1/room.tim", "stage1/room.tim"},
		{"./stage1/room.tim", "stage1/room.tim"},
		{"stage1//room.tim", "stage1/room.tim"},
	}
	for _, c := range cases {
		if got := normalise(c.in); got != c.want {
			t.Errorf("normalise(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
