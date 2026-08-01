// Package tmd decodes PlayStation 1 TMD (3D model data) files. TMD is the
// native model format used by Resident Evil 2 for characters, enemies,
// items, and some room geometry.
//
// A TMD file contains:
//   - 8-byte header: magic (0x41), version, and counts
//   - An array of object descriptors (8 bytes each)
//   - For each object, a primitive block (vertices, normals, faces)
//
// This Phase 3 implementation decodes the header and the vertex/normal
// arrays. Primitive (face) decoding is intentionally limited to the tri/quad
// types RE2 actually uses; exotic primitives (lit/unlit, with/without
// texture) are decoded as plain triangles with their attributes tagged.
package tmd

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Magic is the TMD file magic number.
const Magic = 0x41

// Header is the 8-byte TMD file header.
type Header struct {
	Magic    uint32
	Version  uint32
	ObjCount uint32
}

// Object describes one TMD object's header and points to its primitive data.
type Object struct {
	VertTop   uint32 // first vertex index in the file's vertex pool
	VertCount uint32 // number of vertices in this object
	NormTop   uint32
	NormCount uint32
	PrimTop   uint32
	PrimSize  uint32
	Scale     uint32
}

// Vertex is a 4-byte fixed-point XYZ position (s.3.12).
type Vertex struct {
	X, Y, Z int16
}

// Normal is a 4-byte fixed-point XYZ direction (s.0.15).
type Normal struct {
	X, Y, Z int16
}

// Face is a triangle. RE2 models are triangulated; quads are split by the
// decoder. UV coordinates are in PS1 pixel space (0..255 for a 256-wide
// texture page).
type Face struct {
	V0, V1, V2 uint16       // indices into the object's vertex list
	UV         [3][2]uint16 // UV per vertex, in PS1 texel space
	TexPage    uint32       // texture page this face draws from
	Clut       uint32       // CLUT page this face uses
	Color      [3][3]uint8  // RGB per vertex (lit primitives)
}

// Model is a decoded TMD file: one or more objects, each with its own
// vertex / normal / face arrays.
type Model struct {
	Header  Header
	Objects []ObjectData
}

// ObjectData holds the decoded vertices, normals, and faces for one Object.
type ObjectData struct {
	Header  Object
	Verts   []Vertex
	Normals []Normal
	Faces   []Face
}

