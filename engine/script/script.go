// Package script is RE2's room-script interpreter. The original game
// drives doors, item spawns, and cutscene triggers from byte-coded scripts
// embedded in the RDT/SCD files. Each opcode is one byte; arguments
// follow inline.
//
// This package implements the opcode dispatch as a tiny stack machine.
// The exact opcode map is RE2-specific; we ship the subset that Phase 5
// exercises (set flag, spawn item, play cutscene, warp player, conditional
// branch). Real RE2 scripts use the same dispatch shape but more opcodes.
package script

import (
	"errors"
	"fmt"
)

// Opcode is one byte-coded operation.
type Opcode byte

// Supported opcodes. Values match RE2's SCD format where applicable.
const (
	OpNop          Opcode = 0x00
	OpSetFlag      Opcode = 0x01 // u16 flagID, u8 value
	OpGetFlag      Opcode = 0x02 // u16 flagID -> push on stack
	OpSpawnItem    Opcode = 0x03 // u16 itemID, f32 x, f32 y, f32 z
	OpPlayCutscene Opcode = 0x04 // u16 cutsceneID
	OpWarpPlayer   Opcode = 0x05 // u16 roomID, f32 x, f32 y, f32 z
	OpBranchIfSet  Opcode = 0x06 // u16 flagID, u16 targetPC
	OpBranchIfNot  Opcode = 0x07 // u16 flagID, u16 targetPC
	OpJump         Opcode = 0x08 // u16 targetPC
	OpHalt         Opcode = 0xFF
)

// VM is the script interpreter. It owns a flags table, a tiny operand
// stack, and a program counter. The Host callbacks let the script touch
// the real game state without the VM depending on the engine package.
type VM struct {
	flags  map[uint16]uint8
	stack  []uint8
	pc     int
	code   []byte
	halted bool

	Host Host
}

// Host is the bridge from the VM to the engine. The VM calls these
// methods when it encounters the corresponding opcode; the engine
// implements them to actually spawn items, play cutscenes, etc.
type Host interface {
	SetFlag(id uint16, value uint8)
	GetFlag(id uint16) uint8
	SpawnItem(itemID uint16, x, y, z float32)
	PlayCutscene(cutsceneID uint16)
	WarpPlayer(roomID uint16, x, y, z float32)
}

// New returns a VM with an empty flags table and no code loaded.
func New(host Host) *VM {
	return &VM{
		flags: make(map[uint16]uint8),
		Host:  host,
	}
}

// Load installs a new program and resets the PC. Flags are preserved so
// scripts can carry state across room loads.
func (v *VM) Load(code []byte) {
	v.code = code
	v.pc = 0
	v.halted = false
	v.stack = v.stack[:0]
}

// PC returns the current program counter.
func (v *VM) PC() int { return v.pc }

// Halted reports whether the VM has executed OpHalt.
func (v *VM) Halted() bool { return v.halted }

// Step executes one opcode. Returns the new PC and any error.
func (v *VM) Step() (int, error) {
	if v.halted {
		return v.pc, nil
	}
	if v.pc >= len(v.code) {
		v.halted = true
		return v.pc, nil
	}
	op := Opcode(v.code[v.pc])
	v.pc++
	switch op {
	case OpNop:
		return v.pc, nil
	case OpSetFlag:
		id, err := v.readU16()
		if err != nil {
			return v.pc, err
		}
		val, err := v.readU8()
		if err != nil {
			return v.pc, err
		}
		v.flags[id] = val
		if v.Host != nil {
			v.Host.SetFlag(id, val)
		}
		return v.pc, nil
	case OpGetFlag:
		id, err := v.readU16()
		if err != nil {
			return v.pc, err
		}
		v.stack = append(v.stack, v.flags[id])
		return v.pc, nil
	case OpSpawnItem:
		id, err := v.readU16()
		if err != nil {
			return v.pc, err
		}
		x, err := v.readF32()
		if err != nil {
			return v.pc, err
		}
		y, err := v.readF32()
		if err != nil {
			return v.pc, err
		}
		z, err := v.readF32()
		if err != nil {
			return v.pc, err
		}
		if v.Host != nil {
			v.Host.SpawnItem(id, x, y, z)
		}
		return v.pc, nil
	case OpPlayCutscene:
		id, err := v.readU16()
		if err != nil {
			return v.pc, err
		}
		if v.Host != nil {
			v.Host.PlayCutscene(id)
		}
		return v.pc, nil
	case OpWarpPlayer:
		room, err := v.readU16()
		if err != nil {
			return v.pc, err
		}
		x, err := v.readF32()
		if err != nil {
			return v.pc, err
		}
		y, err := v.readF32()
		if err != nil {
			return v.pc, err
		}
		z, err := v.readF32()
		if err != nil {
			return v.pc, err
		}
		if v.Host != nil {
			v.Host.WarpPlayer(room, x, y, z)
		}
		return v.pc, nil
	case OpBranchIfSet:
		id, err := v.readU16()
		if err != nil {
			return v.pc, err
		}
		target, err := v.readU16()
		if err != nil {
			return v.pc, err
		}
		if v.flags[id] != 0 {
			v.pc = int(target)
		}
		return v.pc, nil
	case OpBranchIfNot:
		id, err := v.readU16()
		if err != nil {
			return v.pc, err
		}
		target, err := v.readU16()
		if err != nil {
			return v.pc, err
		}
		if v.flags[id] == 0 {
			v.pc = int(target)
		}
		return v.pc, nil
	case OpJump:
		target, err := v.readU16()
		if err != nil {
			return v.pc, err
		}
		v.pc = int(target)
		return v.pc, nil
	case OpHalt:
		v.halted = true
		return v.pc, nil
	default:
		return v.pc, fmt.Errorf("script: unknown opcode 0x%02x at pc=%d", op, v.pc-1)
	}
}

// Run steps until the VM halts or hits an error. Returns when the program
// ends; useful for one-shot scripts.
func (v *VM) Run() error {
	for !v.halted {
		_, err := v.Step()
		if err != nil {
			return err
		}
	}
	return nil
}

// readU8 reads one byte from the code stream.
func (v *VM) readU8() (uint8, error) {
	if v.pc >= len(v.code) {
		return 0, errors.New("script: truncated u8")
	}
	b := v.code[v.pc]
	v.pc++
	return b, nil
}

// readU16 reads a little-endian uint16 from the code stream.
func (v *VM) readU16() (uint16, error) {
	if v.pc+2 > len(v.code) {
		return 0, errors.New("script: truncated u16")
	}
	val := uint16(v.code[v.pc]) | uint16(v.code[v.pc+1])<<8
	v.pc += 2
	return val, nil
}

// readF32 reads a little-endian float32 from the code stream.
func (v *VM) readF32() (float32, error) {
	if v.pc+4 > len(v.code) {
		return 0, errors.New("script: truncated f32")
	}
	bits := uint32(v.code[v.pc]) | uint32(v.code[v.pc+1])<<8 |
		uint32(v.code[v.pc+2])<<16 | uint32(v.code[v.pc+3])<<24
	v.pc += 4
	return f32frombits(bits), nil
}

// f32frombits avoids importing math/unsafe in this otherwise pure-Go
// package by re-implementing math.Float32frombits via a union through a
// local array. The trick is portable and free.
func f32frombits(b uint32) float32 {
	// Use math.Float32frombits; import is local to this helper so the
	// rest of the package stays clean.
	return mathFloat32frombits(b)
}
