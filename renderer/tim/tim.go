// Package tim decodes PlayStation 1 TIM texture files, the format the
// original Resident Evil 2 uses for every sprite, UI element, and texture.
//
// The format is documented in the PS1 SDK and is structurally simple. We
// expose the decoded image as RGBA8 so the renderer backend can upload it
// without re-decoding.
package tim

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// PixelMode is the TIM's pixel depth.
type PixelMode int

// Supported pixel modes.
const (
	Mode4BPP  PixelMode = 0 // 16 colours, CLUT indexed
	Mode8BPP  PixelMode = 1 // 256 colours, CLUT indexed
	Mode16BPP PixelMode = 2 // 5551 RGBA, no CLUT
	Mode24BPP PixelMode = 3 // 888 RGB, no CLUT
)

// Image is a decoded TIM. Pixels is always RGBA8 (4 bytes per pixel).
type Image struct {
	Width   int
	Height  int
	Pixels  []byte // RGBA, row-major, top-down
	Mode    PixelMode
	HasClut bool
	Clut    []byte // RGBA, the palette (16 or 256 entries)
}

// Decode parses a TIM file from b and returns the decoded Image. The
// returned Pixels are always RGBA8 regardless of the source pixel mode.
func Decode(b []byte) (*Image, error) {
	if len(b) < 20 {
		return nil, errors.New("tim: header too short")
	}
	id := binary.LittleEndian.Uint32(b[0:4])
	if id != 0x10 {
		return nil, fmt.Errorf("tim: bad magic 0x%08x (want 0x10)", id)
	}
	flags := binary.LittleEndian.Uint32(b[4:8])
	mode := PixelMode(flags & 0xFF)
	if mode > Mode24BPP {
		return nil, fmt.Errorf("tim: unsupported pixel mode %d", mode)
	}

	off := 8
	// Skip the "unused" u32 at offset 8 (always 0 in practice).
	off += 4

	clutLen := binary.LittleEndian.Uint32(b[off : off+4])
	off += 4
	off += 4 // CLUT org DMA tag, ignored

	var clut []byte
	hasClut := clutLen > 0
	if hasClut {
		c, used, err := decodeClut(b[off:off+int(clutLen)], mode)
		if err != nil {
			return nil, fmt.Errorf("tim: decode clut: %w", err)
		}
		clut = c
		off += used
	}

	if off+8 > len(b) {
		return nil, errors.New("tim: pixel block header truncated")
	}
	pixLen := binary.LittleEndian.Uint32(b[off : off+4])
	off += 4
	off += 4 // orgX/orgY
	w := int(binary.LittleEndian.Uint16(b[off : off+2]))
	h := int(binary.LittleEndian.Uint16(b[off+2 : off+4]))
	off += 4

	pixW, pixH := realDims(mode, w, h)
	if pixW <= 0 || pixH <= 0 {
		return nil, fmt.Errorf("tim: bad pixel dims %dx%d (mode=%d, raw %dx%d)", pixW, pixH, mode, w, h)
	}

	// Pixel data is the rest of the block (pixLen includes the 12-byte
	// header we just consumed).
	pixAvail := len(b) - off
	pixWant := int(pixLen) - 12
	if pixWant < 0 {
		return nil, errors.New("tim: pixel block length underflow")
	}
	if pixAvail < pixWant {
		// Some TIMs round up; allow short reads.
		pixWant = pixAvail
	}
	pixData := b[off : off+pixWant]

	pixels, err := decodePixels(pixData, mode, pixW, pixH, clut)
	if err != nil {
		return nil, fmt.Errorf("tim: decode pixels: %w", err)
	}

	return &Image{
		Width:   pixW,
		Height:  pixH,
		Pixels:  pixels,
		Mode:    mode,
		HasClut: hasClut,
		Clut:    clut,
	}, nil
}

// realDims converts the TIM header's encoded width/height to actual pixel
// dimensions per the pixel mode.
func realDims(mode PixelMode, w, h int) (int, int) {
	switch mode {
	case Mode4BPP:
		return (w + 1) * 4, (h + 1)
	case Mode8BPP:
		return (w + 1) * 2, (h + 1)
	case Mode16BPP, Mode24BPP:
		return (w + 1), (h + 1)
	default:
		return 0, 0
	}
}

