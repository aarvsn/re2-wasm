package iso9660

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/aarvsn/re2-wasm/filesystem/cue"
)

// buildSyntheticISO constructs a minimal in-memory ISO 9660 image with a
// root directory containing one tiny file. The image is 18 sectors so it
// satisfies the ">= 17 sectors" check.
func buildSyntheticISO(t *testing.T) []byte {
	t.Helper()
	const N = 18
	img := make([]byte, N*SectorSize)
	// Sector 16: Primary Volume Descriptor.
	pvd := img[16*SectorSize : 17*SectorSize]
	pvd[0] = 1
	copy(pvd[1:6], "CD001")
	pvd[6] = 1
	copy(pvd[8:40], []byte("PLAYSTATION          "))
	copy(pvd[40:72], []byte("RE2                  "))
	binary.LittleEndian.PutUint32(pvd[80:84], uint32(N))
	binary.BigEndian.PutUint32(pvd[84:88], uint32(N))
	binary.LittleEndian.PutUint16(pvd[120:122], 1)
	binary.BigEndian.PutUint16(pvd[122:124], 1)
	binary.LittleEndian.PutUint16(pvd[124:126], 1)
	binary.BigEndian.PutUint16(pvd[126:128], 1)
	binary.LittleEndian.PutUint16(pvd[128:130], SectorSize)
	binary.BigEndian.PutUint16(pvd[130:132], SectorSize)
	// Root directory record at offset 156.
	root := pvd[156 : 156+34]
	root[0] = 34
	binary.LittleEndian.PutUint32(root[2:6], 17)
	binary.BigEndian.PutUint32(root[6:10], 17)
	binary.LittleEndian.PutUint32(root[10:14], SectorSize)
	binary.BigEndian.PutUint32(root[14:18], SectorSize)
	root[25] = FlagDirectory
	root[32] = 1
	root[33] = 0 // "."
	// Sector 17: root directory contents. The "." entry mirrors the root
	// record: location=17, length=2048 (SectorSize, little-endian),
	// flags=FlagDirectory.
	dir := img[17*SectorSize : 18*SectorSize]
	dot := make([]byte, 34)
	dot[0] = 34
	binary.LittleEndian.PutUint32(dot[2:6], 17)
	binary.BigEndian.PutUint32(dot[6:10], 17)
	binary.LittleEndian.PutUint32(dot[10:14], SectorSize)
	binary.BigEndian.PutUint32(dot[14:18], SectorSize)
	dot[25] = FlagDirectory
	dot[32] = 1
	dot[33] = 0
	copy(dir, dot)
	off := len(dot)
	dotdot := make([]byte, 34)
	copy(dotdot, dot)
	dotdot[33] = 1
	copy(dir[off:], dotdot)
	off += len(dotdot)
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
	rec[25] = 0
	rec[32] = byte(len(fileName))
	copy(rec[33:33+len(fileName)], fileName)
	copy(dir[off:], rec)
	return img
}

func TestOpen_ParsesPVD(t *testing.T) {
	img := buildSyntheticISO(t)
	r := NewMemSectorReader(img)
	v, err := Open(r)
	if err != nil {
		t.Fatal(err)
	}
	if v.SystemID != "PLAYSTATION" {
		t.Errorf("SystemID = %q, want PLAYSTATION", v.SystemID)
	}
	if v.VolumeID != "RE2" {
		t.Errorf("VolumeID = %q, want RE2", v.VolumeID)
	}
	if !v.RootDir.IsDir() {
		t.Error("RootDir is not a directory")
	}
}

func TestOpen_RejectsNonISO(t *testing.T) {
	img := make([]byte, 18*SectorSize)
	r := NewMemSectorReader(img)
	_, err := Open(r)
	if err == nil || !strings.Contains(err.Error(), "signature mismatch") {
		t.Fatalf("err = %v, want signature mismatch", err)
	}
}

func TestOpen_TooFewSectors(t *testing.T) {
	r := NewMemSectorReader(make([]byte, 5*SectorSize))
	_, err := Open(r)
	if err == nil || !strings.Contains(err.Error(), "need >= 17") {
		t.Fatalf("err = %v, want need >= 17", err)
	}
}

