// Package discfs glues together the cue, iso9660, and memory packages into
// a single engine.FileSystem implementation that mounts a BIN/CUE pair and
// exposes its files by path.
//
// The package is the public entry point for Phase 2: cmd/re2-wasm and the
// wasm runtime use it to turn the user's dropped disc image into a file
// tree the engine can read from.
package discfs

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"github.com/aarvsn/re2-wasm/filesystem/cue"
	"github.com/aarvsn/re2-wasm/filesystem/iso9660"
)

// DiscFS is an engine.FileSystem backed by one or more mounted BIN/CUE
// pairs plus a loose-file store for users who drop extracted TIM/TMD files
// directly.
type DiscFS struct {
	mu sync.RWMutex

	volumes   []*mountedVolume
	extracted map[string][]byte

	// pendingCUE / pendingBIN hold the most recent dropped .cue / .bin
	// payload so that Mount can pair them once both halves have arrived.
	pendingCUE []byte
	pendingBIN []byte
}

type mountedVolume struct {
	prefix string // lower-cased, no leading/trailing slash
	sheet  *cue.Sheet
	reader iso9660.SectorReader
	vol    *iso9660.Volume
}

// New returns an empty DiscFS.
func New() *DiscFS {
	return &DiscFS{
		extracted: make(map[string][]byte),
	}
}

// MountBINCUE parses the CUE sheet (passed as raw bytes), wraps the BIN
// (also passed as raw bytes) in a RawBINReader, opens the ISO 9660 volume,
// and registers the result under prefix. prefix can be empty for the root
// volume; non-empty prefixes are useful when multiple discs are mounted.
func (d *DiscFS) MountBINCUE(prefix string, cueBytes, binBytes []byte) error {
	sheet, err := cue.Parse(bytes.NewReader(cueBytes))
	if err != nil {
		return fmt.Errorf("discfs: parse cue: %w", err)
	}
	reader, err := iso9660.NewRawBINReader(binBytes, sheet)
	if err != nil {
		return fmt.Errorf("discfs: open bin: %w", err)
	}
	vol, err := iso9660.Open(reader)
	if err != nil {
		return fmt.Errorf("discfs: open iso9660: %w", err)
	}
	d.mu.Lock()
	d.volumes = append(d.volumes, &mountedVolume{
		prefix: normalisePrefix(prefix),
		sheet:  sheet,
		reader: reader,
		vol:    vol,
	})
	d.mu.Unlock()
	return nil
}

// MountExtractedFile registers a single extracted file under its name. This
// is for users who drop loose TIM/TMD files instead of a disc image.
func (d *DiscFS) MountExtractedFile(name string, data []byte) error {
	if name == "" {
		return fmt.Errorf("discfs: name is required")
	}
	if data == nil {
		return fmt.Errorf("discfs: data is nil")
	}
	stored := make([]byte, len(data))
	copy(stored, data)
	d.mu.Lock()
	d.extracted[normalisePath(name)] = stored
	d.mu.Unlock()
	return nil
}

// Has implements engine.FileSystem.
func (d *DiscFS) Has(path string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	p := normalisePath(path)
	if _, ok := d.extracted[p]; ok {
		return true
	}
	for _, v := range d.volumes {
		rel := strings.TrimPrefix(p, v.prefix)
		rel = strings.TrimPrefix(rel, "/")
		if rel == p && v.prefix != "" {
			continue
		}
		if _, err := v.vol.FindPath(v.reader, rel); err == nil {
			return true
		}
	}
	return false
}

// Read implements engine.FileSystem.
func (d *DiscFS) Read(path string) ([]byte, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	p := normalisePath(path)
	if b, ok := d.extracted[p]; ok {
		out := make([]byte, len(b))
		copy(out, b)
		return out, nil
	}
	for _, v := range d.volumes {
		rel := strings.TrimPrefix(p, v.prefix)
		rel = strings.TrimPrefix(rel, "/")
		if rel == p && v.prefix != "" {
			continue
		}
		entry, err := v.vol.FindPath(v.reader, rel)
		if err != nil {
			continue
		}
		if entry.IsDir() {
			return nil, fmt.Errorf("discfs: %q is a directory", path)
		}
		b, err := iso9660.ReadFile(v.reader, entry)
		if err != nil {
			return nil, fmt.Errorf("discfs: read %q: %w", path, err)
		}
		return b, nil
	}
	return nil, fmt.Errorf("discfs: %q not found", path)
}

// Mount implements engine.FileSystem. The payload is dispatched by file
// extension: .cue and .bin are buffered until both halves are present and
// then mounted as a pair; everything else is treated as an extracted file.
func (d *DiscFS) Mount(name string, payload []byte) error {
	low := strings.ToLower(name)
	switch {
	case strings.HasSuffix(low, ".cue"):
		d.mu.Lock()
		d.pendingCUE = payload
		d.mu.Unlock()
		return d.tryPairPending()
	case strings.HasSuffix(low, ".bin") || strings.HasSuffix(low, ".img"):
		d.mu.Lock()
		d.pendingBIN = payload
		d.mu.Unlock()
		return d.tryPairPending()
	default:
		return d.MountExtractedFile(name, payload)
	}
}

// tryPairPending mounts the BIN/CUE pair if both halves are present.
func (d *DiscFS) tryPairPending() error {
	d.mu.Lock()
	cueBytes, binBytes := d.pendingCUE, d.pendingBIN
	d.mu.Unlock()
	if cueBytes == nil || binBytes == nil {
		return nil
	}
	if err := d.MountBINCUE("", cueBytes, binBytes); err != nil {
		return err
	}
	d.mu.Lock()
	d.pendingCUE = nil
	d.pendingBIN = nil
	d.mu.Unlock()
	return nil
}

// List returns every file path the filesystem can serve, across all
// volumes. Walks each volume's directory tree lazily.
func (d *DiscFS) List() ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var out []string
	for p := range d.extracted {
		out = append(out, p)
	}
	for _, v := range d.volumes {
		walked, err := walkVolume(v)
		if err != nil {
			return nil, fmt.Errorf("discfs: list %q: %w", v.prefix, err)
		}
		out = append(out, walked...)
	}
	return out, nil
}

// walkVolume recursively lists every file under vol.
func walkVolume(v *mountedVolume) ([]string, error) {
	var out []string
	var rec func(prefix string, e *iso9660.Entry) error
	rec = func(prefix string, e *iso9660.Entry) error {
		full := prefix + "/" + e.BaseName()
		if !e.IsDir() {
			out = append(out, strings.TrimPrefix(full, "/"))
			return nil
		}
		if len(e.Children) == 0 {
			children, err := iso9660.ReadDirectory(v.reader, e)
			if err != nil {
				return err
			}
			e.Children = children
		}
		for i := range e.Children {
			if err := rec(full, &e.Children[i]); err != nil {
				return err
			}
		}
		return nil
	}
	root := v.vol.RootDir
	for i := range root.Children {
		if err := rec(v.prefix, &root.Children[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// normalisePath lower-cases path and converts backslashes.
func normalisePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "/")
	return strings.ToLower(p)
}

// normalisePrefix lower-cases prefix and strips surrounding slashes.
func normalisePrefix(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.Trim(p, "/")
	return strings.ToLower(p)
}

// Compile-time assertion that DiscFS satisfies the minimal filesystem
// interface the engine expects.
var _ minimalFS = (*DiscFS)(nil)

type minimalFS interface {
	Has(path string) bool
	Read(path string) ([]byte, error)
	Mount(name string, payload []byte) error
}