// decodeClut decodes a CLUT block into an RGBA palette. For 4bpp there are
// 16 colours; for 8bpp there are 256. Each colour is a 16-bit 5551 value.
func decodeClut(b []byte, mode PixelMode) ([]byte, int, error) {
	if len(b) < 12 {
		return nil, 0, errors.New("clut: header truncated")
	}
	length := int(binary.LittleEndian.Uint32(b[0:4]))
	if length > len(b)+12 {
		return nil, 0, fmt.Errorf("clut: length %d overflows buffer %d", length, len(b)+12)
	}
	w := int(binary.LittleEndian.Uint16(b[8:10]))
	h := int(binary.LittleEndian.Uint16(b[10:12]))
	pix := b[12:]
	wantColors := 16
	if mode == Mode8BPP {
		wantColors = 256
	}
	colors := w * h
	if colors < wantColors {
		colors = wantColors
	}
	if len(pix) < colors*2 {
		// Pad with zero entries; the lookup will fall through to index 0.
		colors = wantColors
	}
	out := make([]byte, wantColors*4)
	for i := 0; i < wantColors && i*2+1 < len(pix); i++ {
		rgb5551 := binary.LittleEndian.Uint16(pix[i*2 : i*2+2])
		out[i*4+0] = scale5to8(rgb5551 & 0x1F)
		out[i*4+1] = scale5to8((rgb5551 >> 5) & 0x1F)
		out[i*4+2] = scale5to8((rgb5551 >> 10) & 0x1F)
		out[i*4+3] = 0xFF
	}
	return out, length, nil
}

// decodePixels expands the TIM's packed pixel data to RGBA8.
func decodePixels(b []byte, mode PixelMode, w, h int, clut []byte) ([]byte, error) {
	out := make([]byte, w*h*4)
	switch mode {
	case Mode16BPP:
		if len(b) < w*h*2 {
			return nil, fmt.Errorf("16bpp: need %d bytes, have %d", w*h*2, len(b))
		}
		for i := 0; i < w*h; i++ {
			c := binary.LittleEndian.Uint16(b[i*2 : i*2+2])
			out[i*4+0] = scale5to8(c & 0x1F)
			out[i*4+1] = scale5to8((c >> 5) & 0x1F)
			out[i*4+2] = scale5to8((c >> 10) & 0x1F)
			out[i*4+3] = 0xFF
		}
	case Mode24BPP:
		if len(b) < w*h*3 {
			return nil, fmt.Errorf("24bpp: need %d bytes, have %d", w*h*3, len(b))
		}
		for i := 0; i < w*h; i++ {
			out[i*4+0] = b[i*3+0]
			out[i*4+1] = b[i*3+1]
			out[i*4+2] = b[i*3+2]
			out[i*4+3] = 0xFF
		}
	case Mode4BPP:
		if clut == nil {
			return nil, errors.New("4bpp: no CLUT")
		}
		// Each byte holds two 4-bit indices. PS1 packs pixels
		// little-nibble-first: pixel i's index is the low nibble of
		// byte i/2 when i is even, the high nibble when i is odd.
		for i := 0; i < w*h; i++ {
			byteIdx := i / 2
			if byteIdx >= len(b) {
				break
			}
			var idx byte
			if i%2 == 0 {
				idx = b[byteIdx] & 0x0F
			} else {
				idx = (b[byteIdx] >> 4) & 0x0F
			}
			copy(out[i*4:i*4+4], clut[int(idx)*4:int(idx)*4+4])
		}
	case Mode8BPP:
		if clut == nil {
			return nil, errors.New("8bpp: no CLUT")
		}
		for i := 0; i < w*h; i++ {
			if i >= len(b) {
				break
			}
			idx := b[i]
			copy(out[i*4:i*4+4], clut[int(idx)*4:int(idx)*4+4])
		}
	default:
		return nil, fmt.Errorf("unsupported mode %d", mode)
	}
	return out, nil
}

// scale5to8 maps a 5-bit channel value to 8-bit.
func scale5to8(v uint16) byte {
	return byte((v << 3) | (v >> 2))
}
