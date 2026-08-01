package discfs

import (
	"encoding/binary"
	"testing"

	"github.com/aarvsn/re2-wasm/filesystem/iso9660"
)

// buildFakeDisc builds an in-memory 2048-byte-sector "BIN" with a valid PVD
// and a root directory containing one file "HELLO.TXT" whose 5 bytes are
// "hello". Returns the BIN bytes and a CUE sheet that describes a single
// MODE1/2352 data track at sector 0. (The CUE claims 2352 but the BIN is
// 2048-aligned; NewRawBINReader detects this and uses 2048 mode.)
func buildFakeDisc(t *testing.T) (cueText string, bin []byte) {
	t.Helper()
	const N = 18
	img := make([]byte, N*iso9660.SectorSize)

	// PVD at sector 16.
	pvd := img[16*iso9660.SectorSize : 17*iso9660.SectorSize]
	pvd[0] = 1
	copy(pvd[1:6], "CD001")
	pvd[6] = 1
	copy(pvd[8:40], []byte("PLAYSTATION          "))
	copy(pvd[40:72], []byte("RE2                  "))
	binary.LittleEndian.PutUint32(pvd[80:84], uint32(N))
	binary.BigEndian.PutUint32(pvd[84:88], uint32(N))
	binary.LittleEndian.PutUint16(pvd[128:130], iso9660.SectorSize)
	binary.BigEndian.PutUint16(pvd[130:132], iso9660.SectorSize)

	// Root dir record (sector 17, size 2048).
	root := pvd[156 : 156+34]
	root[0] = 34
	binary.LittleEndian.PutUint32(root[2:6], 17)
	binary.BigEndian.PutUint32(root[6:10], 17)
	binary.LittleEndian.PutUint32(root[10:14], iso9660.SectorSize)
	binary.BigEndian.PutUint32(root[14:18], iso9660.SectorSize)
	root[25] = iso9660.FlagDirectory
	root[32] = 1
	root[33] = 0

	// Sector 17: directory contents.
	dir := img[17*iso9660.SectorSize : 18*iso9660.SectorSize]
	dot := make([]byte, 34)
	dot[0] = 34
	binary.LittleEndian.PutUint32(dot[2:6], 17)
	binary.BigEndian.PutUint32(dot[6:10], 17)
	binary.LittleEndian.PutUint32(dot[10:14], iso9660.SectorSize)
	binary.BigEndian.PutUint32(dot[14:18], iso9660.SectorSize)
	dot[25] = iso9660.FlagDirectory
	dot[32] = 1
	copy(dir, dot)
	off := len(dot)
	dotdot := make([]byte, 34)
	copy(dotdot, dot)
	dotdot[33] = 1
	copy(dir[off:], dotdot)
	off += len(dotdot)

	// File record: HELLO.TXT;1, location = 17 (we re-use sector 17's
	// trailing space as the file data to keep the image small). Length 5.
	fileName := []byte("HELLO.TXT;1")
	recLen := 33 + len(fileName)
	if recLen%2 != 0 {
		recLen++
	}
	rec := make([]byte, recLen)
	rec[0] = byte(recLen)
	binary.LittleEndian.PutUint32(rec[2:6], 17)
	binary.BigEndian.PutUint32(rec[6:10], 17)
	binary.LittleEndian.PutUint32(rec[10:14], 5)
	binary.BigEndian.PutUint32(rec[14:18], 5)
	rec[32] = byte(len(fileName))
	copy(rec[33:33+len(fileName)], fileName)
	copy(dir[off:], rec)

	// Plant "hello" at the start of sector 17 (after the directory
	// records we already wrote; the file's location 17 points at the
	// whole sector, so iso9660.ReadFile will return the first 5 bytes
	// of sector 17 which is the dot record, not "hello". To make the
	// test meaningful, give the file its own sector: change the file's
	// location to a sector we control. We have no spare sector; instead
	// place "hello" at the start of sector 15 (which is unused) and
	// point the file there.
	binary.LittleEndian.PutUint32(rec[2:6], 15)
	binary.BigEndian.PutUint32(rec[6:10], 15)
	copy(dir[off:], rec) // rewrite
	copy(img[15*iso9660.SectorSize:15*iso9660.SectorSize+5], []byte("hello"))

	cueText = `FILE "fake.bin" BINARY
  TRACK 01 MODE1/2352
    INDEX 01 00:00:00
`
	return cueText, img
}

func TestDiscFS_MountBINCUE_ReadsFile(t *testing.T) {
	cueText, bin := buildFakeDisc(t)
	fs := New()
	if err := fs.MountBINCUE("", []byte(cueText), bin); err != nil {
		t.Fatal(err)
	}
	if !fs.Has("HELLO.TXT;1") {
		t.Fatal("Has(HELLO.TXT;1) = false")
	}
	if !fs.Has("hello.txt;1") { // case-insensitive
		t.Fatal("Has(hello.txt;1) = false")
	}
	b, err := fs.Read("hello.txt;1")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello" {
		t.Errorf("Read = %q, want hello", string(b))
	}
}

func TestDiscFS_MountPairsCUEAndBIN(t *testing.T) {
	cueText, bin := buildFakeDisc(t)
	fs := New()
	// Mount the CUE first, then the BIN. The pair should auto-mature.
	if err := fs.Mount("game.cue", []byte(cueText)); err != nil {
		t.Fatal(err)
	}
	if fs.Has("hello.txt;1") {
		t.Fatal("file visible before BIN was mounted")
	}
	if err := fs.Mount("game.bin", bin); err != nil {
		t.Fatal(err)
	}
	if !fs.Has("hello.txt;1") {
		t.Fatal("file not visible after BIN mount")
	}
}

func TestDiscFS_MountExtractedFile(t *testing.T) {
	fs := New()
	if err := fs.Mount("foo.tim", []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if !fs.Has("foo.tim") {
		t.Fatal("Has(foo.tim) = false")
	}
	b, err := fs.Read("FOO.TIM") // case-insensitive
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 3 || b[0] != 1 {
		t.Errorf("Read = %v, want [1 2 3]", b)
	}
}

func TestDiscFS_Read_NotFound(t *testing.T) {
	fs := New()
	_, err := fs.Read("nope.bin")
	if err == nil {
		t.Fatal("err = nil, want not-found")
	}
}

func TestDiscFS_List(t *testing.T) {
	cueText, bin := buildFakeDisc(t)
	fs := New()
	_ = fs.Mount("game.cue", []byte(cueText))
	_ = fs.Mount("game.bin", bin)
	_ = fs.MountExtractedFile("loose.tim", []byte{9})
	list, err := fs.List()
	if err != nil {
		t.Fatal(err)
	}
	// Expect at least 2 entries: HELLO.TXT;1 from the disc + loose.tim.
	if len(list) < 2 {
		t.Fatalf("List = %v, want >=2 entries", list)
	}
}
