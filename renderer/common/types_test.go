package common

import "testing"

func TestPixelFormat_BytesPerPixel(t *testing.T) {
	cases := []struct {
		fmt  PixelFormat
		want int
	}{
		{PixelFormatRGBA8, 4},
		{PixelFormatBGRA8, 4},
		{PixelFormatR8, 1},
		{PixelFormatIndexed8, 1},
		{PixelFormatDXT1, 1},
		{PixelFormatDXT3, 1},
		{PixelFormatDXT5, 1},
		{PixelFormatUnknown, 0},
	}
	for _, c := range cases {
		if got := c.fmt.BytesPerPixel(); got != c.want {
			t.Errorf("BytesPerPixel(%v) = %d, want %d", c.fmt, got, c.want)
		}
	}
}

func TestTextureDesc_Validate(t *testing.T) {
	cases := []struct {
		name string
		desc TextureDesc
		ok   bool
	}{
		{"ok_rgba8", TextureDesc{Width: 8, Height: 8, Format: PixelFormatRGBA8}, true},
		{"ok_indexed8_with_palette", TextureDesc{
			Width: 256, Height: 256, Format: PixelFormatIndexed8,
			Palette: make([]byte, 1024),
		}, true},
		{"zero_width", TextureDesc{Width: 0, Height: 8, Format: PixelFormatRGBA8}, false},
		{"zero_height", TextureDesc{Width: 8, Height: 0, Format: PixelFormatRGBA8}, false},
		{"unknown_format", TextureDesc{Width: 8, Height: 8, Format: PixelFormatUnknown}, false},
		{"indexed8_no_palette", TextureDesc{
			Width: 256, Height: 256, Format: PixelFormatIndexed8,
		}, false},
		{"indexed8_short_palette", TextureDesc{
			Width: 256, Height: 256, Format: PixelFormatIndexed8,
			Palette: make([]byte, 512),
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.desc.Validate()
			if (err == nil) != c.ok {
				t.Fatalf("Validate = %v, want ok=%v", err, c.ok)
			}
		})
	}
}
