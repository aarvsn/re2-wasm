package tim

import (
	"encoding/binary"
	"testing"
)

// buildTIM constructs a synthetic TIM with the given pixel mode, dimensions,
// CLUT, and pixel data. Used by every test below so we don't ship real RE2
// asset bytes.
func buildTIM(mode PixelMode, w, h int, clut []byte, pixels []byte) []byte {
	const magic = 0x10
	hasClut := clut != nil
	flags := uint32(mode)

	var out []byte
	hdr := make([]byte, 20)
	binary.LittleEndian.PutUint32(hdr[0:4], magic)
	binary.LittleEndian.PutUint32(hdr[4:8], flags)
	binary.LittleEndian.PutUint32(hdr[8:12], 0)
	off := 12

	if hasClut {
		clutBlock := buildClutBlock(mode, clut)
		binary.LittleEndian.PutUint32(hdr[off:off+4], uint32(len(clutBlock)))
		off += 4
		binary.LittleEndian.PutUint32(hdr[off:off+4], 0) // org
		off += 4
		out = append(out, hdr...)
		out = append(out, clutBlock...)
	} else {
		binary.LittleEndian.PutUint32(hdr[off:off+4], 0)
		off += 4
		binary.LittleEndian.PutUint32(hdr[off:off+4], 0)
		off += 4
		out = append(out, hdr...)
	}

	pixBlock := buildPixelBlock(mode, w, h, pixels)
	out = append(out, pixBlock...)
	return out
}

func buildClutBlock(mode PixelMode, rgba []byte) []byte {
	wantColors := 16
	if mode == Mode8BPP {
		wantColors = 256
	}
	pix := make([]byte, wantColors*2)
	for i := 0; i < wantColors && i*4+3 < len(rgba); i++ {
		r := uint16(rgba[i*4+0] >> 3)
		g := uint16(rgba[i*4+1] >> 3)
		b := uint16(rgba[i*4+2] >> 3)
		v := r | (g << 5) | (b << 10)
		binary.LittleEndian.PutUint16(pix[i*2:i*2+2], v)
	}
	block := make([]byte, 12+len(pix))
	binary.LittleEndian.PutUint32(block[0:4], uint32(len(block)))
	binary.LittleEndian.PutUint16(block[4:6], 0)                     // orgX
	binary.LittleEndian.PutUint16(block[6:8], 0)                     // orgY
	binary.LittleEndian.PutUint16(block[8:10], uint16(wantColors-1)) // w
	binary.LittleEndian.PutUint16(block[10:12], 1)                   // h
	copy(block[12:], pix)
	return block
}

func buildPixelBlock(mode PixelMode, w, h int, pixels []byte) []byte {
	encW := w - 1
	switch mode {
	case Mode4BPP:
		encW = w/4 - 1
	case Mode8BPP:
		encW = w/2 - 1
	case Mode16BPP, Mode24BPP:
		encW = w - 1
	}
	block := make([]byte, 12+len(pixels))
	binary.LittleEndian.PutUint32(block[0:4], uint32(len(block)))
	binary.LittleEndian.PutUint16(block[4:6], 0)             // orgX
	binary.LittleEndian.PutUint16(block[6:8], 0)             // orgY
	binary.LittleEndian.PutUint16(block[8:10], uint16(encW)) // w
	binary.LittleEndian.PutUint16(block[10:12], uint16(h-1)) // h
	copy(block[12:], pixels)
	return block
}

func TestDecode_16BPP(t *testing.T) {
	// 2x1 image, two red pixels (5551 red = 0x001F).
	pix := []byte{0x1F, 0x00, 0x1F, 0x00}
	timBytes := buildTIM(Mode16BPP, 2, 1, nil, pix)
	img, err := Decode(timBytes)
	if err != nil {
		t.Fatal(err)
	}
	if img.Width != 2 || img.Height != 1 {
		t.Fatalf("dims = %dx%d, want 2x1", img.Width, img.Height)
	}
	if len(img.Pixels) != 8 {
		t.Fatalf("pixels len = %d, want 8", len(img.Pixels))
	}
	// Red 5551 0x1F -> scale5to8(0x1F) = 0xFF
	if img.Pixels[0] != 0xFF || img.Pixels[1] != 0 || img.Pixels[2] != 0 {
		t.Errorf("pixel 0 = rgba(%d,%d,%d,%d), want red",
			img.Pixels[0], img.Pixels[1], img.Pixels[2], img.Pixels[3])
	}
}

