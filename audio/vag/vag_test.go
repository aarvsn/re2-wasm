package vag

import (
	"encoding/binary"
	"math"
	"testing"
)

// buildVAG constructs a synthetic VAG file with the given ADPCM blocks.
// The header is minimal but valid; the audio data is what the test cares
// about.
func buildVAG(blocks [][]byte, sampleRate uint32) []byte {
	const headerSize = 48
	audioLen := 0
	for _, b := range blocks {
		audioLen += len(b)
	}
	out := make([]byte, headerSize+audioLen)
	copy(out[0:4], MagicVAG)
	binary.BigEndian.PutUint32(out[4:8], 3) // version
	binary.BigEndian.PutUint32(out[12:16], uint32(audioLen))
	if sampleRate == 0 {
		sampleRate = SampleRate
	}
	binary.BigEndian.PutUint32(out[16:20], sampleRate)
	off := headerSize
	for _, b := range blocks {
		copy(out[off:], b)
		off += len(b)
	}
	return out
}

// makeBlock builds a 16-byte VAG block with the given shift, filter, and
// 14 bytes of nibble data. The 2-byte header (shift+filter, flags) is
// built so the decoder reads the values the test expects.
func makeBlock(shift, filter int, nibbles []byte) []byte {
	if len(nibbles) != 14 {
		panic("nibbles must be 14 bytes")
	}
	out := make([]byte, 16)
	out[0] = byte(shift&0x0F) | byte((filter&0x0F)<<4)
	out[1] = 0 // flags
	copy(out[2:], nibbles)
	return out
}

func TestDecode_BadMagic(t *testing.T) {
	b := make([]byte, 64)
	copy(b[0:4], "XXXX")
	_, _, err := Decode(b)
	if err == nil {
		t.Fatal("err = nil, want bad magic")
	}
}

func TestDecode_TooShort(t *testing.T) {
	_, _, err := Decode(make([]byte, 10))
	if err == nil {
		t.Fatal("err = nil, want too-short")
	}
}

func TestDecode_Silence(t *testing.T) {
	// A block of all-zero nibbles with filter 0 should produce 28 zero
	// samples (since prev1 and prev2 are 0).
	nibbles := make([]byte, 14)
	block := makeBlock(0, 0, nibbles)
	vag := buildVAG([][]byte{block}, 44100)
	out, sr, err := Decode(vag)
	if err != nil {
		t.Fatal(err)
	}
	if sr != 44100 {
		t.Errorf("sampleRate = %d, want 44100", sr)
	}
	if len(out) != 28 {
		t.Fatalf("samples = %d, want 28", len(out))
	}
	for i, v := range out {
		if v != 0 {
			t.Errorf("sample %d = %v, want 0", i, v)
		}
	}
}

func TestDecode_PositiveShift(t *testing.T) {
	// All nibbles = 1, shift = 4, filter = 0. Expected sample = 1/16 = 0.0625.
	nibbles := make([]byte, 14)
	for i := range nibbles {
		nibbles[i] = 0x11 // two 1-nibbles per byte
	}
	block := makeBlock(4, 0, nibbles)
	vag := buildVAG([][]byte{block}, 44100)
	out, _, err := Decode(vag)
	if err != nil {
		t.Fatal(err)
	}
	want := float32(1.0 / 16.0)
	for i, v := range out {
		if math.Abs(float64(v-want)) > 1e-5 {
			t.Errorf("sample %d = %v, want %v", i, v, want)
			break
		}
	}
}

func TestDecode_SignExtends(t *testing.T) {
	// All nibbles = 8 (the most negative 4-bit value). With shift=0 and
	// filter=0, the decoded sample should be -8.
	nibbles := make([]byte, 14)
	for i := range nibbles {
		nibbles[i] = 0x88 // two 8-nibbles per byte
	}
	block := makeBlock(0, 0, nibbles)
	vag := buildVAG([][]byte{block}, 44100)
	out, _, err := Decode(vag)
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

func TestDecode_ChainsStateAcrossBlocks(t *testing.T) {
	// First block has all nibbles = 1, shift = 4, filter = 1 (predictor
	// coefficient 0.5). The first block's last sample influences the
	// second block's first sample.
	nibbles := make([]byte, 14)
	for i := range nibbles {
		nibbles[i] = 0x11
	}
	b1 := makeBlock(4, 1, nibbles)
	b2 := makeBlock(4, 1, nibbles)
	vag := buildVAG([][]byte{b1, b2}, 44100)
	out, _, err := Decode(vag)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 56 {
		t.Fatalf("samples = %d, want 56", len(out))
	}
	// The first sample of block 2 (index 28) should equal
	//   1/16 + 0.5 * (last sample of block 1)
	// because filter 1 = (0.5, 0). The last sample of block 1 is also
	// influenced by its own chain, but the formula is the same.
	lastOfB1 := out[27]
	expected := float32(1.0/16.0) + 0.5*lastOfB1
	if math.Abs(float64(out[28]-expected)) > 1e-5 {
		t.Errorf("sample 28 = %v, want %v (lastOfB1=%v)", out[28], expected, lastOfB1)
	}
}

func TestDecodeBlock_UnknownFilterFallsBackToZero(t *testing.T) {
	// filter = 0xF (15) is not a known Sony filter; the decoder should
	// fall back to filter 0 (no prediction) rather than panic.
	nibbles := make([]byte, 14)
	for i := range nibbles {
		nibbles[i] = 0x11
	}
	b := make([]byte, 16)
	b[0] = byte(0<<0) | byte(0xF<<4)
	b[1] = 0
	copy(b[2:], nibbles)
	out, p1, p2 := decodeBlock(b[2:16], 0, 0xF, 0, 0)
	// With filter falling back to 0, every sample should be the raw
	// nibble value (1).
	for i, v := range out {
		if v != 1 {
			t.Errorf("sample %d = %v, want 1", i, v)
			break
		}
	}
	_ = p1
	_ = p2
}
