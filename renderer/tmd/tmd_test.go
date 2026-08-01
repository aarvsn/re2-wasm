package tmd

import (
	"encoding/binary"
	"testing"
)

// buildTMD constructs a synthetic TMD with one object, the given vertices
// (no normals, no prims). Used to test header/vertex decoding without
// pulling in real asset bytes.
func buildTMD(verts []Vertex) []byte {
	const magic = Magic
	const version = 0
	const objCount = 1

	// Layout: 12-byte header + 24-byte obj descriptor + 8-byte verts.
	total := 12 + 24 + len(verts)*8
	out := make([]byte, total)
	binary.LittleEndian.PutUint32(out[0:4], magic)
	binary.LittleEndian.PutUint32(out[4:8], version)
	binary.LittleEndian.PutUint32(out[8:12], objCount)

	// Object descriptor (24 bytes). We use a minimal layout: vertices
	// only, no normals or prims.
	off := 12
	binary.LittleEndian.PutUint32(out[off+0:off+4], 0) // vertTop
	binary.LittleEndian.PutUint32(out[off+4:off+8], 0) // normTop
	binary.LittleEndian.PutUint32(out[off+12:off+16], uint32(len(verts)))
	binary.LittleEndian.PutUint32(out[off+16:off+20], 0) // normCount
	binary.LittleEndian.PutUint32(out[off+20:off+24], 0) // primTop
	off += 24

	for i, v := range verts {
		vo := off + i*8
		binary.LittleEndian.PutUint16(out[vo+0:vo+2], uint16(v.X))
		binary.LittleEndian.PutUint16(out[vo+2:vo+4], uint16(v.Y))
		binary.LittleEndian.PutUint16(out[vo+4:vo+6], uint16(v.Z))
	}
	return out
}

func TestDecode_HeaderAndVerts(t *testing.T) {
	verts := []Vertex{{1, 2, 3}, {-1, -2, -3}, {100, 200, 300}}
	b := buildTMD(verts)
	m, warns, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) > 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if m.Header.Magic != Magic {
		t.Errorf("magic = 0x%x", m.Header.Magic)
	}
	if m.Header.ObjCount != 1 {
		t.Errorf("objCount = %d", m.Header.ObjCount)
	}
	if len(m.Objects) != 1 {
		t.Fatalf("objects = %d, want 1", len(m.Objects))
	}
	od := m.Objects[0]
	if len(od.Verts) != 3 {
		t.Fatalf("verts = %d, want 3", len(od.Verts))
	}
	for i, v := range verts {
		got := od.Verts[i]
		if got != v {
			t.Errorf("vert %d = %+v, want %+v", i, got, v)
		}
	}
}

func TestDecode_BadMagic(t *testing.T) {
	b := make([]byte, 12)
	binary.LittleEndian.PutUint32(b[0:4], 0xdeadbeef)
	_, _, err := Decode(b)
	if err == nil {
		t.Fatal("err = nil, want bad magic")
	}
}

func TestDecode_TruncatedHeader(t *testing.T) {
	_, _, err := Decode(make([]byte, 5))
	if err == nil {
		t.Fatal("err = nil, want truncated")
	}
}

func TestDecode_ObjTableOverflow(t *testing.T) {
	b := make([]byte, 12)
	binary.LittleEndian.PutUint32(b[0:4], Magic)
	binary.LittleEndian.PutUint32(b[8:12], 100) // claims 100 objects
	_, _, err := Decode(b)
	if err == nil {
		t.Fatal("err = nil, want overflow")
	}
}

func TestDecode_NoObjects(t *testing.T) {
	b := make([]byte, 12)
	binary.LittleEndian.PutUint32(b[0:4], Magic)
	binary.LittleEndian.PutUint32(b[8:12], 0)
	m, _, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Objects) != 0 {
		t.Errorf("objects = %d, want 0", len(m.Objects))
	}
}

