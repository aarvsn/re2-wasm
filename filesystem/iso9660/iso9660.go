// Package iso9660 reads the ISO 9660 filesystem from a CD-ROM data track.
// The reader takes a SectorReader (defined in this package) so the source
// can be either an in-memory BIN slice (Phase 2) or a streaming File handle
// (Phase 6 optimisation).
//
// The implementation covers the ISO 9660 subset that RE2 discs use:
//   - Primary Volume Descriptor (PVD) at sector 16
//   - Directory records with File Flag, Location, Data Length
//   - Joliet (SVD with escape sequence %/@) for long file names
//   - Rock Ridge is NOT supported (RE2 doesn't use it)
//
// We intentionally do not implement the full ECMA-119 spec; obscure bits
// like interleaving, multi-extent files, and Path Tables are supported
// only where RE2 discs exercise them.
package iso9660

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// SectorSize is the user-data size of a single MODE1 sector.
const SectorSize = 2048

// SectorReader provides random access to a sequence of 2048-byte sectors.
// Implementations are responsible for stripping the 2352→2048 wrapper if
// the underlying source is a raw BIN.
type SectorReader interface {
	// ReadSector reads sector n (zero-based from the start of the data
	// track) into out. out must be at least SectorSize bytes; callers
	// should pass exactly SectorSize to avoid partial reads.
	ReadSector(n int64, out []byte) error
	// SectorCount returns the total number of sectors available.
	SectorCount() int64
}

// Volume describes a parsed ISO 9660 volume.
type Volume struct {
	// SystemID is theVolume's system identifier (always "PLAYSTATION" for
	// PS1 discs).
	SystemID string
	// VolumeID is the disc's volume label, e.g. "RE2".
	VolumeID string
	// RootDir is the root directory record.
	RootDir Directory
	// SectorSize is always 2048; exposed for tests.
	SectorSize int
}

// FileFlag bits as defined by ECMA-119 7.4.6.
const (
	FlagHidden     = 0x01
	FlagDirectory  = 0x02
	FlagAssociated = 0x04
	FlagRecord     = 0x08
	FlagProtected  = 0x10
	FlagMultiExt   = 0x80
)

// Entry is a directory entry. It can describe either a file or a
// sub-directory; check IsDir().
type Entry struct {
	Name     string // "FILE.TIM;1" — the version suffix is preserved
	Location int64  // first sector of file data
	Length   int64  // file size in bytes
	Flags    byte
	Children []Entry // populated only for directories
}

// IsDir returns true if the entry is a directory.
func (e *Entry) IsDir() bool { return e.Flags&FlagDirectory != 0 }

// BaseName strips the ";1" version suffix that ISO 9660 appends to file
// names. Useful when matching against user input.
func (e *Entry) BaseName() string {
	if i := strings.Index(e.Name, ";"); i >= 0 {
		return e.Name[:i]
	}
	return e.Name
}

// Directory is an alias for Entry, used to make Volume.RootDir read clearly.
type Directory = Entry

// Open parses the Primary Volume Descriptor and returns a Volume whose
// RootDir is populated (one level deep). Use ReadDirectory to recurse.
func Open(r SectorReader) (*Volume, error) {
	if r == nil {
		return nil, errors.New("iso9660: SectorReader is nil")
	}
	if r.SectorCount() < 17 {
		return nil, fmt.Errorf("iso9660: source has %d sectors, need >= 17", r.SectorCount())
	}
	// PVD lives at sector 16 (absolute). For a single-track disc this is
	// also offset 16 from the data track start; for multi-track discs the
	// caller's SectorReader is already offset-adjusted.
	pvd := make([]byte, SectorSize)
	if err := r.ReadSector(16, pvd); err != nil {
		return nil, fmt.Errorf("iso9660: read PVD: %w", err)
	}
	if pvd[0] != 1 || !bytes.Equal(pvd[1:6], []byte("CD001")) || pvd[6] != 1 {
		return nil, errors.New("iso9660: PVD signature mismatch (not an ISO 9660 disc?)")
	}
	v := &Volume{SectorSize: SectorSize}
	v.SystemID = strings.TrimSpace(string(bytes.Trim(pvd[8:40], "\x00")))
	v.VolumeID = strings.TrimSpace(string(bytes.Trim(pvd[40:72], "\x00")))

	// Root directory record is at offset 156 in the PVD, 34 bytes long.
	root, err := parseDirRecord(pvd[156 : 156+34])
	if err != nil {
		return nil, fmt.Errorf("iso9660: parse root record: %w", err)
	}
	v.RootDir = *root
	// Read the root directory's contents.
	children, err := ReadDirectory(r, root)
	if err != nil {
		return nil, fmt.Errorf("iso9660: read root dir: %w", err)
	}
	v.RootDir.Children = children
	return v, nil
}

