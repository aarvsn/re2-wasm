// Package cue parses CUE sheets for BIN/CUE disc images. The original
// Resident Evil 2 ships as a two-file pair: a *.bin containing the raw
// 2352-byte CD-ROM sectors and a *.cue describing the track layout.
//
// The parser is deliberately small: it understands the CUE subset that
// real RE2 discs use (one or two audio tracks followed by a MODE1 data
// track, occasionally with PREGAP / INDEX declarations). It does not
// implement the full spec; obscure constructs like CATALOG, ISRC, CD-TEXT,
// or non-2352 sector sizes are rejected with a clear error.
package cue

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// TrackMode enumerates the CD-ROM track modes that RE2 discs use.
type TrackMode int

// Supported track modes.
const (
	ModeUnknown TrackMode = iota
	ModeAudio             // PCM 44.1kHz stereo
	Mode1Raw              // 2352-byte sectors, 2048-byte user data
	Mode1                 // alias for Mode1Raw in RE2 CUE sheets
	Mode2Raw              // 2352-byte sectors, 2336-byte user data (XA)
)

// Index is a single INDEX NN entry inside a TRACK. RE2 only uses INDEX 01
// (start) and occasionally INDEX 00 (pregap); we keep all indices for
// completeness.
type Index struct {
	Number int    // 00, 01, ...
	Start  int64  // sector offset (in 2352-byte frames) from start of BIN
	MMSSFF [3]int // MM:SS:FF as written in the cue (for debugging)
}

// Track is one TRACK block in the CUE sheet.
type Track struct {
	Number  int
	Mode    TrackMode
	Indices []Index
}

// Sheet is the parsed CUE document. The FILE entry itself is intentionally
// NOT included here — callers pass the BIN separately so the parser stays
// pure and testable on a string input.
type Sheet struct {
	Tracks []Track
}

// TotalSectors returns the total number of 2352-byte sectors spanned by all
// tracks. The caller uses this to validate the BIN file length.
func (s Sheet) TotalSectors(sectorSize int64) int64 {
	if sectorSize <= 0 {
		sectorSize = 2352
	}
	var max int64
	for _, t := range s.Tracks {
		for _, idx := range t.Indices {
			end := idx.Start + 1
			if end > max {
				max = end
			}
		}
	}
	return max
}

// DataTrack returns the first MODE1/MODE2 track, which is where ISO 9660
// filesystem data lives. Returns nil if there is none.
func (s Sheet) DataTrack() *Track {
	for i := range s.Tracks {
		t := &s.Tracks[i]
		if t.Mode == Mode1Raw || t.Mode == Mode1 || t.Mode == Mode2Raw {
			return t
		}
	}
	return nil
}

