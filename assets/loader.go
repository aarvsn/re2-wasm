// Package assets is the asset loading pipeline. Phase 2 ships a trivial
// reader that pulls bytes from any engine.FileSystem-compatible source;
// Phase 3 will add TIM/TMD/ADT decoding and streaming.
//
// The package implements engine.AssetSource so the engine can request named
// assets without knowing where they came from.
package assets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
)

// FileSource is the minimal read interface the loader needs. Both
// filesystem.MemoryFS and filesystem/discfs.DiscFS satisfy it.
type FileSource interface {
	Has(path string) bool
	Read(path string) ([]byte, error)
}

// Loader is the engine.AssetSource implementation. It owns a reference to a
// FileSource and reads synchronously; Phase 3 will add a worker pool.
type Loader struct {
	fs FileSource
}

// New returns a Loader backed by fs. fs may be nil; in that case Open
// returns an error. This makes the constructor safe to call before the
// user has mounted any files.
func New(fs FileSource) *Loader {
	return &Loader{fs: fs}
}

// SetSource swaps the underlying FileSource. Used by the WASM runtime to
// inject the discfs.DiscFS after the user drops files.
func (l *Loader) SetSource(fs FileSource) {
	l.fs = fs
}

// Open implements engine.AssetSource.Open. The returned io.ReadCloser is a
// bytes.Reader wrapper so callers can stream without loading the whole
// asset into a heap-allocated slice at once.
func (l *Loader) Open(_ context.Context, name string) (io.ReadCloser, error) {
	if l.fs == nil {
		return nil, errors.New("assets: no file source mounted")
	}
	if name == "" {
		return nil, errors.New("assets: name is required")
	}
	b, err := l.fs.Read(name)
	if err != nil {
		return nil, fmt.Errorf("assets: open %q: %w", name, err)
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