// Decode parses a TMD file from b and returns a Model with every object's
// vertices and normals decoded. Faces are decoded for the primitive types
// RE2 uses most (FT3, FT4, GT3, GT4 in TMD terminology); other primitive
// types are skipped with a warning returned via the returned []error slice.
func Decode(b []byte) (*Model, []error, error) {
	if len(b) < 12 {
		return nil, nil, errors.New("tmd: header too short")
	}
	hdr := Header{
		Magic:    binary.LittleEndian.Uint32(b[0:4]),
		Version:  binary.LittleEndian.Uint32(b[4:8]),
		ObjCount: binary.LittleEndian.Uint32(b[8:12]),
	}
	if hdr.Magic != Magic {
		return nil, nil, fmt.Errorf("tmd: bad magic 0x%08x (want 0x%x)", hdr.Magic, Magic)
	}
	if hdr.ObjCount == 0 {
		return &Model{Header: hdr}, nil, nil
	}

	objTableOff := 12
	objTableEnd := objTableOff + int(hdr.ObjCount)*24
	if objTableEnd > len(b) {
		return nil, nil, fmt.Errorf("tmd: object table overflows buffer (%d > %d)", objTableEnd, len(b))
	}

	m := &Model{Header: hdr, Objects: make([]ObjectData, hdr.ObjCount)}
	var warnings []error

	// Vertex / normal pools start immediately after the object table.
	poolBase := objTableEnd

	for i := uint32(0); i < hdr.ObjCount; i++ {
		off := objTableOff + int(i)*24
		oh := Object{
			VertTop:   binary.LittleEndian.Uint32(b[off+0 : off+4]),
			NormTop:   binary.LittleEndian.Uint32(b[off+4 : off+8]),
			VertCount: binary.LittleEndian.Uint32(b[off+12 : off+16]),
			NormCount: binary.LittleEndian.Uint32(b[off+16 : off+20]),
			PrimTop:   binary.LittleEndian.Uint32(b[off+20 : off+24]),
		}
		od := ObjectData{Header: oh}

		// Decode vertices. VertTop is an index into the file's vertex pool;
		// the pool starts at poolBase.
		vStart := poolBase + int(oh.VertTop)*8
		vEnd := vStart + int(oh.VertCount)*8
		if vEnd > len(b) {
			return nil, nil, fmt.Errorf("tmd: obj %d verts overflow", i)
		}
		od.Verts = make([]Vertex, oh.VertCount)
		for j := uint32(0); j < oh.VertCount; j++ {
			vo := vStart + int(j)*8
			od.Verts[j] = Vertex{
				X: int16(binary.LittleEndian.Uint16(b[vo+0 : vo+2])),
				Y: int16(binary.LittleEndian.Uint16(b[vo+2 : vo+4])),
				Z: int16(binary.LittleEndian.Uint16(b[vo+4 : vo+6])),
			}
		}

		// Decode normals. NormTop is an index into the file's normal pool.
		nStart := poolBase + int(oh.NormTop)*8
		nEnd := nStart + int(oh.NormCount)*8
		if nEnd > len(b) {
			return nil, nil, fmt.Errorf("tmd: obj %d normals overflow", i)
		}
		od.Normals = make([]Normal, oh.NormCount)
		for j := uint32(0); j < oh.NormCount; j++ {
			no := nStart + int(j)*8
			od.Normals[j] = Normal{
				X: int16(binary.LittleEndian.Uint16(b[no+0 : no+2])),
				Y: int16(binary.LittleEndian.Uint16(b[no+2 : no+4])),
				Z: int16(binary.LittleEndian.Uint16(b[no+4 : no+6])),
			}
		}

		// Decode primitives. We only handle the common types; everything
		// else is skipped with a warning.
		primOff := int(oh.PrimTop)
		if primOff >= len(b) {
			warnings = append(warnings, fmt.Errorf("tmd: obj %d prim top out of range", i))
			m.Objects[i] = od
			continue
		}
		faces, warns, err := decodePrimitives(b[primOff:], oh.PrimSize)
		if err != nil {
			return nil, nil, fmt.Errorf("tmd: obj %d prims: %w", i, err)
		}
		od.Faces = faces
		warnings = append(warnings, warns...)
		m.Objects[i] = od
	}

	return m, warnings, nil
}

// decodePrimitives walks the primitive block of one object. We support the
// four TMD primitive types RE2 uses most: 0x24 (FT3), 0x28 (FT4), 0x34
// (GT3), 0x38 (GT4). Other types are skipped (their lengths are looked up
// in a small table) and reported via the returned warnings slice.
func decodePrimitives(b []byte, totalSize uint32) ([]Face, []error, error) {
	var faces []Face
	var warnings []error
	off := 0
	end := int(totalSize)
	if end > len(b) {
		end = len(b)
	}
	for off+4 <= end {
		primHeader := binary.LittleEndian.Uint32(b[off : off+4])
		primLen := primHeader >> 24
		primType := primHeader & 0xFF
		if primLen == 0 {
			// Some TMDs pad the end with zero-length prims; stop.
			break
		}
		if off+int(primLen) > end {
			warnings = append(warnings, fmt.Errorf("tmd: prim at +%d length %d overflows", off, primLen))
			break
		}
		body := b[off+4 : off+int(primLen)]
		switch primType {
		case 0x24, 0x34:
			// FT3 / GT3: 3 vertices, 3 UVs, optional RGBs.
			f, err := decodeTri(body, primType)
			if err != nil {
				warnings = append(warnings, fmt.Errorf("tmd: tri at +%d: %w", off, err))
			} else {
				faces = append(faces, f)
			}
		case 0x28, 0x38:
			// FT4 / GT4: 4 vertices, 4 UVs; split into two triangles.
			f1, f2, err := decodeQuad(body, primType)
			if err != nil {
				warnings = append(warnings, fmt.Errorf("tmd: quad at +%d: %w", off, err))
			} else {
				faces = append(faces, f1, f2)
			}
		default:
			warnings = append(warnings, fmt.Errorf("tmd: skipping primitive type 0x%02x at +%d", primType, off))
		}
		off += int(primLen)
	}
	return faces, warnings, nil
}