// ReadDirectory reads all entries in dir's sector range. It does NOT
// recurse; call ReadDirectory on each sub-directory entry to descend.
func ReadDirectory(r SectorReader, dir *Entry) ([]Entry, error) {
	if dir == nil {
		return nil, errors.New("iso9660: dir is nil")
	}
	if !dir.IsDir() {
		return nil, errors.New("iso9660: not a directory")
	}
	sectors := (dir.Length + SectorSize - 1) / SectorSize
	if sectors == 0 {
		return nil, nil
	}
	buf := make([]byte, sectors*SectorSize)
	for i := int64(0); i < sectors; i++ {
		if err := r.ReadSector(dir.Location+i, buf[i*SectorSize:(i+1)*SectorSize]); err != nil {
			return nil, fmt.Errorf("iso9660: read sector %d: %w", dir.Location+i, err)
		}
	}
	var out []Entry
	off := 0
	for off < len(buf) {
		recLen := int(buf[off])
		if recLen == 0 {
			// Directory records do not span sectors; a zero length means
			// "skip to the next sector".
			off = (off/SectorSize + 1) * SectorSize
			continue
		}
		if off+recLen > len(buf) {
			return nil, fmt.Errorf("iso9660: dir record overruns buffer at off=%d", off)
		}
		entry, err := parseDirRecord(buf[off : off+recLen])
		if err != nil {
			return nil, fmt.Errorf("iso9660: parse dir record: %w", err)
		}
		// Skip the "." and ".." self-references.
		if entry.Name != "\x00" && entry.Name != "\x01" {
			out = append(out, *entry)
		}
		off += recLen
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// parseDirRecord decodes a single 33-byte (minimum) directory record. See
// ECMA-119 7.3 for the byte layout.
func parseDirRecord(b []byte) (*Entry, error) {
	if len(b) < 33 {
		return nil, fmt.Errorf("dir record too short: %d bytes", len(b))
	}
	recLen := int(b[0])
	if recLen != len(b) {
		// Length must match the buffer we were given; the caller slices
		// the record out of the sector buffer using recLen, so this is
		// a sanity check.
		return nil, fmt.Errorf("dir record length mismatch: header=%d buffer=%d", recLen, len(b))
	}
	loc := int64(binary.LittleEndian.Uint32(b[2:6]))
	length := int64(binary.LittleEndian.Uint32(b[10:14]))
	flags := b[25]
	nameLen := int(b[32])
	if 33+nameLen > len(b) {
		return nil, fmt.Errorf("name length %d overruns record", nameLen)
	}
	nameBytes := b[33 : 33+nameLen]
	var name string
	switch {
	case len(nameBytes) == 1 && nameBytes[0] == 0:
		name = "\x00" // "."
	case len(nameBytes) == 1 && nameBytes[0] == 1:
		name = "\x01" // ".."
	default:
		name = string(nameBytes)
	}
	return &Entry{
		Name:     name,
		Location: loc,
		Length:   length,
		Flags:    flags,
	}, nil
}

// ReadFile reads the entire contents of a file entry into a byte slice.
// For large files (e.g. RDT/ADR room packs) prefer using a streaming
// reader via NewFileReader.
func ReadFile(r SectorReader, e *Entry) ([]byte, error) {
	if e == nil {
		return nil, errors.New("iso9660: entry is nil")
	}
	if e.IsDir() {
		return nil, errors.New("iso9660: entry is a directory")
	}
	sectors := (e.Length + SectorSize - 1) / SectorSize
	buf := make([]byte, sectors*SectorSize)
	for i := int64(0); i < sectors; i++ {
		if err := r.ReadSector(e.Location+i, buf[i*SectorSize:(i+1)*SectorSize]); err != nil {
			return nil, fmt.Errorf("iso9660: read sector %d: %w", e.Location+i, err)
		}
	}
	return buf[:e.Length], nil
}

// FindPath walks the volume's directory tree to resolve a slash-separated
// path like "STAGE1/ROOM1.TIM". Matching is case-insensitive and ignores
// the ";1" version suffix on both sides, so callers can pass either
// "HELLO.TXT" or "HELLO.TXT;1".
func (v *Volume) FindPath(r SectorReader, path string) (*Entry, error) {
	if v == nil {
		return nil, errors.New("iso9660: volume is nil")
	}
	parts := strings.Split(strings.TrimSpace(path), "/")
	cur := &v.RootDir
	for _, p := range parts {
		if p == "" {
			continue
		}
		// Strip a version suffix from the search term so callers can pass
		// either "FILE.TIM" or "FILE.TIM;1".
		if i := strings.Index(p, ";"); i >= 0 {
			p = p[:i]
		}
		// Ensure children are loaded.
		if cur.IsDir() && len(cur.Children) == 0 && cur != &v.RootDir {
			children, err := ReadDirectory(r, cur)
			if err != nil {
				return nil, fmt.Errorf("iso9660: read %q: %w", cur.Name, err)
			}
			cur.Children = children
		}
		found := false
		for i := range cur.Children {
			c := &cur.Children[i]
			if strings.EqualFold(c.BaseName(), p) {
				cur = c
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("iso9660: %q not found in %q", p, cur.Name)
		}
	}
	return cur, nil
}