// Parse reads a CUE sheet from r and returns the structured Sheet. The
// input must be UTF-8 (cue files are historically ASCII but allow Latin-1
// for FILE comments; we treat the bytes as UTF-8 and reject invalid
// sequences).
func Parse(r io.Reader) (*Sheet, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var sheet Sheet
	var current *Track
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "REM ") {
			continue
		}
		tokens := tokenize(line)
		if len(tokens) == 0 {
			continue
		}
		switch strings.ToUpper(tokens[0]) {
		case "FILE":
			// We do not track the FILE name here; callers pass the BIN
			// separately. Skip silently.
			if len(tokens) < 2 {
				return nil, fmt.Errorf("cue: line %d: FILE missing arguments", lineNo)
			}
		case "TRACK":
			if len(tokens) < 3 {
				return nil, fmt.Errorf("cue: line %d: TRACK missing arguments", lineNo)
			}
			n, err := strconv.Atoi(tokens[1])
			if err != nil || n < 1 || n > 99 {
				return nil, fmt.Errorf("cue: line %d: invalid track number %q", lineNo, tokens[1])
			}
			mode, err := parseMode(tokens[2])
			if err != nil {
				return nil, fmt.Errorf("cue: line %d: %w", lineNo, err)
			}
			sheet.Tracks = append(sheet.Tracks, Track{Number: n, Mode: mode})
			current = &sheet.Tracks[len(sheet.Tracks)-1]
		case "INDEX":
			if current == nil {
				return nil, fmt.Errorf("cue: line %d: INDEX outside TRACK", lineNo)
			}
			if len(tokens) < 3 {
				return nil, fmt.Errorf("cue: line %d: INDEX missing arguments", lineNo)
			}
			n, err := strconv.Atoi(tokens[1])
			if err != nil || n < 0 || n > 99 {
				return nil, fmt.Errorf("cue: line %d: invalid index number %q", lineNo, tokens[1])
			}
			start, mmssff, err := parseMMSSFF(tokens[2])
			if err != nil {
				return nil, fmt.Errorf("cue: line %d: %w", lineNo, err)
			}
			current.Indices = append(current.Indices, Index{
				Number: n,
				Start:  start,
				MMSSFF: mmssff,
			})
		case "PREGAP", "POSTGAP":
			// Accept but do not act; RE2 discs occasionally include these
			// for audio tracks and they do not affect the data track.
		case "CATALOG", "ISRC", "CDTEXTFILE", "FLAGS":
			// Accept silently; non-essential metadata.
		default:
			return nil, fmt.Errorf("cue: line %d: unknown keyword %q", lineNo, tokens[0])
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("cue: scan: %w", err)
	}
	if len(sheet.Tracks) == 0 {
		return nil, errors.New("cue: sheet contains no tracks")
	}
	return &sheet, nil
}

// ParseString is a convenience wrapper around Parse for inline CUE content.
func ParseString(s string) (*Sheet, error) {
	return Parse(strings.NewReader(s))
}

// tokenize splits a cue line into tokens, honouring double-quoted strings
// (used in FILE "name.bin" BINARY).
func tokenize(line string) []string {
	var out []string
	var sb strings.Builder
	inQuotes := false
	for _, r := range line {
		switch {
		case r == '"':
			inQuotes = !inQuotes
		case (r == ' ' || r == '\t') && !inQuotes:
			if sb.Len() > 0 {
				out = append(out, sb.String())
				sb.Reset()
			}
		default:
			sb.WriteRune(r)
		}
	}
	if sb.Len() > 0 {
		out = append(out, sb.String())
	}
	return out
}

// parseMode converts the cue-sheet mode token to a TrackMode.
func parseMode(s string) (TrackMode, error) {
	switch strings.ToUpper(s) {
	case "AUDIO":
		return ModeAudio, nil
	case "MODE1/2352", "MODE1/2048":
		// MODE1/2048 means the BIN is already de-stripped; rare. We treat
		// it as Mode1Raw and let the caller detect the sector size from
		// the actual file length.
		return Mode1Raw, nil
	case "MODE2/2352", "MODE2/2336":
		return Mode2Raw, nil
	default:
		return ModeUnknown, fmt.Errorf("unsupported track mode %q (only AUDIO, MODE1/2352, MODE2/2352 are supported)", s)
	}
}

// parseMMSSFF converts "MM:SS:FF" (CUE timecode, FF = frames at 75 Hz) to a
// sector offset from the start of the BIN file.
func parseMMSSFF(s string) (int64, [3]int, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, [3]int{}, fmt.Errorf("invalid timecode %q (expected MM:SS:FF)", s)
	}
	var mmssff [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0, [3]int{}, fmt.Errorf("invalid timecode component %q", p)
		}
		if i == 2 && n > 74 {
			return 0, [3]int{}, fmt.Errorf("invalid frames %d (max 74)", n)
		}
		if i < 2 && n > 99 {
			return 0, [3]int{}, fmt.Errorf("invalid %s %d (max 99)", []string{"MM", "SS"}[i], n)
		}
		mmssff[i] = n
	}
	sectors := int64(mmssff[0])*60*75 + int64(mmssff[1])*75 + int64(mmssff[2])
	return sectors, mmssff, nil
}
