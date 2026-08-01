// Package skin implements RE2's skinned-mesh math. RE2 characters are
// rigged to a small skeleton (Leon has ~24 bones, zombies ~18); each
// vertex carries up to 4 bone weights and the renderer blends the
// per-bone matrices at draw time.
//
// This package provides the CPU-side blend math so tests can verify the
// pipeline. In production the same algorithm runs in a vertex shader;
// the math here is the reference implementation.
package skin

import (
	"math"
)

// Bone is one joint in the skeleton. Local is the bone's transform
// relative to its parent; World is the absolute transform (computed by
// UpdateWorld).
type Bone struct {
	Name   string
	Parent int // -1 for the root
	Local  [16]float32
	World  [16]float32
}

// Skeleton is a hierarchy of bones.
type Skeleton struct {
	Bones []Bone
}

// NewSkeleton returns an empty skeleton.
func NewSkeleton() *Skeleton { return &Skeleton{} }

// Add appends a bone and returns its index.
func (s *Skeleton) Add(b Bone) int {
	s.Bones = append(s.Bones, b)
	return len(s.Bones) - 1
}

// UpdateWorld recomputes every bone's World matrix. Bones must be added
// in dependency order (parents before children); the function panics if
// a child references a parent that does not yet have a World matrix.
func (s *Skeleton) UpdateWorld() {
	for i := range s.Bones {
		b := &s.Bones[i]
		if b.Parent < 0 {
			b.World = b.Local
			continue
		}
		if b.Parent >= i {
			panic("skin: bone parent must come before child")
		}
		b.World = matMul(s.Bones[b.Parent].World, b.Local)
	}
}

// VertexWeight pairs a vertex with up to 4 bones and their weights. Weights
// must sum to 1 (the engine normalises if they don't).
type VertexWeight struct {
	Bones   [4]int // -1 means "unused slot"
	Weights [4]float32
}

// BlendVertex computes the skinned world-space position of one vertex
// given its rest pose (rest) and its per-bone weights. The result is
// the weighted sum of (bone.World * rest).
func BlendVertex(s *Skeleton, w VertexWeight, rest [3]float32) [3]float32 {
	// Normalise weights so they sum to 1 (defensive).
	var sum float32
	for _, wv := range w.Weights {
		sum += wv
	}
	if sum == 0 {
		return rest
	}
	var out [3]float32
	for i := 0; i < 4; i++ {
		if w.Bones[i] < 0 || w.Weights[i] == 0 {
			continue
		}
		b := &s.Bones[w.Bones[i]]
		transformed := transformPoint(b.World, rest)
		weight := w.Weights[i] / sum
		out[0] += transformed[0] * weight
		out[1] += transformed[1] * weight
		out[2] += transformed[2] * weight
	}
	return out
}

// matMul multiplies two row-major 4x4 matrices.
func matMul(a, b [16]float32) [16]float32 {
	var out [16]float32
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			var sum float32
			for k := 0; k < 4; k++ {
				sum += a[i*4+k] * b[k*4+j]
			}
			out[i*4+j] = sum
		}
	}
	return out
}

// transformPoint applies a 4x4 matrix to a point (w=1).
func transformPoint(m [16]float32, p [3]float32) [3]float32 {
	return [3]float32{
		m[0]*p[0] + m[4]*p[1] + m[8]*p[2] + m[12],
		m[1]*p[0] + m[5]*p[1] + m[9]*p[2] + m[13],
		m[2]*p[0] + m[6]*p[1] + m[10]*p[2] + m[14],
	}
}

// IdentityMatrix returns a row-major 4x4 identity.
func IdentityMatrix() [16]float32 {
	return [16]float32{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
}

// TranslationMatrix returns a row-major 4x4 translation.
func TranslationMatrix(x, y, z float32) [16]float32 {
	m := IdentityMatrix()
	m[12] = x
	m[13] = y
	m[14] = z
	return m
}

// RotationYMatrix returns a row-major 4x4 rotation about Y.
func RotationYMatrix(radians float32) [16]float32 {
	c := float32(math.Cos(float64(radians)))
	s := float32(math.Sin(float64(radians)))
	return [16]float32{
		c, 0, -s, 0,
		0, 1, 0, 0,
		s, 0, c, 0,
		0, 0, 0, 1,
	}
}
