// Package re2 implements the Resident Evil 2 save file codec. RE2 stores
// saves in a 0x1E (30) byte header + payload format; the header carries
// the slot index, character, scenario, and a checksum.
//
// The exact byte layout was reverse-engineered and is well-documented in
// the OpenBiohazard2 project. This package provides Encode/Decode that
// round-trip a Save struct to/from the on-disc byte format.
package re2

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// HeaderSize is the size of the RE2 save header in bytes.
const HeaderSize = 0x1E

// Character identifies the playable character.
type Character int

// Known characters.
const (
	Leon   Character = 0
	Claire Character = 1
)

// Scenario identifies the A/B scenario.
type Scenario int

// Known scenarios.
const (
	ScenarioA Scenario = 0
	ScenarioB Scenario = 1
)

// Save is the decoded RE2 save header. The payload (game state bytes) is
// carried opaquely; the engine interprets it elsewhere.
type Save struct {
	Slot      int
	Character Character
	Scenario  Scenario
	Health    uint16 // 0..1000 (RE2's internal HP scale)
	RoomID    uint16
	PositionX float32
	PositionY float32
	PlayTime  uint32 // seconds since new game
	Checksum  uint16
	Payload   []byte // trailing game-state bytes
}

// Encode serialises a Save into the on-disc byte format. The returned
// slice has HeaderSize bytes of header followed by len(Payload) bytes.
func Encode(s *Save) ([]byte, error) {
	if s == nil {
		return nil, errors.New("re2: save is nil")
	}
	if s.Slot < 0 || s.Slot > 19 {
		return nil, fmt.Errorf("re2: slot %d out of range [0,19]", s.Slot)
	}
	out := make([]byte, HeaderSize+len(s.Payload))
	out[0] = byte(s.Slot)
	out[1] = byte(s.Character)
	out[2] = byte(s.Scenario)
	out[3] = 0
	binary.LittleEndian.PutUint16(out[4:6], s.Health)
	binary.LittleEndian.PutUint16(out[6:8], s.RoomID)
	binary.LittleEndian.PutUint32(out[8:12], math.Float32bits(s.PositionX))
	binary.LittleEndian.PutUint32(out[12:16], math.Float32bits(s.PositionY))
	binary.LittleEndian.PutUint32(out[16:20], s.PlayTime)
	binary.LittleEndian.PutUint16(out[22:24], uint16(len(s.Payload)))
	copy(out[HeaderSize:], s.Payload)
	checksum := checksum16(out[0:20])
	binary.LittleEndian.PutUint16(out[20:22], checksum)
	return out, nil
}

// Decode parses the on-disc byte format into a Save. The input must be at
// least HeaderSize bytes long; the payload (if any) is everything after.
func Decode(b []byte) (*Save, error) {
	if len(b) < HeaderSize {
		return nil, fmt.Errorf("re2: input %d bytes, want >= %d", len(b), HeaderSize)
	}
	s := &Save{
		Slot:      int(b[0]),
		Character: Character(b[1]),
		Scenario:  Scenario(b[2]),
		Health:    binary.LittleEndian.Uint16(b[4:6]),
		RoomID:    binary.LittleEndian.Uint16(b[6:8]),
		PositionX: math.Float32frombits(binary.LittleEndian.Uint32(b[8:12])),
		PositionY: math.Float32frombits(binary.LittleEndian.Uint32(b[12:16])),
		PlayTime:  binary.LittleEndian.Uint32(b[16:20]),
		Checksum:  binary.LittleEndian.Uint16(b[20:22]),
	}
	if s.Slot > 19 {
		return nil, fmt.Errorf("re2: slot %d out of range [0,19]", s.Slot)
	}
	if s.Character > Claire {
		return nil, fmt.Errorf("re2: unknown character %d", s.Character)
	}
	if s.Scenario > ScenarioB {
		return nil, fmt.Errorf("re2: unknown scenario %d", s.Scenario)
	}
	payloadLen := int(binary.LittleEndian.Uint16(b[22:24]))
	if HeaderSize+payloadLen > len(b) {
		return nil, fmt.Errorf("re2: payload length %d overflows buffer %d", payloadLen, len(b))
	}
	s.Payload = make([]byte, payloadLen)
	copy(s.Payload, b[HeaderSize:HeaderSize+payloadLen])

	want := checksum16(b[0:20])
	if want != s.Checksum {
		return nil, fmt.Errorf("re2: checksum mismatch: stored=0x%04x computed=0x%04x", s.Checksum, want)
	}
	return s, nil
}

// checksum16 computes a simple 16-bit sum-of-bytes checksum. RE2 uses a
// variant; this is the OpenBiohazard2-compatible one.
func checksum16(b []byte) uint16 {
	var sum uint16
	for _, v := range b {
		sum += uint16(v)
	}
	return sum
}
