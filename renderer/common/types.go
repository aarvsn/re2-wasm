// Package common defines renderer-agnostic interfaces and value types shared
// between the WebGL2 and (future) WebGPU backends. Keeping these in a separate
// package prevents import cycles: both renderer/webgl and renderer/webgpu will
// depend on common, and the engine depends on common for its public types.
package common

// PixelFormat enumerates the texture formats the renderer can upload. Only a
// subset of WebGL2's internal formats is exposed; the original RE2 assets use
// 8-bit RGBA, BGRA, indexed, and DXT-compressed data, and those map onto the
// values below.
type PixelFormat int

// Supported pixel formats.
const (
	PixelFormatUnknown PixelFormat = iota
	PixelFormatRGBA8
	PixelFormatBGRA8
	PixelFormatR8
	PixelFormatIndexed8 // paletteised; upload needs an external palette
	PixelFormatDXT1
	PixelFormatDXT3
	PixelFormatDXT5
)

// BytesPerPixel returns the unpacked byte size of one pixel in fmt. Indexed
// and compressed formats return 1 (their uncompressed equivalent is computed
// at upload time by the backend).
func (p PixelFormat) BytesPerPixel() int {
	switch p {
	case PixelFormatRGBA8, PixelFormatBGRA8:
		return 4
	case PixelFormatR8:
		return 1
	case PixelFormatIndexed8:
		return 1
	case PixelFormatDXT1, PixelFormatDXT3, PixelFormatDXT5:
		return 1 // compressed; size is block-based, not per-pixel
	default:
		return 0
	}
}

// TextureDesc is the host-side description of a texture. Backends translate it
// into their own internal format (WebGLTexture, GPUTexture, ...).
type TextureDesc struct {
	Width     int
	Height    int
	Format    PixelFormat
	Mipmapped bool
	// Palette is the 256-entry RGBA palette for PixelFormatIndexed8. nil for
	// non-indexed formats. Each entry is one byte; the slice length must be
	// 1024 (256 * 4) when set.
	Palette []byte
}

// Validate returns an error if the descriptor is malformed.
func (t TextureDesc) Validate() error {
	if t.Width <= 0 || t.Height <= 0 {
		return ErrInvalidTextureDesc{Field: "Width/Height", Reason: "must be > 0"}
	}
	if t.Format == PixelFormatUnknown {
		return ErrInvalidTextureDesc{Field: "Format", Reason: "must be set"}
	}
	if t.Format == PixelFormatIndexed8 && len(t.Palette) != 1024 {
		return ErrInvalidTextureDesc{Field: "Palette", Reason: "indexed8 requires 1024-byte palette"}
	}
	return nil
}

// ErrInvalidTextureDesc is returned by TextureDesc.Validate and by backends
// when they reject an upload.
type ErrInvalidTextureDesc struct {
	Field  string
	Reason string
}

// Error implements error.
func (e ErrInvalidTextureDesc) Error() string {
	return "common: invalid texture desc: " + e.Field + ": " + e.Reason
}

// Vertex is the engine's canonical vertex layout. Backends convert this into
// a vertex attribute setup; shaders consume it as `vec3 pos; vec2 uv; vec4 col`.
type Vertex struct {
	Pos  [3]float32
	UV   [2]float32
	Col  [4]float32 // RGBA, 0..1
	Bone [4]float32 // bone weights for skinned meshes (Phase 3+)
}

// Mesh is a renderable mesh. Phase 3 will populate this from ADT/TIM model
// data; Phase 1 leaves it unused.
type Mesh struct {
	Vertices []Vertex
	Indices  []uint32
	Texture  TextureID
}

// TextureID is a backend-allocated handle. The zero value means "no texture".
type TextureID uint32

// MeshID is a backend-allocated handle for uploaded geometry.
type MeshID uint32

// Stats exposes renderer-side counters so the UI / overlay can show them.
type Stats struct {
	FrameNumber uint64
	DrawCalls   uint32
	Textures    uint32
	Meshes      uint32
	FrameMS     float32
}
