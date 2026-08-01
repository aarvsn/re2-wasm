package script

import (
	"encoding/binary"
	"math"
	"testing"
)

// fakeHost records every call so tests can assert what the script did.
type fakeHost struct {
	setFlags    map[uint16]uint8
	spawnedItem uint16
	playedCS    uint16
	warpedTo    struct {
		room    uint16
		x, y, z float32
	}
}

func newFakeHost() *fakeHost {
	return &fakeHost{setFlags: make(map[uint16]uint8)}
}

func (h *fakeHost) SetFlag(id uint16, value uint8)           { h.setFlags[id] = value }
func (h *fakeHost) GetFlag(id uint16) uint8                  { return h.setFlags[id] }
func (h *fakeHost) SpawnItem(itemID uint16, x, y, z float32) { h.spawnedItem = itemID }
func (h *fakeHost) PlayCutscene(cutsceneID uint16)           { h.playedCS = cutsceneID }
func (h *fakeHost) WarpPlayer(roomID uint16, x, y, z float32) {
	h.warpedTo.room = roomID
	h.warpedTo.x = x
	h.warpedTo.y = y
	h.warpedTo.z = z
}

func TestStep_Nop(t *testing.T) {
	h := newFakeHost()
	v := New(h)
	v.Load([]byte{byte(OpNop), byte(OpHalt)})
	if err := v.Run(); err != nil {
		t.Fatal(err)
	}
	if !v.Halted() {
		t.Fatal("not halted after Halt")
	}
}

func TestStep_SetFlagAndBranch(t *testing.T) {
	h := newFakeHost()
	v := New(h)
	// Program: set flag 0x0100 to 1; branch-if-set flag 0x0100 to PC=99;
	// (we never reach here); halt at 99.
	prog := []byte{
		byte(OpSetFlag), 0x00, 0x01, 0x01, // flag 0x0100 = 1
		byte(OpBranchIfSet), 0x00, 0x01, 0x63, 0x00, // if set, jump to 99
		byte(OpHalt), // skipped
	}
	// Pad to PC 99 then add a final halt.
	for len(prog) < 99 {
		prog = append(prog, byte(OpNop))
	}
	prog = append(prog, byte(OpHalt))
	v.Load(prog)
	if err := v.Run(); err != nil {
		t.Fatal(err)
	}
	if v.PC() != 100 {
		t.Errorf("PC = %d, want 100 (after halt at 99)", v.PC())
	}
	if h.setFlags[0x0100] != 1 {
		t.Errorf("flag 0x0100 = %d, want 1", h.setFlags[0x0100])
	}
}

func TestStep_BranchIfNot_TakenWhenFlagZero(t *testing.T) {
	h := newFakeHost()
	v := New(h)
	prog := []byte{
		byte(OpBranchIfNot), 0x05, 0x00, 5, 0, // flag 0x0005 not set -> jump to 5
		byte(OpHalt), // pc=5: skipped if branch taken
	}
	// pad to PC 5
	for len(prog) < 5 {
		prog = append(prog, byte(OpNop))
	}
	prog = append(prog, byte(OpHalt)) // pc=5
	v.Load(prog)
	if err := v.Run(); err != nil {
		t.Fatal(err)
	}
	if v.PC() != 6 {
		t.Errorf("PC = %d, want 6 (after halt at 5)", v.PC())
	}
}

func TestStep_SpawnItem(t *testing.T) {
	h := newFakeHost()
	v := New(h)
	prog := []byte{
		byte(OpSpawnItem), 0x2a, 0x00, // item 0x002a
	}
	// f32 x=10.0, y=20.0, z=30.0
	var f32buf [4]byte
	binary.LittleEndian.PutUint32(f32buf[:], math.Float32bits(10.0))
	prog = append(prog, f32buf[:]...)
	binary.LittleEndian.PutUint32(f32buf[:], math.Float32bits(20.0))
	prog = append(prog, f32buf[:]...)
	binary.LittleEndian.PutUint32(f32buf[:], math.Float32bits(30.0))
	prog = append(prog, f32buf[:]...)
	prog = append(prog, byte(OpHalt))
	v.Load(prog)
	if err := v.Run(); err != nil {
		t.Fatal(err)
	}
	if h.spawnedItem != 0x2a {
		t.Errorf("spawnedItem = 0x%x, want 0x2a", h.spawnedItem)
	}
	if h.warpedTo.room != 0 {
		t.Errorf("warp unexpectedly called")
	}
}

