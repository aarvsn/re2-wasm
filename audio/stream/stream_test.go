package stream

import (
	"errors"
	"testing"
)

func TestRingBuffer_PushPop(t *testing.T) {
	r := NewRingBuffer(3)
	if !r.Push([]float32{1, 2, 3}) {
		t.Fatal("Push returned false")
	}
	if !r.Push([]float32{4, 5, 6}) {
		t.Fatal("Push returned false")
	}
	if r.Len() != 2 {
		t.Fatalf("Len = %d, want 2", r.Len())
	}
	c := r.Pop()
	if c[0] != 1 || c[2] != 3 {
		t.Errorf("first Pop = %v, want [1 2 3]", c)
	}
	c = r.Pop()
	if c[0] != 4 {
		t.Errorf("second Pop = %v, want [4 5 6]", c)
	}
	if r.Pop() != nil {
		t.Error("Pop on empty ring returned non-nil")
	}
}

func TestRingBuffer_RejectsWhenFull(t *testing.T) {
	r := NewRingBuffer(2)
	r.Push([]float32{1})
	r.Push([]float32{2})
	if r.Push([]float32{3}) {
		t.Fatal("Push returned true on full ring")
	}
}

func TestRingBuffer_Close(t *testing.T) {
	r := NewRingBuffer(2)
	r.Close()
	if !r.Closed() {
		t.Error("Closed = false after Close")
	}
	if r.Push([]float32{1}) {
		t.Error("Push returned true on closed ring")
	}
}

func TestRingBuffer_MinimumCapacity(t *testing.T) {
	r := NewRingBuffer(0)
	if r.capacity != 2 {
		t.Errorf("capacity = %d, want 2 (clamped)", r.capacity)
	}
}

func TestStreamer_PumpAndFinish(t *testing.T) {
	callCount := 0
	decode := func(b []byte) ([]float32, error) {
		callCount++
		return []float32{float32(b[0])}, nil
	}
	s := New(5, decode)
	ok, err := s.Pump([]byte{42})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Pump returned false on first call")
	}
	if callCount != 1 {
		t.Errorf("decode called %d times, want 1", callCount)
	}
	c := s.Ring().Pop()
	if c[0] != 42 {
		t.Errorf("decoded = %v, want [42]", c)
	}
	s.Finish()
	if !s.Ring().Closed() {
		t.Error("Ring not closed after Finish")
	}
}

func TestStreamer_PumpPropagatesError(t *testing.T) {
	decode := func(b []byte) ([]float32, error) {
		return nil, errors.New("bad data")
	}
	s := New(5, decode)
	_, err := s.Pump([]byte{1})
	if err == nil {
		t.Fatal("err = nil, want decode error")
	}
}

func TestStreamer_PumpAfterFinish(t *testing.T) {
	decode := func(b []byte) ([]float32, error) {
		return []float32{1}, nil
	}
	s := New(5, decode)
	s.Finish()
	ok, err := s.Pump([]byte{1})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("Pump returned true after Finish")
	}
}

func TestErrNoMoreInput(t *testing.T) {
	if ErrNoMoreInput.Error() != "stream: no more input" {
		t.Errorf("ErrNoMoreInput message wrong")
	}
}
