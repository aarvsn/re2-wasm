package xa

import (
	"math"
	"testing"
)

func TestDecode_RejectsEmpty(t *testing.T) {
	_, err := Decode(nil, ModeStereo)
	if err == nil {
		t.Fatal("err = nil, want empty-input error")
	}
}

func TestDecode_RejectsBadLength(t *testing.T) {
	_, err := Decode(make([]byte, 100), ModeStereo) // not multiple of 128
	if err == nil {
		t.Fatal("err = nil, want bad-length error")
	}
}

func TestDecode_Silence(t *testing.T) {
	// 128-byte block of all zeros with filter 0 should produce 4*28=112
	// zero samples per channel.
	b := make([]byte, 128)
	out, err := Decode(b, ModeMono)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 112 {
		t.Fatalf("samples = %d, want 112", len(out))
	}
	for i, v := range out {
		if v != 0 {
			t.Errorf("sample %d = %v, want 0", i, v)
			break
		}
	}
}

func TestDecode_SignedNibble(t *testing.T) {
	// All nibbles = 8 (most-negative 4-bit value). With shift=0 and
	// filter=0, each sample should be -8.
	b := make([]byte, 128)
	for i := range b {
		b[i] = 0x88
	}
	// Set the header bytes so shift=0, filter=0.
	b[0] = 0
	b[1] = 0
	out, err := Decode(b, ModeMono)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range out {
		if math.Abs(float64(v-(-8.0))) > 1e-5 {
			t.Errorf("sample %d = %v, want -8", i, v)
			break
		}
	}
}

func TestDecode_StereoProducesTwoChannels(t *testing.T) {
	// In stereo mode, the output length should be 2x the mono length.
	b := make([]byte, 128)
	out, err := Decode(b, ModeStereo)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 224 {
		t.Errorf("stereo samples = %d, want 224", len(out))
	}
}

func TestXAFilter_Table(t *testing.T) {
	cases := []struct {
		idx int
		f0  float32
		f1  float32
	}{
		{0, 0, 0},
		{1, 0.9375, 0},
		{2, 1.796875, -0.8125},
		{3, 1.53125, -0.859375},
		{4, 0, 0}, // unknown -> fallback
		{99, 0, 0},
	}
	for _, c := range cases {
		f0, f1 := xaFilter(c.idx)
		if f0 != c.f0 || f1 != c.f1 {
			t.Errorf("xaFilter(%d) = (%v, %v), want (%v, %v)", c.idx, f0, f1, c.f0, c.f1)
		}
	}
}
