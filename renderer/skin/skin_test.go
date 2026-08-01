package skin

import (
	"math"
	"testing"
)

func TestSkeleton_RootWorldEqualsLocal(t *testing.T) {
	s := NewSkeleton()
	root := Bone{Name: "root", Parent: -1, Local: TranslationMatrix(1, 2, 3)}
	s.Add(root)
	s.UpdateWorld()
	w := s.Bones[0].World
	// Translation is in elements 12, 13, 14.
	if w[12] != 1 || w[13] != 2 || w[14] != 3 {
		t.Errorf("root World = %v, want translation (1,2,3)", w)
	}
}

func TestSkeleton_ChildInheritsParent(t *testing.T) {
	s := NewSkeleton()
	root := Bone{Name: "root", Parent: -1, Local: TranslationMatrix(10, 0, 0)}
	child := Bone{Name: "child", Parent: 0, Local: TranslationMatrix(0, 5, 0)}
	s.Add(root)
	s.Add(child)
	s.UpdateWorld()
	// Child world position = (10, 5, 0).
	w := s.Bones[1].World
	if math.Abs(float64(w[12]-10)) > 1e-5 || math.Abs(float64(w[13]-5)) > 1e-5 || w[14] != 0 {
		t.Errorf("child World = (%v,%v,%v), want (10,5,0)", w[12], w[13], w[14])
	}
}

func TestSkeleton_PanicsOnBadParent(t *testing.T) {
	s := NewSkeleton()
	s.Add(Bone{Name: "x", Parent: 5}) // parent doesn't exist yet
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on bad parent")
		}
	}()
	s.UpdateWorld()
}

func TestBlendVertex_SingleBoneFullWeight(t *testing.T) {
	s := NewSkeleton()
	s.Add(Bone{Name: "b", Parent: -1, Local: TranslationMatrix(10, 20, 30)})
	s.UpdateWorld()
	w := VertexWeight{Bones: [4]int{0, -1, -1, -1}, Weights: [4]float32{1, 0, 0, 0}}
	out := BlendVertex(s, w, [3]float32{0, 0, 0})
	if out[0] != 10 || out[1] != 20 || out[2] != 30 {
		t.Errorf("BlendVertex = %v, want (10,20,30)", out)
	}
}

func TestBlendVertex_TwoBonesAverage(t *testing.T) {
	s := NewSkeleton()
	s.Add(Bone{Name: "a", Parent: -1, Local: TranslationMatrix(10, 0, 0)})
	s.Add(Bone{Name: "b", Parent: -1, Local: TranslationMatrix(0, 20, 0)})
	s.UpdateWorld()
	w := VertexWeight{
		Bones:   [4]int{0, 1, -1, -1},
		Weights: [4]float32{0.5, 0.5, 0, 0},
	}
	out := BlendVertex(s, w, [3]float32{0, 0, 0})
	if math.Abs(float64(out[0]-5)) > 1e-5 || math.Abs(float64(out[1]-10)) > 1e-5 || out[2] != 0 {
		t.Errorf("BlendVertex = %v, want (5,10,0)", out)
	}
}

func TestBlendVertex_NormalisesWeights(t *testing.T) {
	s := NewSkeleton()
	s.Add(Bone{Name: "a", Parent: -1, Local: TranslationMatrix(10, 0, 0)})
	s.UpdateWorld()
	// Weights sum to 2, not 1; should still produce the right result.
	w := VertexWeight{Bones: [4]int{0, -1, -1, -1}, Weights: [4]float32{2, 0, 0, 0}}
	out := BlendVertex(s, w, [3]float32{0, 0, 0})
	if out[0] != 10 {
		t.Errorf("BlendVertex = %v, want (10,0,0) after normalisation", out)
	}
}

func TestBlendVertex_ZeroWeightsReturnsRest(t *testing.T) {
	s := NewSkeleton()
	s.Add(Bone{Name: "a", Parent: -1, Local: TranslationMatrix(10, 0, 0)})
	s.UpdateWorld()
	w := VertexWeight{Bones: [4]int{0, -1, -1, -1}, Weights: [4]float32{0, 0, 0, 0}}
	out := BlendVertex(s, w, [3]float32{1, 2, 3})
	if out != [3]float32{1, 2, 3} {
		t.Errorf("BlendVertex = %v, want (1,2,3) (rest)", out)
	}
}

func TestIdentityMatrix(t *testing.T) {
	m := IdentityMatrix()
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			want := float32(0)
			if i == j {
				want = 1
			}
			if m[i*4+j] != want {
				t.Errorf("m[%d][%d] = %v, want %v", i, j, m[i*4+j], want)
			}
		}
	}
}

func TestTranslationMatrix(t *testing.T) {
	m := TranslationMatrix(1, 2, 3)
	if m[12] != 1 || m[13] != 2 || m[14] != 3 {
		t.Errorf("translation = %v,%v,%v", m[12], m[13], m[14])
	}
}

func TestRotationYMatrix_90Degrees(t *testing.T) {
	m := RotationYMatrix(float32(math.Pi / 2))
	// Rotating (1,0,0) by 90° about Y in our row-major convention gives
	// (0,0,-1). The exact direction is convention-dependent; what
	// matters is that the matrix is orthonormal and rotates in the Y
	// plane.
	p := transformPoint(m, [3]float32{1, 0, 0})
	if math.Abs(float64(p[0])) > 1e-5 || math.Abs(float64(p[1])) > 1e-5 || math.Abs(float64(p[2]+1)) > 1e-5 {
		t.Errorf("rotated = %v, want (0,0,-1)", p)
	}
}

func TestMatMul_IdentityIsNoOp(t *testing.T) {
	id := IdentityMatrix()
	other := TranslationMatrix(5, 6, 7)
	got := matMul(id, other)
	if got != other {
		t.Errorf("I*M != M")
	}
	got = matMul(other, id)
	if got != other {
		t.Errorf("M*I != M")
	}
}