func TestReadDirectory_ListsFiles(t *testing.T) {
	img := buildSyntheticISO(t)
	r := NewMemSectorReader(img)
	v, err := Open(r)
	if err != nil {
		t.Fatal(err)
	}
	children, err := ReadDirectory(r, &v.RootDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 {
		t.Fatalf("children = %d, want 1 (%+v)", len(children), children)
	}
	if children[0].Name != "HELLO.TXT;1" {
		t.Errorf("name = %q, want HELLO.TXT;1", children[0].Name)
	}
	if children[0].Length != 5 {
		t.Errorf("length = %d, want 5", children[0].Length)
	}
	if children[0].IsDir() {
		t.Error("HELLO.TXT reported as directory")
	}
}

func TestEntry_BaseName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"HELLO.TXT;1", "HELLO.TXT"},
		{"NO_VERSION", "NO_VERSION"},
		{"A;1;2", "A"},
		{"", ""},
	}
	for _, c := range cases {
		e := Entry{Name: c.in}
		if got := e.BaseName(); got != c.want {
			t.Errorf("BaseName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMemSectorReader_OutOfRange(t *testing.T) {
	r := NewMemSectorReader(make([]byte, 5*SectorSize))
	out := make([]byte, SectorSize)
	if err := r.ReadSector(0, out); err != nil {
		t.Fatalf("ReadSector(0) err = %v", err)
	}
	if err := r.ReadSector(4, out); err != nil {
		t.Fatalf("ReadSector(4) err = %v", err)
	}
	if err := r.ReadSector(5, out); err == nil {
		t.Fatal("ReadSector(5) err=nil, want out-of-range")
	}
	if err := r.ReadSector(-1, out); err == nil {
		t.Fatal("ReadSector(-1) err=nil, want out-of-range")
	}
}

func TestMemSectorReader_BufferTooSmall(t *testing.T) {
	r := NewMemSectorReader(make([]byte, 5*SectorSize))
	out := make([]byte, 100)
	if err := r.ReadSector(0, out); err == nil {
		t.Fatal("err=nil, want buffer-too-small")
	}
}

func TestRawBINReader_2352Mode(t *testing.T) {
	// Build a 2352-byte-sector BIN. Plant a sentinel in the user-data area
	// of sector 17 (absolute). The data track starts at sector 0 per the
	// CUE sheet, so ReadSector(17) should return that sentinel.
	const sectors = 20
	bin := make([]byte, sectors*2352)
	bin[16*2352+16] = 1 // PVD marker at sector 16 (not used here, but realistic)
	copy(bin[16*2352+16+1:16*2352+16+6], "CD001")
	bin[16*2352+16+6] = 1
	want := []byte{0xde, 0xad, 0xbe, 0xef}
	copy(bin[17*2352+16:17*2352+16+4], want)

	sheet, err := cue.ParseString(`FILE "fake.bin" BINARY
  TRACK 01 MODE1/2352
    INDEX 01 00:00:00
`)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRawBINReader(bin, sheet)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]byte, SectorSize)
	if err := r.ReadSector(17, out); err != nil {
		t.Fatalf("ReadSector(17) err = %v", err)
	}
	if !bytes.Equal(out[:4], want) {
		t.Errorf("user data = %x, want %x", out[:4], want)
	}
}

func TestRawBINReader_2048Mode(t *testing.T) {
	// A BIN whose length is a multiple of 2048 but not 2352 should be
	// detected as already-stripped.
	const sectors = 20
	bin := make([]byte, sectors*2048)
	want := []byte{0xca, 0xfe}
	copy(bin[17*2048:17*2048+2], want)
	sheet, err := cue.ParseString(`FILE "fake.bin" BINARY
  TRACK 01 MODE1/2352
    INDEX 01 00:00:00
`)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRawBINReader(bin, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if r.sectorSize != 2048 {
		t.Errorf("sectorSize = %d, want 2048", r.sectorSize)
	}
	out := make([]byte, SectorSize)
	if err := r.ReadSector(17, out); err != nil {
		t.Fatalf("ReadSector(17) err = %v", err)
	}
	if !bytes.Equal(out[:2], want) {
		t.Errorf("user data = %x, want %x", out[:2], want)
	}
}

func TestRawBINReader_2048Mode_WithNonZeroDataStart(t *testing.T) {
	// If a BIN is 2048-aligned and already-stripped, it lacks the pregap.
	// Even if the CUE has INDEX 01 00:02:00 (offset = 150), NewRawBINReader
	// should treat the data start as 0 and read without offset.
	const sectors = 20
	bin := make([]byte, sectors*2048)
	want := []byte{0xbe, 0xef}
	copy(bin[17*2048:17*2048+2], want)
	sheet, err := cue.ParseString(`FILE "fake.bin" BINARY
  TRACK 01 MODE1/2352
    INDEX 01 00:02:00
`)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRawBINReader(bin, sheet)
	if err != nil {
		t.Fatal(err)
	}
	if r.sectorSize != 2048 {
		t.Errorf("sectorSize = %d, want 2048", r.sectorSize)
	}
	out := make([]byte, SectorSize)
	if err := r.ReadSector(17, out); err != nil {
		t.Fatalf("ReadSector(17) err = %v", err)
	}
	if !bytes.Equal(out[:2], want) {
		t.Errorf("user data = %x, want %x", out[:2], want)
	}
}

func TestRawBINReader_NoDataTrack(t *testing.T) {
	sheet, _ := cue.ParseString(`FILE "fake.bin" BINARY
  TRACK 01 AUDIO
    INDEX 01 00:00:00
`)
	_, err := NewRawBINReader(make([]byte, 100*2352), sheet)
	if err == nil || !strings.Contains(err.Error(), "no MODE1") {
		t.Fatalf("err = %v, want no MODE1", err)
	}
}