// decodeTri decodes an FT3 (0x24) or GT3 (0x34) primitive.
// Layout:
//
//	u32  prim header
//	u8[3] RGB0 (GT3 only) + pad
//	u16[3] V0, V1, V2
//	u16[2] UV0 + pad, UV1 + pad, UV2 + pad
//	u8[3] RGB1, RGB2 (GT3 only)
//	u32   tpage / clut
func decodeTri(b []byte, ptype uint32) (Face, error) {
	if len(b) < 16 {
		return Face{}, fmt.Errorf("tri body %d bytes, want >=16", len(b))
	}
	var f Face
	off := 0
	if ptype == 0x34 {
		// GT3 has per-vertex colours; we read only RGB0 here and skip
		// RGB1/RGB2 from later in the body.
		f.Color[0][0] = b[off]
		f.Color[0][1] = b[off+1]
		f.Color[0][2] = b[off+2]
		off += 4
	}
	if off+6 > len(b) {
		return f, fmt.Errorf("tri verts truncated")
	}
	f.V0 = binary.LittleEndian.Uint16(b[off : off+2])
	f.V1 = binary.LittleEndian.Uint16(b[off+2 : off+4])
	f.V2 = binary.LittleEndian.Uint16(b[off+4 : off+6])
	off += 6
	for i := 0; i < 3; i++ {
		if off+4 > len(b) {
			return f, fmt.Errorf("tri uv %d truncated", i)
		}
		f.UV[i][0] = binary.LittleEndian.Uint16(b[off : off+2])
		f.UV[i][1] = binary.LittleEndian.Uint16(b[off+2 : off+4])
		off += 4
	}
	if ptype == 0x34 {
		// Skip RGB1 (3 bytes + 1 pad) and RGB2 (3 bytes + 1 pad).
		off += 8
	}
	if off+4 <= len(b) {
		f.TexPage = binary.LittleEndian.Uint32(b[off : off+4])
		f.Clut = f.TexPage // encoded in the same word for our purposes
	}
	return f, nil
}

// decodeQuad decodes an FT4 (0x28) or GT4 (0x38) primitive and splits it
// into two triangles (V0-V1-V2 and V0-V2-V3).
func decodeQuad(b []byte, ptype uint32) (Face, Face, error) {
	if len(b) < 20 {
		return Face{}, Face{}, fmt.Errorf("quad body %d bytes, want >=20", len(b))
	}
	var f0, f1 Face
	off := 0
	if ptype == 0x38 {
		f0.Color[0][0] = b[off]
		f0.Color[0][1] = b[off+1]
		f0.Color[0][2] = b[off+2]
		off += 4
	}
	if off+8 > len(b) {
		return f0, f1, fmt.Errorf("quad verts truncated")
	}
	v := [4]uint16{
		binary.LittleEndian.Uint16(b[off : off+2]),
		binary.LittleEndian.Uint16(b[off+2 : off+4]),
		binary.LittleEndian.Uint16(b[off+4 : off+6]),
		binary.LittleEndian.Uint16(b[off+6 : off+8]),
	}
	off += 8
	var uv [4][2]uint16
	for i := 0; i < 4; i++ {
		if off+4 > len(b) {
			return f0, f1, fmt.Errorf("quad uv %d truncated", i)
		}
		uv[i][0] = binary.LittleEndian.Uint16(b[off : off+2])
		uv[i][1] = binary.LittleEndian.Uint16(b[off+2 : off+4])
		off += 4
	}
	if ptype == 0x38 {
		off += 12 // skip RGB1, RGB2, RGB3
	}
	if off+4 <= len(b) {
		tpage := binary.LittleEndian.Uint32(b[off : off+4])
		f0.TexPage = tpage
		f1.TexPage = tpage
		f0.Clut = tpage
		f1.Clut = tpage
	}
	// Triangle 0: V0, V1, V2
	f0.V0, f0.V1, f0.V2 = v[0], v[1], v[2]
	f0.UV[0], f0.UV[1], f0.UV[2] = uv[0], uv[1], uv[2]
	// Triangle 1: V0, V2, V3
	f1.V0, f1.V1, f1.V2 = v[0], v[2], v[3]
	f1.UV[0], f1.UV[1], f1.UV[2] = uv[0], uv[2], uv[3]
	return f0, f1, nil
}
