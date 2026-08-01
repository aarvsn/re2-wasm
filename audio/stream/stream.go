// Package stream provides chunked XA-ADPCM decoding for BGM playback.
// Instead of loading an entire cutscene's worth of XA audio into memory,
// the engine feeds sector-sized chunks to the Streamer, which decodes
// them into a rolling AudioBuffer ring.
//
// The host-testable math lives in audio/xa; this package orchestrates the
// ring buffer and the chunk-pump state machine.
package stream

import (
	"errors"
	"sync"
)

// ChunkSize is the number of samples each chunk decodes to. At 44.1 kHz
// stereo this is ~25 ms of audio, small enough to keep latency low.
const ChunkSize = 2304

// RingBuffer is a lock-free-ish ring of decoded sample chunks. One
// producer (the decoder goroutine) appends; one consumer (the audio
// playback thread) drains.
type RingBuffer struct {
	mu       sync.Mutex
	chunks   [][]float32
	capacity int
	closed   bool
}

// NewRingBuffer returns a ring with the given chunk capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity < 2 {
		capacity = 2
	}
	return &RingBuffer{capacity: capacity}
}

// Push appends a chunk. Returns false if the ring is full or closed; the
// producer should retry on the next pump tick.
func (r *RingBuffer) Push(chunk []float32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return false
	}
	if len(r.chunks) >= r.capacity {
		return false
	}
	r.chunks = append(r.chunks, chunk)
	return true
}

// Pop removes and returns the oldest chunk. Returns nil if empty.
func (r *RingBuffer) Pop() []float32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.chunks) == 0 {
		return nil
	}
	c := r.chunks[0]
	r.chunks = r.chunks[1:]
	return c
}

// Len returns the number of buffered chunks.
func (r *RingBuffer) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.chunks)
}

// Close marks the ring as done. Future Push calls return false.
func (r *RingBuffer) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
}

// Closed reports whether Close has been called.
func (r *RingBuffer) Closed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

// Streamer drives a decoder callback over chunk-sized inputs and pushes
// the results into a RingBuffer. It is the bridge between the asset
// pipeline (which produces []byte chunks) and the audio layer (which
// consumes []float32 chunks).
type Streamer struct {
	ring   *RingBuffer
	decode func([]byte) ([]float32, error)
}

// New returns a Streamer that decodes via fn into a ring of the given
// capacity.
func New(ringCapacity int, decode func([]byte) ([]float32, error)) *Streamer {
	return &Streamer{
		ring:   NewRingBuffer(ringCapacity),
		decode: decode,
	}
}

// Ring returns the streamer's ring buffer so the audio layer can drain it.
func (s *Streamer) Ring() *RingBuffer { return s.ring }

// Pump decodes one chunk and pushes it into the ring. Returns false when
// the ring is full (caller should back off) or when decode returns an
// error (caller should stop the stream).
func (s *Streamer) Pump(input []byte) (bool, error) {
	if s.ring.Closed() {
		return false, nil
	}
	samples, err := s.decode(input)
	if err != nil {
		return false, err
	}
	if !s.ring.Push(samples) {
		return false, nil
	}
	return true, nil
}

// Finish marks the stream as complete so the audio layer knows there will
// be no more chunks.
func (s *Streamer) Finish() { s.ring.Close() }

// ErrNoMoreInput is returned by Pump when the caller has no more bytes to
// feed. It is not a fatal error; the caller should call Finish instead.
var ErrNoMoreInput = errors.New("stream: no more input")
