package adt

import (
	"encoding/binary"
	"testing"
)

func buildADT(verts []Vertex, faces []Face) []byte {
	total := 16 + len(verts)*8 + len(faces)*24
	out := make([]byte, total)
	binary.LittleEndian.PutUint32(out[0:4], Magic)
	binary.LittleEndian.PutUint16(out[4:6], 42) // roomID
	binary.LittleEndian.PutUint16(out[6:8], 0)  // flags
	binary.LittleEndian.PutUint32(out[8:12], uint32(len(verts)))
	binary.LittleEndian.PutUint32(out[12:16], uint32(len(faces)))
	off := 16
	for i, v := range verts {
		vo := off + i*8
		binary.LittleEndian.PutUint16(out[vo+0:vo+2], uint16(v.X))
		binary.LittleEndian.PutUint16(out[vo+2:vo+4], uint16(v.Y))
		binary.LittleEndian.PutUint16(out[vo+4:vo+6], uint16(v.Z))
		out[vo+6] = v.Flags
	}
	off += len(verts) * 8
	for i, f := range faces {
		fo := off + i*24
		binary.LittleEndian.PutUint16(out[fo+0:fo+2], f.V0)
		binary.LittleEndian.PutUint16(out[fo+2:fo+4], f.V1)
		binary.LittleEndian.PutUint16(out[fo+4:fo+6], f.V2)
		for j := 0; j < 3; j++ {
			uo := fo + 6 + j*4
			binary.LittleEndian.PutUint16(out[uo+0:uo+2], f.UV[j][0])
			binary.LittleEndian.PutUint16(out[uo+2:uo+4], f.UV[j][1])
		}
		binary.LittleEndian.PutUint16(out[fo+18:fo+20], f.TexPage)
		binary.LittleEndian.PutUint16(out[fo+20:fo+22], f.Clut)
	}
	return out
}

func TestDecode_Header(t *testing.T) {
	b := buildADT(nil, nil)
	r, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if r.Header.RoomID != 42 {
		t.Errorf("RoomID = %d, want 42", r.Header.RoomID)
	}
	if r.VertexCount() != 0 || r.FaceCount() != 0 {
		t.Errorf("counts = %d/%d, want 0/0", r.VertexCount(), r.FaceCount())
	}
}

func TestDecode_VertsAndFaces(t *testing.T) {
	verts := []Vertex{
		{X: 100, Y: 0, Z: 50, Flags: 1},
		{X: -50, Y: 25, Z: 0},
		{X: 0, Y: -25, Z: 100},
	}
	faces := []Face{
		{
			V0: 0, V1: 1, V2: 2,
			UV:      [3][2]uint16{{0, 0}, {16, 0}, {16, 16}},
			TexPage: 5,
			Clut:    10,
		},
	}
	b := buildADT(verts, faces)
	r, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Verts) != 3 {
		t.Fatalf("verts = %d, want 3", len(r.Verts))
	}
	for i, v := range verts {
		got := r.Verts[i]
		if got.X != v.X || got.Y != v.Y || got.Z != v.Z {
			t.Errorf("vert %d = %+v, want %+v", i, got, v)
		}
	}
	if len(r.Faces) != 1 {
		t.Fatalf("faces = %d, want 1", len(r.Faces))
	}
	f := r.Faces[0]
	if f.V0 != 0 || f.V1 != 1 || f.V2 != 2 {
		t.Errorf("face verts = %d %d %d, want 0 1 2", f.V0, f.V1, f.V2)
	}
	if f.TexPage != 5 || f.Clut != 10 {
		t.Errorf("tpage/clut = %d/%d, want 5/10", f.TexPage, f.Clut)
	}
	if f.UV[1][0] != 16 || f.UV[2][1] != 16 {
		t.Errorf("UVs wrong: %+v", f.UV)
	}
}

func TestDecode_PreservesExtra(t *testing.T) {
	b := buildADT(nil, nil)
	extra := []byte{0xde, 0xad, 0xbe, 0xef}
	b = append(b, extra...)
	r, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.RawExtra) != 4 {
		t.Fatalf("RawExtra len = %d, want 4", len(r.RawExtra))
	}
	if r.RawExtra[0] != 0xde || r.RawExtra[3] != 0xef {
		t.Errorf("RawExtra = %v, want deadbeef", r.RawExtra)
	}
}

func TestDecode_BadMagic(t *testing.T) {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint32(b[0:4], 0xdeadbeef)
	_, err := Decode(b)
	if err == nil {
		t.Fatal("err = nil, want bad magic")
	}
}

func TestDecode_Truncated(t *testing.T) {
	_, err := Decode(make([]byte, 5))
	if err == nil {
		t.Fatal("err = nil, want truncated")
	}
}

func TestDecode_VertsOverflow(t *testing.T) {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint32(b[0:4], Magic)
	binary.LittleEndian.PutUint32(b[8:12], 1000) // claims 1000 verts
	_, err := Decode(b)
	if err == nil {
		t.Fatal("err = nil, want overflow")
	}
}
