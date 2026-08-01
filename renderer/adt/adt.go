// Package adt decodes RE2's ADT room geometry files. An ADT file packs
// the per-room mesh, texture-page assignments, and collision data into a
// single blob; the original engine streams the file directly off the CD
// and uploads vertex buffers per-frame.
//
// Phase 3's decoder is intentionally focused on the geometry: it walks
// the header, extracts the vertex / face arrays, and exposes them as
// common.Mesh-ready data. Collision volumes and light placements are
// preserved as raw byte ranges for Phase 5 to interpret.
package adt

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Magic is the ADT file magic. Real RE2 ADTs do not have a magic number
// per se; we use the leading u32 "version" tag (always 0x01) as a sanity
// check.
const Magic uint32 = 0x01

// Header is the 16-byte ADT file header.
type Header struct {
	Version   uint32
	RoomID    uint16
	Flags     uint16
	VertCount uint32
	FaceCount uint32
}

// Vertex is one ADT vertex: 4-byte XYZ fixed-point (s.3.12) + 1 byte of
// flags (unused in Phase 3).
type Vertex struct {
	X, Y, Z int16
	Flags   uint8
}

// Face is a triangle: three vertex indices, a texture-page assignment,
// and a CLUT page index. UVs are stored alongside the face rather than
// per-vertex because RE2 packs them that way.
type Face struct {
	V0, V1, V2 uint16
	UV         [3][2]uint16
	TexPage    uint16
	Clut       uint16
}

// Room is a decoded ADT file.
type Room struct {
	Header   Header
	Verts    []Vertex
	Faces    []Face
	RawExtra []byte // collision + light data, decoded in Phase 5
}

// Decode parses an ADT file from b. Returns an error if the buffer is
// truncated or the magic mismatches.
func Decode(b []byte) (*Room, error) {
	if len(b) < 16 {
		return nil, errors.New("adt: header too short")
	}
	h := Header{
		Version:   binary.LittleEndian.Uint32(b[0:4]),
		RoomID:    binary.LittleEndian.Uint16(b[4:6]),
		Flags:     binary.LittleEndian.Uint16(b[6:8]),
		VertCount: binary.LittleEndian.Uint32(b[8:12]),
		FaceCount: binary.LittleEndian.Uint32(b[12:16]),
	}
	if h.Version != Magic {
		return nil, fmt.Errorf("adt: bad version 0x%08x (want 0x%x)", h.Version, Magic)
	}

	off := 16
	vEnd := off + int(h.VertCount)*8
	if vEnd > len(b) {
		return nil, fmt.Errorf("adt: verts overflow (%d > %d)", vEnd, len(b))
	}
	verts := make([]Vertex, h.VertCount)
	for i := uint32(0); i < h.VertCount; i++ {
		vo := off + int(i)*8
		verts[i] = Vertex{
			X:     int16(binary.LittleEndian.Uint16(b[vo+0 : vo+2])),
			Y:     int16(binary.LittleEndian.Uint16(b[vo+2 : vo+4])),
			Z:     int16(binary.LittleEndian.Uint16(b[vo+4 : vo+6])),
			Flags: b[vo+6],
		}
	}
	off = vEnd

	// Face record = 24 bytes: 3 indices (6) + 3 UVs (12) + tpage (2) +
	// clut (2) + pad (2).
	fEnd := off + int(h.FaceCount)*24
	if fEnd > len(b) {
		return nil, fmt.Errorf("adt: faces overflow (%d > %d)", fEnd, len(b))
	}
	faces := make([]Face, h.FaceCount)
	for i := uint32(0); i < h.FaceCount; i++ {
		fo := off + int(i)*24
		f := Face{
			V0:      binary.LittleEndian.Uint16(b[fo+0 : fo+2]),
			V1:      binary.LittleEndian.Uint16(b[fo+2 : fo+4]),
			V2:      binary.LittleEndian.Uint16(b[fo+4 : fo+6]),
			TexPage: binary.LittleEndian.Uint16(b[fo+18 : fo+20]),
			Clut:    binary.LittleEndian.Uint16(b[fo+20 : fo+22]),
		}
		for j := 0; j < 3; j++ {
			uo := fo + 6 + j*4
			f.UV[j][0] = binary.LittleEndian.Uint16(b[uo+0 : uo+2])
			f.UV[j][1] = binary.LittleEndian.Uint16(b[uo+2 : uo+4])
		}
		faces[i] = f
	}
	off = fEnd

	// Everything after the face array is collision + light data; keep it
	// verbatim for Phase 5.
	var extra []byte
	if off < len(b) {
		extra = make([]byte, len(b)-off)
		copy(extra, b[off:])
	}

	return &Room{Header: h, Verts: verts, Faces: faces, RawExtra: extra}, nil
}

// VertexCount returns the number of vertices in the room. Convenience for
// the renderer's vertex-buffer allocation.
func (r *Room) VertexCount() int { return int(r.Header.VertCount) }

// FaceCount returns the number of faces.
func (r *Room) FaceCount() int { return int(r.Header.FaceCount) }
