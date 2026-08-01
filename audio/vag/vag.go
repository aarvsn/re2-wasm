// Package vag decodes PlayStation 1 VAG audio files. VAG is the SFX format
// used by RE2 for one-shot sound effects (footsteps, gunshots, doors, UI
// beeps). The format is a Sony ADPCM with 28-sample blocks, 4-bit
// nibbles, and per-block filter / range coefficients.
//
// The decoder produces 44.1 kHz mono float32 samples that the Web Audio
// API can wrap in an AudioBuffer.
package vag

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// MagicVAG is the magic at the start of a VAG file.
const MagicVAG = "VAGp"

// SampleRate is the rate VAG files are sampled at. RE2's SFX are 44.1 kHz.
const SampleRate = 44100

// Decode decodes a VAG file into mono float32 samples in the range [-1, 1].
// The returned slice can be passed directly to AudioBuffer.copyToChannel.
func Decode(b []byte) ([]float32, int, error) {
	if len(b) < 64 {
		return nil, 0, errors.New("vag: file too short")
	}
	if string(b[0:4]) != MagicVAG {
		return nil, 0, fmt.Errorf("vag: bad magic %q (want %q)", b[0:4], MagicVAG)
	}
	version := binary.BigEndian.Uint32(b[4:8])
	_ = version // version 2 and 3 both use the same block format
	size := binary.BigEndian.Uint32(b[12:16])
	sampleRate := binary.BigEndian.Uint32(b[16:20])
	if sampleRate == 0 {
		sampleRate = SampleRate
	}

	// Audio data starts at offset 48 (after the 32-byte header + 16-byte
	// second header). Some files pad extra; we just read until end.
	dataStart := 48
	if dataStart >= len(b) {
		return nil, 0, errors.New("vag: no audio data")
	}
	if size == 0 || size > uint32(len(b)-dataStart) {
		size = uint32(len(b) - dataStart)
	}
	data := b[dataStart : dataStart+int(size)]

	out := make([]float32, 0, len(data)*7)
	var (
		// ADPCM state — predictor and scaling carried across blocks.
		prev1, prev2 float32
	)
	for off := 0; off+16 <= len(data); off += 16 {
		// Block header: 1 byte shift, 1 byte filter, 2 bytes flags.
		shift := int(data[off] & 0x0F)
		filter := int((data[off] >> 4) & 0x0F)
		// data[off+1] is the filter / flag byte; we use only the lower nibble.
		// Some VAGs pack an end flag in the upper nibble; we ignore it.
		samples, p1, p2 := decodeBlock(data[off+2:off+16], shift, filter, prev1, prev2)
		out = append(out, samples...)
		prev1, prev2 = p1, p2
	}
	return out, int(sampleRate), nil
}

// decodeBlock decodes one 14-byte block (28 samples) of VAG ADPCM. The
// returned samples are float32 in [-1, 1]. prev1 / prev2 are the previous
// two decoded samples (used by the predictor); the function returns the
// new prev1 / prev2 so the caller can chain.
func decodeBlock(b []byte, shift, filter int, prev1, prev2 float32) ([]float32, float32, float32) {
	if len(b) < 14 {
		return nil, prev1, prev2
	}
	out := make([]float32, 28)
	// Each of the 14 bytes holds two 4-bit nibbles. Each nibble is a signed
	// 4-bit sample (range -8..7). We decode 28 nibbles per block.
	for i := 0; i < 28; i++ {
		nibble := b[i/2]
		var sample int
		if i%2 == 0 {
			sample = int(nibble & 0x0F)
		} else {
			sample = int(nibble >> 4)
		}
		// Sign-extend 4-bit to a signed int.
		if sample&0x08 != 0 {
			sample -= 16
		}
		// Decode using the predictor.
		s := float32(sample) / float32(int(1)<<shift)
		// Apply the filter coefficients (Sony's standard set).
		var f0, f1 float32
		switch filter {
		case 0:
			f0, f1 = 0, 0
		case 1:
			f0, f1 = 0.5, 0
		case 2:
			f0, f1 = 0.625, -0.125
		case 3:
			f0, f1 = 0.7142857, -0.1428571
		default:
			// Fall back to filter 0 for unknown filters.
			f0, f1 = 0, 0
		}
		out[i] = s + f0*prev1 + f1*prev2
		prev2 = prev1
		prev1 = out[i]
	}
	return out, prev1, prev2
}