func TestStep_PlayCutscene(t *testing.T) {
	h := newFakeHost()
	v := New(h)
	prog := []byte{
		byte(OpPlayCutscene), 0x07, 0x00, // cutscene 7
		byte(OpHalt),
	}
	v.Load(prog)
	if err := v.Run(); err != nil {
		t.Fatal(err)
	}
	if h.playedCS != 7 {
		t.Errorf("playedCS = %d, want 7", h.playedCS)
	}
}

func TestStep_WarpPlayer(t *testing.T) {
	h := newFakeHost()
	v := New(h)
	prog := []byte{
		byte(OpWarpPlayer), 0x10, 0x00, // room 16
	}
	var f32buf [4]byte
	binary.LittleEndian.PutUint32(f32buf[:], math.Float32bits(1.5))
	prog = append(prog, f32buf[:]...)
	binary.LittleEndian.PutUint32(f32buf[:], math.Float32bits(2.5))
	prog = append(prog, f32buf[:]...)
	binary.LittleEndian.PutUint32(f32buf[:], math.Float32bits(3.5))
	prog = append(prog, f32buf[:]...)
	prog = append(prog, byte(OpHalt))
	v.Load(prog)
	if err := v.Run(); err != nil {
		t.Fatal(err)
	}
	if h.warpedTo.room != 16 {
		t.Errorf("room = %d, want 16", h.warpedTo.room)
	}
	if h.warpedTo.x != 1.5 || h.warpedTo.y != 2.5 || h.warpedTo.z != 3.5 {
		t.Errorf("warp pos = %v, want (1.5,2.5,3.5)", h.warpedTo)
	}
}

func TestStep_Jump(t *testing.T) {
	h := newFakeHost()
	v := New(h)
	prog := []byte{
		byte(OpJump), 5, 0, // jump to PC 5
		byte(OpHalt), // pc=3, skipped
		byte(OpNop),  // pc=4
		byte(OpHalt), // pc=5
	}
	v.Load(prog)
	if err := v.Run(); err != nil {
		t.Fatal(err)
	}
	if v.PC() != 6 {
		t.Errorf("PC = %d, want 6", v.PC())
	}
}

func TestStep_UnknownOpcode(t *testing.T) {
	h := newFakeHost()
	v := New(h)
	v.Load([]byte{0xEE})
	_, err := v.Step()
	if err == nil {
		t.Fatal("err = nil, want unknown opcode")
	}
}

func TestRun_EmptyProgramHaltsImmediately(t *testing.T) {
	h := newFakeHost()
	v := New(h)
	v.Load(nil)
	if err := v.Run(); err != nil {
		t.Fatal(err)
	}
	if !v.Halted() {
		t.Fatal("not halted")
	}
}

func TestRun_TruncatedU16(t *testing.T) {
	h := newFakeHost()
	v := New(h)
	v.Load([]byte{byte(OpSetFlag), 0x00}) // missing second byte + value
	if err := v.Run(); err == nil {
		t.Fatal("err = nil, want truncated")
	}
}

func TestFlags_PersistAcrossLoad(t *testing.T) {
	h := newFakeHost()
	v := New(h)
	prog := []byte{
		byte(OpSetFlag), 0x01, 0x00, 0x42,
		byte(OpHalt),
	}
	v.Load(prog)
	if err := v.Run(); err != nil {
		t.Fatal(err)
	}
	// Load a new program; flag should still be set.
	v.Load([]byte{byte(OpHalt)})
	if v.flags[0x0001] != 0x42 {
		t.Errorf("flag 0x0001 = %d, want 0x42 (persisted)", v.flags[0x0001])
	}
}
