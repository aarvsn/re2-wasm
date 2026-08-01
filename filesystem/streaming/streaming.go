//go:build js && wasm

// Package streaming wraps the browser's File System Access API so the
// engine can read large BIN files without loading them entirely into
// memory. The browser provides a FileSystemFileHandle whosegetFile()
// returns a File with slice()/arrayBuffer() methods; we expose a
// SectorReader that pulls sectors on demand.
//
// When the File System Access API is unavailable (Safari, Firefox as of
// 2026) callers fall back to filesystem.MemoryFS, which is what loader.js
// already does via FileReader.
package streaming

import (
	"errors"
	"sync"
	"syscall/js"

	"github.com/aarvsn/re2-wasm/filesystem/cue"
	"github.com/aarvsn/re2-wasm/filesystem/iso9660"
)

// IsSupported reports whether navigator.storage.getDirectory is available.
// loader.js uses this to gate the "Open folder" button.
func IsSupported() bool {
	if !js.Global().Get("navigator").Truthy() {
		return false
	}
	storage := js.Global().Get("navigator").Get("storage")
	if !storage.Truthy() {
		return false
	}
	return storage.Get("getDirectory").Truthy()
}

// FileHandle wraps a browser FileSystemFileHandle for random-access reads.
// It is safe for concurrent use; reads are serialised via a mutex because
// the underlying File.slice() returns a fresh Promise each call.
type FileHandle struct {
	mu     sync.Mutex
	handle js.Value
	file   js.Value
	size   int64
}

// NewFileHandle constructs a handle from a browser FileSystemFileHandle.
// The actual File is fetched lazily on the first ReadSector call so that
// construction never blocks.
func NewFileHandle(handle js.Value) (*FileHandle, error) {
	if !handle.Truthy() {
		return nil, errors.New("streaming: handle is nil")
	}
	return &FileHandle{handle: handle}, nil
}

// ensureFile fetches the underlying File object the first time it is needed.
func (f *FileHandle) ensureFile() error {
	if f.file.Truthy() {
		return nil
	}
	promise := f.handle.Call("getFile")
	val, err := awaitPromise(promise)
	if err != nil {
		return err
	}
	f.file = val
	f.size = int64(val.Get("size").Float())
	return nil
}

// Size returns the file's total size in bytes.
func (f *FileHandle) Size() (int64, error) {
	if err := f.ensureFile(); err != nil {
		return 0, err
	}
	return f.size, nil
}

// ReadAt reads len(out) bytes at the given absolute offset. Blocks until
// the slice resolves or fails.
func (f *FileHandle) ReadAt(out []byte, off int64) error {
	if err := f.ensureFile(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if off < 0 || off+int64(len(out)) > f.size {
		return errors.New("streaming: read out of range")
	}
	blob := f.file.Call("slice", off, off+int64(len(out)))
	promise := blob.Call("arrayBuffer")
	buf, err := awaitPromise(promise)
	if err != nil {
		return err
	}
	arr := js.Global().Get("Uint8Array").New(buf)
	if arr.Length() != len(out) {
		return errors.New("streaming: short read")
	}
	js.CopyBytesToGo(out, arr)
	return nil
}

// SectorReader implements iso9660.SectorReader over a FileHandle plus a
// parsed CUE sheet. It pulls exactly one sector's worth of bytes per call.
type SectorReader struct {
	handle     *FileHandle
	sectorSize int64
	dataStart  int64 // sector offset of the data track
	total      int64 // total data sectors available
}

// NewSectorReader returns a reader for the given file handle + cue sheet.
func NewSectorReader(handle *FileHandle, sheet *cue.Sheet) (*SectorReader, error) {
	if sheet == nil {
		return nil, errors.New("streaming: sheet is required")
	}
	dt := sheet.DataTrack()
	if dt == nil {
		return nil, errors.New("streaming: cue sheet has no MODE1/MODE2 data track")
	}
	size, err := handle.Size()
	if err != nil {
		return nil, err
	}
	sectorSize := int64(2352)
	if size%2352 != 0 && size%2048 == 0 {
		sectorSize = 2048
	}
	dataStart := dt.Indices[0].Start
	return &SectorReader{
		handle:     handle,
		sectorSize: sectorSize,
		dataStart:  dataStart,
		total:      size/sectorSize - dataStart,
	}, nil
}

// ReadSector implements iso9660.SectorReader. n is relative to the data
// track, not the file.
func (r *SectorReader) ReadSector(n int64, out []byte) error {
	if n < 0 || n >= r.total {
		return errors.New("streaming: sector out of range")
	}
	if len(out) < iso9660.SectorSize {
		return errors.New("streaming: out buffer too small")
	}
	absSector := r.dataStart + n
	if r.sectorSize == 2048 {
		return r.handle.ReadAt(out[:iso9660.SectorSize], absSector*2048)
	}
	// Raw 2352-byte sector: skip 12-byte sync + 4-byte header.
	return r.handle.ReadAt(out[:iso9660.SectorSize], absSector*2352+16)
}

// SectorCount implements iso9660.SectorReader.
func (r *SectorReader) SectorCount() int64 { return r.total }

// awaitPromise resolves a JS Promise synchronously from Go. Returns the
// resolved value or an error wrapping the rejection reason.
func awaitPromise(p js.Value) (js.Value, error) {
	type result struct {
		v   js.Value
		err error
	}
	ch := make(chan result, 1)
	onResolve := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			ch <- result{v: args[0]}
		} else {
			ch <- result{}
		}
		return nil
	})
	onReject := js.FuncOf(func(this js.Value, args []js.Value) any {
		var msg string
		if len(args) > 0 {
			msg = args[0].String()
		}
		ch <- result{err: errors.New("streaming: " + msg)}
		return nil
	})
	defer onResolve.Release()
	defer onReject.Release()
	p.Call("then", onResolve, onReject)
	r := <-ch
	return r.v, r.err
}
