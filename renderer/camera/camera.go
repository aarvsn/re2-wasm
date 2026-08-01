// Package camera implements RE2's fixed-angle camera model. Each room in
// the original game has one or more pre-baked camera positions; when the
// player crosses a trigger, the active camera switches. This package owns
// the math (matrices) but not the trigger logic — that lives in the engine.
//
// All matrices are row-major, 4x4 float32, Go-native so they can be tested
// on the host without dragging in WebGL types.
package camera

import (
	"fmt"
	"math"
)

// Camera is a fixed-angle camera. It has a position, a look-at target, an
// up vector, and a vertical field-of-view in degrees.
type Camera struct {
	Pos    [3]float32
	Target [3]float32
	Up     [3]float32
	FovY   float32 // degrees
	Aspect float32 // width / height
	Near   float32
	Far    float32
}

// New returns a camera with sensible RE2 defaults: 45° FOV, 16:9 aspect,
// near=0.1, far=1000. The original game uses orthographic-ish perspective
// but a real perspective projection preserves the look well enough.
func New() *Camera {
	return &Camera{
		Up:     [3]float32{0, 1, 0},
		FovY:   45,
		Aspect: 16.0 / 9.0,
		Near:   0.1,
		Far:    1000,
	}
}

// View returns the camera's view matrix (row-major 4x4).
func (c *Camera) View() [16]float32 {
	return lookAt(c.Pos, c.Target, c.Up)
}

// Projection returns the camera's perspective projection matrix. Callers
// must set Aspect before calling; if it is zero, 16:9 is assumed.
func (c *Camera) Projection() [16]float32 {
	aspect := c.Aspect
	if aspect == 0 {
		aspect = 16.0 / 9.0
	}
	return perspective(c.FovY, aspect, c.Near, c.Far)
}

// ViewProjection multiplies View * Projection in the order most shaders
// expect (v_clip = Projection * View * v_model).
func (c *Camera) ViewProjection() [16]float32 {
	v := c.View()
	p := c.Projection()
	return matMul(p, v)
}

// lookAt computes a right-handed look-at matrix. The result transforms
// world space into view space.
func lookAt(eye, target, up [3]float32) [16]float32 {
	f := normalize(sub(target, eye))
	s := normalize(cross(f, up))
	u := cross(s, f)
	return [16]float32{
		s[0], u[0], -f[0], 0,
		s[1], u[1], -f[1], 0,
		s[2], u[2], -f[2], 0,
		-dot(s, eye), -dot(u, eye), dot(f, eye), 1,
	}
}

// perspective computes a right-handed perspective projection matrix.
// fovY is in degrees.
func perspective(fovY, aspect, near, far float32) [16]float32 {
	if near <= 0 || far <= 0 || near >= far {
		panic(fmt.Sprintf("camera: invalid near/far: %v/%v", near, far))
	}
	f := 1.0 / float32(math.Tan(float64(fovY)*math.Pi/180.0/2.0))
	return [16]float32{
		f / aspect, 0, 0, 0,
		0, f, 0, 0,
		0, 0, (far + near) / (near - far), -1,
		0, 0, (2 * far * near) / (near - far), 0,
	}
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

// vec3 helpers — kept here to avoid pulling in a math/vector package.
func sub(a, b [3]float32) [3]float32 { return [3]float32{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func add(a, b [3]float32) [3]float32 { return [3]float32{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func dot(a, b [3]float32) float32    { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func cross(a, b [3]float32) [3]float32 {
	return [3]float32{
		a[1]*b[2] - a[2]*b[1],
		a[2]*b[0] - a[0]*b[2],
		a[0]*b[1] - a[1]*b[0],
	}
}
func normalize(a [3]float32) [3]float32 {
	l := float32(math.Sqrt(float64(dot(a, a))))
	if l == 0 {
		return a
	}
	return [3]float32{a[0] / l, a[1] / l, a[2] / l}
}

// ensure add is used so the linter stays happy; it is part of the math API
// for future camera interpolation (Phase 5).
var _ = add
