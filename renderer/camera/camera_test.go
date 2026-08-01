package camera

import (
	"math"
	"testing"
)

func approxEqual(a, b [16]float32, eps float64) bool {
	for i := 0; i < 16; i++ {
		if math.Abs(float64(a[i]-b[i])) > eps {
			return false
		}
	}
	return true
}

func TestLookAt_IdentityWhenEyeEqualsTarget(t *testing.T) {
	// Degenerate: eye == target. We can't compute forward, so the result
	// is implementation-defined. We just assert no panic and non-NaN.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	m := lookAt([3]float32{0, 0, 0}, [3]float32{0, 0, 0}, [3]float32{0, 1, 0})
	for _, v := range m {
		if math.IsNaN(float64(v)) {
			t.Errorf("NaN in lookAt result: %v", m)
			return
		}
	}
}

func TestLookAt_TranslatesWorldToView(t *testing.T) {
	// Eye at (0,0,5), target at origin, up = +Y. The view matrix's
	// translation column should be (0, 0, -5) — i.e. the eye in view space
	// is at (0,0,-5) because we negate forward.
	m := lookAt([3]float32{0, 0, 5}, [3]float32{0, 0, 0}, [3]float32{0, 1, 0})
	want := [16]float32{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, -5, 1,
	}
	if !approxEqual(m, want, 1e-5) {
		t.Errorf("m = %v, want %v", m, want)
	}
}

func TestPerspective_BasicShape(t *testing.T) {
	m := perspective(90, 1.0, 1, 100)
	// At 90° FOV and aspect=1, the (0,0) entry should be 1 (since
	// f = 1/tan(45°) = 1).
	if math.Abs(float64(m[0])-1.0) > 1e-5 {
		t.Errorf("m[0] = %v, want 1", m[0])
	}
	// (1,1) should also be 1 (aspect=1).
	if math.Abs(float64(m[5])-1.0) > 1e-5 {
		t.Errorf("m[5] = %v, want 1", m[5])
	}
	// (2,2) = (far+near)/(near-far) = 101/(-99) ≈ -1.0202
	if math.Abs(float64(m[10])-(-101.0/99.0)) > 1e-5 {
		t.Errorf("m[10] = %v, want %v", m[10], -101.0/99.0)
	}
	// (3,2) = 2*far*near/(near-far) = 200/(-99) ≈ -2.0202
	if math.Abs(float64(m[14])-(-200.0/99.0)) > 1e-5 {
		t.Errorf("m[14] = %v, want %v", m[14], -200.0/99.0)
	}
}

func TestPerspective_PanicsOnBadNearFar(t *testing.T) {
	cases := []struct{ near, far float32 }{
		{0, 100},
		{-1, 100},
		{100, 100},
		{100, 1},
	}
	for _, c := range cases {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("perspective(near=%v far=%v) did not panic", c.near, c.far)
				}
			}()
			_ = perspective(45, 1, c.near, c.far)
		}()
	}
}

func TestMatMul_Identity(t *testing.T) {
	id := [16]float32{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
	other := [16]float32{
		1, 2, 3, 4,
		5, 6, 7, 8,
		9, 10, 11, 12,
		13, 14, 15, 16,
	}
	got := matMul(id, other)
	if got != other {
		t.Errorf("I*M = %v, want %v", got, other)
	}
	got = matMul(other, id)
	if got != other {
		t.Errorf("M*I = %v, want %v", got, other)
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		in   [3]float32
		want [3]float32
	}{
		{[3]float32{0, 0, 0}, [3]float32{0, 0, 0}}, // zero -> zero (no panic)
		{[3]float32{1, 0, 0}, [3]float32{1, 0, 0}},
		{[3]float32{2, 0, 0}, [3]float32{1, 0, 0}},
		{[3]float32{0, 3, 4}, [3]float32{0, 0.6, 0.8}},
	}
	for _, c := range cases {
		got := normalize(c.in)
		if math.Abs(float64(got[0]-c.want[0])) > 1e-5 ||
			math.Abs(float64(got[1]-c.want[1])) > 1e-5 ||
			math.Abs(float64(got[2]-c.want[2])) > 1e-5 {
			t.Errorf("normalize(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestCamera_ViewProjection_Consistent(t *testing.T) {
	cam := New()
	cam.Pos = [3]float32{0, 5, 10}
	cam.Target = [3]float32{0, 0, 0}
	vp := cam.ViewProjection()
	v := cam.View()
	p := cam.Projection()
	manual := matMul(p, v)
	if !approxEqual(vp, manual, 1e-5) {
		t.Errorf("ViewProjection != Projection*View")
	}
}