func TestDecodeTri_Minimal(t *testing.T) {
	// Build a minimal FT3 (0x24) primitive body. Layout per decodeTri:
	//   u16 V0, u16 V1, u16 V2
	//   u16 U0, u16 V0_, u16 U1, u16 V1_, u16 U2, u16 V2_
	//   u32 tpage
	body := make([]byte, 24)
	binary.LittleEndian.PutUint16(body[0:2], 10)   // V0
	binary.LittleEndian.PutUint16(body[2:4], 20)   // V1
	binary.LittleEndian.PutUint16(body[4:6], 30)   // V2
	binary.LittleEndian.PutUint16(body[6:8], 0)    // U0
	binary.LittleEndian.PutUint16(body[8:10], 0)   // V0_
	binary.LittleEndian.PutUint16(body[10:12], 16) // U1
	binary.LittleEndian.PutUint16(body[12:14], 16) // V1_
	binary.LittleEndian.PutUint16(body[14:16], 32) // U2
	binary.LittleEndian.PutUint16(body[16:18], 32) // V2_
	binary.LittleEndian.PutUint32(body[18:22], 0xdead)
	f, err := decodeTri(body, 0x24)
	if err != nil {
		t.Fatal(err)
	}
	if f.V0 != 10 || f.V1 != 20 || f.V2 != 30 {
		t.Errorf("verts = %d %d %d, want 10 20 30", f.V0, f.V1, f.V2)
	}
	if f.UV[0][0] != 0 || f.UV[1][0] != 16 || f.UV[2][0] != 32 {
		t.Errorf("uvs = %v, want U=0,16,32", f.UV)
	}
	if f.TexPage != 0xdead {
		t.Errorf("tpage = 0x%x, want 0xdead", f.TexPage)
	}
}

func TestDecodeQuad_SplitsToTwoTris(t *testing.T) {
	// Build a minimal FT4 (0x28) body. Layout per decodeQuad:
	//   u16 V0 V1 V2 V3
	//   u16 U0 V0_ U1 V1_ U2 V2_ U3 V3_
	//   u32 tpage
	body := make([]byte, 32)
	binary.LittleEndian.PutUint16(body[0:2], 1)    // V0
	binary.LittleEndian.PutUint16(body[2:4], 2)    // V1
	binary.LittleEndian.PutUint16(body[4:6], 3)    // V2
	binary.LittleEndian.PutUint16(body[6:8], 4)    // V3
	binary.LittleEndian.PutUint16(body[8:10], 0)   // U0
	binary.LittleEndian.PutUint16(body[10:12], 0)  // V0_
	binary.LittleEndian.PutUint16(body[12:14], 16) // U1
	binary.LittleEndian.PutUint16(body[14:16], 0)  // V1_
	binary.LittleEndian.PutUint16(body[16:18], 16) // U2
	binary.LittleEndian.PutUint16(body[18:20], 16) // V2_
	binary.LittleEndian.PutUint16(body[20:22], 0)  // U3
	binary.LittleEndian.PutUint16(body[22:24], 16) // V3_
	binary.LittleEndian.PutUint32(body[24:28], 0xcafe)
	f0, f1, err := decodeQuad(body, 0x28)
	if err != nil {
		t.Fatal(err)
	}
	// First triangle: V0, V1, V2.
	if f0.V0 != 1 || f0.V1 != 2 || f0.V2 != 3 {
		t.Errorf("f0 verts = %d %d %d, want 1 2 3", f0.V0, f0.V1, f0.V2)
	}
	// Second triangle: V0, V2, V3.
	if f1.V0 != 1 || f1.V1 != 3 || f1.V2 != 4 {
		t.Errorf("f1 verts = %d %d %d, want 1 3 4", f1.V0, f1.V1, f1.V2)
	}
	if f0.TexPage != 0xcafe || f1.TexPage != 0xcafe {
		t.Errorf("tpage = f0=0x%x f1=0x%x, want 0xcafe both", f0.TexPage, f1.TexPage)
	}
}
