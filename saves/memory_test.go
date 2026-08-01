package saves

import (
	"context"
	"errors"
	"testing"
)

func TestMemStore_SaveLoadRoundTrip(t *testing.T) {
	s := NewMemStore()
	ctx := context.Background()
	cases := []struct {
		slot int
		data []byte
	}{
		{0, []byte("save A")},
		{5, []byte("save B")},
		{19, make([]byte, 8192)},
	}
	for _, c := range cases {
		if err := s.Save(ctx, c.slot, c.data); err != nil {
			t.Fatalf("Save(%d): %v", c.slot, err)
		}
		got, err := s.Load(ctx, c.slot)
		if err != nil {
			t.Fatalf("Load(%d): %v", c.slot, err)
		}
		if string(got) != string(c.data) {
			t.Fatalf("Load(%d) = %q, want %q", c.slot, got, c.data)
		}
	}
}

func TestMemStore_LoadEmpty(t *testing.T) {
	s := NewMemStore()
	_, err := s.Load(context.Background(), 3)
	var empty ErrSlotEmpty
	if !errors.As(err, &empty) {
		t.Fatalf("err = %v, want ErrSlotEmpty", err)
	}
	if empty.Slot != 3 {
		t.Fatalf("ErrSlotEmpty.Slot = %d, want 3", empty.Slot)
	}
}

func TestMemStore_SlotRange(t *testing.T) {
	s := NewMemStore()
	ctx := context.Background()
	cases := []struct {
		slot int
		ok   bool
	}{
		{-1, false},
		{0, true},
		{19, true},
		{20, false},
		{100, false},
	}
	for _, c := range cases {
		err := s.Save(ctx, c.slot, []byte{1})
		if (err == nil) != c.ok {
			t.Errorf("Save(%d): err=%v, want ok=%v", c.slot, err, c.ok)
		}
	}
}

func TestMemStore_List(t *testing.T) {
	s := NewMemStore()
	ctx := context.Background()
	_ = s.Save(ctx, 1, []byte("a"))
	_ = s.Save(ctx, 5, []byte("b"))
	_ = s.Save(ctx, 10, []byte("c"))
	got, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("List = %v, want 3 entries", got)
	}
}

func TestMemStore_ImportExport(t *testing.T) {
	s := NewMemStore()
	want := []byte("imported")
	if err := s.Import(7, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Export(7)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("Export = %q, want %q", got, want)
	}
}

func TestMemStore_SaveRejectsNil(t *testing.T) {
	s := NewMemStore()
	if err := s.Save(context.Background(), 0, nil); err == nil {
		t.Fatal("expected error on nil data")
	}
}

func TestMemStore_LoadReturnsDefensiveCopy(t *testing.T) {
	s := NewMemStore()
	ctx := context.Background()
	_ = s.Save(ctx, 0, []byte{1, 2, 3})
	a, _ := s.Load(ctx, 0)
	a[0] = 99
	b, _ := s.Load(ctx, 0)
	if b[0] != 1 {
		t.Fatalf("mutation leaked: b[0] = %d", b[0])
	}
}
