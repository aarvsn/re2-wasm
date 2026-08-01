package cue

import (
	"strings"
	"testing"
)

func TestParse_MinimalDataTrack(t *testing.T) {
	src := `FILE "re2.bin" BINARY
  TRACK 01 MODE1/2352
    INDEX 01 00:00:00
`
	sheet, err := ParseString(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheet.Tracks) != 1 {
		t.Fatalf("tracks = %d, want 1", len(sheet.Tracks))
	}
	dt := sheet.DataTrack()
	if dt == nil {
		t.Fatal("DataTrack returned nil")
	}
	if dt.Mode != Mode1Raw {
		t.Errorf("mode = %v, want Mode1Raw", dt.Mode)
	}
	if len(dt.Indices) != 1 || dt.Indices[0].Number != 1 || dt.Indices[0].Start != 0 {
		t.Errorf("indices = %+v, want [{1 0 ...}]", dt.Indices)
	}
}

func TestParse_AudioThenData(t *testing.T) {
	// Typical RE2 disc: audio track first, then MODE1 data.
	src := `FILE "re2.bin" BINARY
  TRACK 01 AUDIO
    INDEX 01 00:00:00
  TRACK 02 MODE1/2352
    INDEX 01 00:30:00
`
	sheet, err := ParseString(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheet.Tracks) != 2 {
		t.Fatalf("tracks = %d, want 2", len(sheet.Tracks))
	}
	dt := sheet.DataTrack()
	if dt == nil {
		t.Fatal("DataTrack nil")
	}
	if dt.Number != 2 {
		t.Errorf("data track number = %d, want 2", dt.Number)
	}
	// 00:30:00 = 30s * 75 = 2250 sectors
	if dt.Indices[0].Start != 2250 {
		t.Errorf("data track start = %d, want 2250", dt.Indices[0].Start)
	}
}

func TestParseMMSSFF_Table(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantTS  [3]int
		wantErr bool
	}{
		{"00:00:00", 0, [3]int{0, 0, 0}, false},
		{"00:01:00", 75, [3]int{0, 1, 0}, false},
		{"00:00:74", 74, [3]int{0, 0, 74}, false},
		{"01:00:00", 4500, [3]int{1, 0, 0}, false},
		{"02:30:37", 2*60*75 + 30*75 + 37, [3]int{2, 30, 37}, false},
		{"00:00:75", 0, [3]int{}, true},                // FF > 74
		{"00:99:00", 99 * 75, [3]int{0, 99, 0}, false}, // SS = 99 (loose upper bound)
		{"00:100:00", 0, [3]int{}, true},               // SS > 99
		{"bad", 0, [3]int{}, true},
		{"00:00", 0, [3]int{}, true},
	}
	for _, c := range cases {
		got, mmssff, err := parseMMSSFF(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseMMSSFF(%q) err=nil, want error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseMMSSFF(%q) err=%v, want nil", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseMMSSFF(%q) sectors=%d, want %d", c.in, got, c.want)
		}
		if mmssff != c.wantTS {
			t.Errorf("parseMMSSFF(%q) mmssff=%v, want %v", c.in, mmssff, c.wantTS)
		}
	}
}

func TestParse_RejectsUnknownKeyword(t *testing.T) {
	src := `FILE "x.bin" BINARY
  FROBNICATE 01
`
	_, err := ParseString(src)
	if err == nil || !strings.Contains(err.Error(), "unknown keyword") {
		t.Fatalf("err = %v, want unknown keyword error", err)
	}
}

func TestParse_RejectsIndexOutsideTrack(t *testing.T) {
	src := `FILE "x.bin" BINARY
  INDEX 01 00:00:00
`
	_, err := ParseString(src)
	if err == nil || !strings.Contains(err.Error(), "INDEX outside TRACK") {
		t.Fatalf("err = %v, want INDEX outside TRACK", err)
	}
}

func TestParse_EmptySheet(t *testing.T) {
	_, err := ParseString("")
	if err == nil {
		t.Fatal("err = nil, want error for empty sheet")
	}
	if !strings.Contains(err.Error(), "no tracks") {
		t.Fatalf("err = %v, want a 'no tracks' error", err)
	}
}

func TestParse_TokenizesQuotedFileName(t *testing.T) {
	// File name with spaces inside quotes should be one token.
	toks := tokenize(`FILE "my game.bin" BINARY`)
	want := []string{"FILE", "my game.bin", "BINARY"}
	if len(toks) != len(want) {
		t.Fatalf("tokens = %v, want %v", toks, want)
	}
	for i := range want {
		if toks[i] != want[i] {
			t.Errorf("token[%d] = %q, want %q", i, toks[i], want[i])
		}
	}
}

func TestSheet_TotalSectors(t *testing.T) {
	sheet := &Sheet{Tracks: []Track{
		{Number: 1, Mode: ModeAudio, Indices: []Index{{Number: 1, Start: 0}}},
		{Number: 2, Mode: Mode1Raw, Indices: []Index{{Number: 1, Start: 2250}}},
	}}
	// TotalSectors returns the max end sector; for a single INDEX per track
	// we get max(0+1, 2250+1) = 2251. This is intentionally a lower bound;
	// callers compute the real total from the BIN file length.
	if got := sheet.TotalSectors(2352); got != 2251 {
		t.Errorf("TotalSectors = %d, want 2251", got)
	}
}

func TestParseMode_Table(t *testing.T) {
	cases := []struct {
		in   string
		want TrackMode
		err  bool
	}{
		{"AUDIO", ModeAudio, false},
		{"audio", ModeAudio, false},
		{"MODE1/2352", Mode1Raw, false},
		{"MODE1/2048", Mode1Raw, false},
		{"MODE2/2352", Mode2Raw, false},
		{"MODE2/2336", Mode2Raw, false},
		{"MODE3/2352", ModeUnknown, true},
		{"", ModeUnknown, true},
	}
	for _, c := range cases {
		got, err := parseMode(c.in)
		if c.err {
			if err == nil {
				t.Errorf("parseMode(%q) err=nil, want error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseMode(%q) err=%v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseMode(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
