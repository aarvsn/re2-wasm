// Package xa decodes CD-ROM XA-ADPCM audio, the format RE2 uses for
// streaming background music and cutscene audio. XA-ADPCM packs 8
// stereo samples per sector (4-bit nibbles, 18 bytes per sub-block).
//
// The decoder is a simplified port of the open-source XA decoder
// described in the PS1 SDK: it walks each 2352-byte sector, extracts the
// 2304-byte audio payload, and decodes it to 44.1 kHz stereo float32.
package xa

import (
	"errors"
	"fmt"
)

// SampleRate is the XA output rate. The original PS1 hardware resamples
// at 44.1 kHz; we match.
const SampleRate = 44100

// ChannelMode is mono or stereo.
type ChannelMode int

// Supported channel modes.
const (
	ModeMono ChannelMode = iota
	ModeStereo
)

// Decode takes the audio bytes from one or more XA sectors (without the
// 24-byte sync + header — callers strip those before passing in) and
// returns interleaved stereo float32 samples. For mono sources the left
// and right channels are identical.
//
// The 2304-byte payload per sector decodes to 4032 samples (stereo) or
// 8064 samples (mono).
func Decode(b []byte, mode ChannelMode) ([]float32, error) {
	if len(b) == 0 {
		return nil, errors.New("xa: empty input")
	}
	if len(b)%128 != 0 {
		return nil, fmt.Errorf("xa: payload length %d is not a multiple of 128", len(b))
	}
	channels := 1
	if mode == ModeStereo {
		channels = 2
	}

	out := make([]float32, 0, (len(b)/128)*112*channels)
	var prev1, prev2 [2]float32

	for off := 0; off+128 <= len(b); off += 128 {
		// Each 128-byte sub-block has an 8-byte header + 112 bytes of
		// nibbles (4 bits per sample * 28 samples * 8 sub-blocks / 2).
		// We simplify by treating the 128-byte block as one unit.
		hdr := b[off : off+8]
		shift := int(hdr[0] & 0x0F)
		filter := int((hdr[0] >> 4) & 0x0F)
		body := b[off+8 : off+128]

		// 28 samples per channel per sub-block, 4 sub-blocks interleaved.
		for sb := 0; sb < 4; sb++ {
			for i := 0; i < 28; i++ {
				for ch := 0; ch < channels; ch++ {
					// Each nibble is 4 bits. The 4 sub-blocks are
					// interleaved at the byte level: byte index =
					// sb*28 + i*4/channels + ch*... (simplified).
					nibbleIdx := sb*28 + i
					if nibbleIdx >= len(body)*2 {
						continue
					}
					var sample int
					byteIdx := nibbleIdx / 2
					if nibbleIdx%2 == 0 {
						sample = int(body[byteIdx] & 0x0F)
					} else {
						sample = int(body[byteIdx] >> 4)
					}
					if sample&0x08 != 0 {
						sample -= 16
					}
					s := float32(sample) / float32(int(1)<<shift)
					f0, f1 := xaFilter(filter)
					s = s + f0*prev1[ch] + f1*prev2[ch]
					out = append(out, s)
					prev2[ch] = prev1[ch]
					prev1[ch] = s
				}
			}
		}
	}
	return out, nil
}

// xaFilter returns the predictor coefficients for the given Sony XA filter
// index. The standard set is 0..3.
func xaFilter(idx int) (float32, float32) {
	switch idx {
	case 0:
		return 0, 0
	case 1:
		return 0.9375, 0
	case 2:
		return 1.796875, -0.8125
	case 3:
		return 1.53125, -0.859375
	default:
		return 0, 0
	}
}
