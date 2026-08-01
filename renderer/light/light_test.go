package light

import (
	"math"
	"testing"
)

func approxEq(a, b [4]uint8, tol int) bool {
	for i := 0; i < 4; i++ {
		if int(a[i])-int(b[i]) > tol || int(b[i])-int(a[i]) > tol {
			return false
		}
	}
	return true
}

func TestBake_AmbientOnly(t *testing.T) {
	s := Scene{Ambient: AmbientLight{R: 0.5, G: 0.5, B: 0.5}}
	c := Bake(s, [3]float32{0, 1, 0})
	want := [4]uint8{128, 128, 128, 255}
	if !approxEq(c, want, 1) {
		t.Errorf("Bake = %v, want %v", c, want)
	}
}

func TestBake_DirectionalStraightOn(t *testing.T) {
	// Light pointing down (-Y), normal pointing up (+Y). diff = 1.
	s := Scene{
		Directionals: []DirectionalLight{
			{Dir: [3]float32{0, -1, 0}, R: 1, G: 1, B: 1},
		},
	}
	c := Bake(s, [3]float32{0, 1, 0})
	want := [4]uint8{255, 255, 255, 255}
	if !approxEq(c, want, 1) {
		t.Errorf("Bake = %v, want %v", c, want)
	}
}

func TestBake_DirectionalPerpendicular(t *testing.T) {
	// Light pointing down, normal pointing +X. diff = 0.
	s := Scene{
		Directionals: []DirectionalLight{
			{Dir: [3]float32{0, -1, 0}, R: 1, G: 1, B: 1},
		},
	}
	c := Bake(s, [3]float32{1, 0, 0})
	want := [4]uint8{0, 0, 0, 255}
	if !approxEq(c, want, 1) {
		t.Errorf("Bake = %v, want %v", c, want)
	}
}

func TestBake_AmbientPlusDirectional(t *testing.T) {
	s := Scene{
		Ambient: AmbientLight{R: 0.2, G: 0.2, B: 0.2},
		Directionals: []DirectionalLight{
			{Dir: [3]float32{0, -1, 0}, R: 0.5, G: 0.5, B: 0.5},
		},
	}
	c := Bake(s, [3]float32{0, 1, 0})
	// 0.2 + 1*0.5 = 0.7 -> 179
	want := [4]uint8{179, 179, 179, 255}
	if !approxEq(c, want, 2) {
		t.Errorf("Bake = %v, want %v", c, want)
	}
}

func TestBake_ClampsOverflow(t *testing.T) {
	// Two full-intensity lights + ambient = > 1.0; should clamp to 255.
	s := Scene{
		Ambient: AmbientLight{R: 1, G: 1, B: 1},
		Directionals: []DirectionalLight{
			{Dir: [3]float32{0, -1, 0}, R: 1, G: 1, B: 1},
			{Dir: [3]float32{0, -1, 0}, R: 1, G: 1, B: 1},
		},
	}
	c := Bake(s, [3]float32{0, 1, 0})
	if c[0] != 255 || c[1] != 255 || c[2] != 255 {
		t.Errorf("Bake = %v, want clamped to 255", c)
	}
}

func TestBake_NormalisesNormal(t *testing.T) {
	s := Scene{
		Directionals: []DirectionalLight{
			{Dir: [3]float32{0, -1, 0}, R: 1, G: 1, B: 1},
		},
	}
	// Un-normalised normal (0, 2, 0) should give the same result as
	// the unit normal (0, 1, 0).
	c1 := Bake(s, [3]float32{0, 1, 0})
	c2 := Bake(s, [3]float32{0, 2, 0})
	if !approxEq(c1, c2, 1) {
		t.Errorf("un-normalised normal gave different result: %v vs %v", c1, c2)
	}
}

func TestClamp01(t *testing.T) {
	cases := []struct{ in, want float32 }{
		{-1, 0},
		{0, 0},
		{0.5, 0.5},
		{1, 1},
		{2, 1},
	}
	for _, c := range cases {
		if got := clamp01(c.in); got != c.want {
			t.Errorf("clamp01(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestFloatToUint8(t *testing.T) {
	cases := []struct {
		in   float32
		want uint8
	}{
		{0, 0},
		{0.5, 128},
		{1, 255},
		{-1, 0},
		{2, 255},
	}
	for _, c := range cases {
		got := floatToUint8(c.in)
		if math.Abs(float64(got-c.want)) > 1 {
			t.Errorf("floatToUint8(%v) = %d, want ~%d", c.in, got, c.want)
		}
	}
}

func TestDefault_HasAmbientAndDirectional(t *testing.T) {
	s := Default()
	if s.Ambient.R <= 0 {
		t.Error("Default ambient is zero")
	}
	if len(s.Directionals) == 0 {
		t.Error("Default has no directional lights")
	}
}
