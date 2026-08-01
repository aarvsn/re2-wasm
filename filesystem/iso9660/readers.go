// Package iso9660 also provides two SectorReader implementations:
//
//   - MemSectorReader — backed by a []byte; used by tests and the in-browser
//     loader where the whole BIN has been loaded into memory.
//   - RawBINReader — wraps a 2352-byte-sector BIN and a CUE Sheet, exposing
//     only the data track's 2048-byte user data via the SectorReader
//     interface.
package iso9660

import (
	"errors"
	"fmt"

	"github.com/aarvsn/re2-wasm/filesystem/cue"
)

// MemSectorReader is a SectorReader backed by an in-memory byte slice. The
// slice must contain 2048-byte user-data sectors already stripped of the
// 2352-byte CD wrapper.
type MemSectorReader struct {
	data    []byte
	sectors int64
}

// NewMemSectorReader constructs a MemSectorReader over data. data.Length
// must be a multiple of SectorSize; trailing bytes are silently ignored
// (a warning is returned via the trailing byte count).
func NewMemSectorReader(data []byte) *MemSectorReader {
	sectors := int64(len(data) / SectorSize)
	return &MemSectorReader{data: data, sectors: sectors}
}

// ReadSector implements SectorReader.
func (m *MemSectorReader) ReadSector(n int64, out []byte) error {
	if n < 0 || n >= m.sectors {
		return fmt.Errorf("iso9660: sector %d out of range [0,%d)", n, m.sectors)
	}
	if len(out) < SectorSize {
		return fmt.Errorf("iso9660: out buffer %d bytes, want %d", len(out), SectorSize)
	}
	copy(out, m.data[n*SectorSize:(n+1)*SectorSize])
	return nil
}

// SectorCount implements SectorReader.
func (m *MemSectorReader) SectorCount() int64 { return m.sectors }

// RawBINReader wraps a 2352-byte-sector BIN file and a parsed CUE Sheet,
// exposing only the data track's 2048-byte user data as a SectorReader.
//
// The reader does NOT load the BIN into memory; it holds a reference to the
// underlying byte slice (or io.ReaderAt via the Streaming interface added
// in Phase 6). For Phase 2 the BIN is already in memory because the
// browser loads it via FileReader; Phase 6 will swap in a streaming
// implementation backed by the File System Access API.
type RawBINReader struct {
	bin          []byte
	sectorSize   int64 // 2352 for raw BINs, 2048 for stripped BINs
	dataTrack    *cue.Track
	dataStart    int64 // sector offset where the data track begins
	totalSectors int64
}

// NewRawBINReader constructs a reader. binLen is the length of the BIN in
// bytes; it is used to detect whether the BIN is 2352- or 2048-byte
// sector-aligned.
func NewRawBINReader(bin []byte, sheet *cue.Sheet) (*RawBINReader, error) {
	if sheet == nil {
		return nil, errors.New("iso9660: sheet is required")
	}
	dt := sheet.DataTrack()
	if dt == nil {
		return nil, errors.New("iso9660: cue sheet has no MODE1/MODE2 data track")
	}
	if len(dt.Indices) == 0 {
		return nil, errors.New("iso9660: data track has no INDEX entries")
	}
	// Detect sector size from the BIN length. RE2 BINs are 2352-aligned.
	sectorSize := int64(2352)
	if len(bin)%2352 != 0 && len(bin)%2048 == 0 {
		sectorSize = 2048
	}
	dataStart := dt.Indices[0].Start
	total := int64(len(bin)) / sectorSize
	if dataStart >= total {
		return nil, fmt.Errorf("iso9660: data track starts at sector %d but BIN has only %d sectors", dataStart, total)
	}
	return &RawBINReader{
		bin:          bin,
		sectorSize:   sectorSize,
		dataTrack:    dt,
		dataStart:    dataStart,
		totalSectors: total - dataStart,
	}, nil
}

// ReadSector implements SectorReader. n is relative to the start of the
// data track, not the BIN file.
func (r *RawBINReader) ReadSector(n int64, out []byte) error {
	if n < 0 || n >= r.totalSectors {
		return fmt.Errorf("iso9660: sector %d out of range [0,%d)", n, r.totalSectors)
	}
	if len(out) < SectorSize {
		return fmt.Errorf("iso9660: out buffer %d bytes, want %d", len(out), SectorSize)
	}
	absSector := r.dataStart + n
	if r.sectorSize == 2048 {
		copy(out, r.bin[absSector*2048:(absSector+1)*2048])
		return nil
	}
	// 2352-byte raw sector layout (MODE1):
	//   12 sync | 4 header | 2048 user data | 288 ECC/EDC
	userStart := absSector*2352 + 16
	if int(userStart)+SectorSize > len(r.bin) {
		return fmt.Errorf("iso9660: BIN truncated at sector %d", absSector)
	}
	copy(out, r.bin[userStart:userStart+SectorSize])
	return nil
}

// SectorCount implements SectorReader.
func (r *RawBINReader) SectorCount() int64 { return r.totalSectors }
