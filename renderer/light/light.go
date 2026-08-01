// Package light implements RE2's per-vertex lighting model. The original
// engine bakes lighting into the vertex colours at room-load time using a
// small set of directional + ambient lights; the GPU then renders the
// mesh with the pre-baked colours and no runtime lighting cost.
//
// This package provides the math for that baking step. It is pure Go so
// the same code runs on host tests and in the WASM build.
package light

import (
	"math"

	"github.com/aarvsn/re2-wasm/renderer/tmd"
)

// AmbientLight is the constant ambient term added to every vertex.
type AmbientLight struct {
	R, G, B float32 // 0..1
}

// DirectionalLight is a parallel ray source (sun, moon, ceiling lamp).
type DirectionalLight struct {
	Dir     [3]float32 // must be normalised
	R, G, B float32    // 0..1
}

// Scene is the set of lights applied to a room.
type Scene struct {
	Ambient      AmbientLight
	Directionals []DirectionalLight
}

// Default returns the lighting scene RE2 uses for most indoor rooms: a
// dim ambient plus one overhead directional.
func Default() Scene {
	return Scene{
		Ambient: AmbientLight{R: 0.25, G: 0.25, B: 0.25},
		Directionals: []DirectionalLight{
			{Dir: [3]float32{0, -1, 0}, R: 0.7, G: 0.7, B: 0.7},
		},
	}
}

// Bake computes the RGBA colour for one vertex given its normal and the
// scene's lights. The returned colour is pre-multiplied alpha = 1.
func Bake(s Scene, normal [3]float32) [4]uint8 {
	n := normalize(normal)
	r := s.Ambient.R
	g := s.Ambient.G
	b := s.Ambient.B
	for _, d := range s.Directionals {
		// Lambert: intensity = max(0, -dot(N, L)) because the light
		// direction points from the source toward the surface.
		diff := clamp01(-dot(n, d.Dir))
		r += diff * d.R
		g += diff * d.G
		b += diff * d.B
	}
	return [4]uint8{
		floatToUint8(clamp01(r)),
		floatToUint8(clamp01(g)),
		floatToUint8(clamp01(b)),
		255,
	}
}

// BakeModel applies Bake to every vertex of a TMD model. The returned
// slice is flat RGBA, one entry per vertex (so len == len(verts)*4).
func BakeModel(s Scene, m *tmd.Model) []uint8 {
	if m == nil {
		return nil
	}
	var out []uint8
	for _, obj := range m.Objects {
		for _, n := range obj.Normals {
			normal := [3]float32{
				float32(n.X) / 4096.0,
				float32(n.Y) / 4096.0,
				float32(n.Z) / 4096.0,
			}
			c := Bake(s, normal)
			out = append(out, c[0], c[1], c[2], c[3])
		}
	}
	return out
}

// dot returns the dot product of two 3-vectors.
func dot(a, b [3]float32) float32 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

// normalize returns the unit vector along a, or a if a is zero.
func normalize(a [3]float32) [3]float32 {
	l := float32(math.Sqrt(float64(dot(a, a))))
	if l == 0 {
		return a
	}
	return [3]float32{a[0] / l, a[1] / l, a[2] / l}
}

// clamp01 restricts v to [0, 1].
func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// floatToUint8 maps a [0, 1] float to a 0..255 byte.
func floatToUint8(v float32) uint8 {
	return uint8(clamp01(v)*255 + 0.5)
}