func TestDecode_8BPP_UsesClut(t *testing.T) {
	// 2x1 image, indices [1, 0]. CLUT[1] = green, CLUT[0] = black.
	clut := make([]byte, 256*4)
	clut[4] = 0
	clut[5] = 0xFF
	clut[6] = 0
	clut[7] = 0xFF
	pix := []byte{1, 0}
	timBytes := buildTIM(Mode8BPP, 2, 1, clut, pix)
	img, err := Decode(timBytes)
	if err != nil {
		t.Fatal(err)
	}
	if img.Width != 2 || img.Height != 1 {
		t.Fatalf("dims = %dx%d, want 2x1", img.Width, img.Height)
	}
	// Pixel 0 should be green (from CLUT[1]).
	if img.Pixels[1] != 0xFF {
		t.Errorf("pixel 0 green = %d, want 0xFF", img.Pixels[1])
	}
	// Pixel 1 should be black (from CLUT[0]).
	if img.Pixels[4] != 0 || img.Pixels[5] != 0 {
		t.Errorf("pixel 1 = rgba(%d,%d,%d,%d), want black",
			img.Pixels[4], img.Pixels[5], img.Pixels[6], img.Pixels[7])
	}
}

func TestDecode_4BPP_UsesClut(t *testing.T) {
	// 2x1 image, indices [3, 7] packed as one byte 0x73.
	clut := make([]byte, 16*4)
	clut[3*4+0] = 0xFF // index 3 = red
	clut[3*4+1] = 0
	clut[3*4+2] = 0
	clut[3*4+3] = 0xFF
	clut[7*4+0] = 0 // index 7 = blue
	clut[7*4+1] = 0
	clut[7*4+2] = 0xFF
	clut[7*4+3] = 0xFF
	pix := []byte{0x73}
	timBytes := buildTIM(Mode4BPP, 4, 1, clut, pix) // 4bpp width must be /4
	// Actually 4bpp width = (w+1)*4; for 4 pixels we want header w=0.
	// buildPixelBlock divides w by 4, so we pass w=4 and it writes encW=0.
	img, err := Decode(timBytes)
	if err != nil {
		t.Fatal(err)
	}
	if img.Width != 4 || img.Height != 1 {
		t.Fatalf("dims = %dx%d, want 4x1", img.Width, img.Height)
	}
	// First nibble (lo of byte 0x73 = 3) -> red.
	if img.Pixels[0] != 0xFF {
		t.Errorf("pixel 0 R = %d, want 0xFF (red)", img.Pixels[0])
	}
	// Second nibble (hi of byte 0x73 = 7) -> blue.
	if img.Pixels[4*1+2] != 0xFF {
		t.Errorf("pixel 1 B = %d, want 0xFF (blue)", img.Pixels[4*1+2])
	}
}

func TestDecode_BadMagic(t *testing.T) {
	b := make([]byte, 20)
	binary.LittleEndian.PutUint32(b[0:4], 0xdeadbeef)
	_, err := Decode(b)
	if err == nil {
		t.Fatal("err = nil, want bad-magic")
	}
}

func TestDecode_TruncatedHeader(t *testing.T) {
	_, err := Decode(make([]byte, 5))
	if err == nil {
		t.Fatal("err = nil, want truncated")
	}
}

func TestScale5to8_Table(t *testing.T) {
	cases := []struct{ in, want byte }{
		{0, 0},
		{1, 8},    // (1<<3) | (1>>2) = 8 | 0 = 8
		{16, 132}, // (16<<3) | (16>>2) = 128 | 4 = 132
		{31, 255}, // (31<<3) | (31>>2) = 248 | 7 = 255
	}
	for _, c := range cases {
		if got := scale5to8(uint16(c.in)); got != c.want {
			t.Errorf("scale5to8(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestRealDims_Table(t *testing.T) {
	cases := []struct {
		mode         PixelMode
		w, h         int
		wantW, wantH int
	}{
		{Mode4BPP, 0, 0, 4, 1}, // (0+1)*4, (0+1)
		{Mode4BPP, 1, 1, 8, 2},
		{Mode8BPP, 0, 0, 2, 1},
		{Mode8BPP, 1, 1, 4, 2},
		{Mode16BPP, 0, 0, 1, 1},
		{Mode16BPP, 15, 7, 16, 8},
		{Mode24BPP, 0, 0, 1, 1},
	}
	for _, c := range cases {
		gotW, gotH := realDims(c.mode, c.w, c.h)
		if gotW != c.wantW || gotH != c.wantH {
			t.Errorf("realDims(%v,%d,%d) = %dx%d, want %dx%d",
				c.mode, c.w, c.h, gotW, gotH, c.wantW, c.wantH)
		}
	}
}
