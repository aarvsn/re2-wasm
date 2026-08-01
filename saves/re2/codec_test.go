package re2

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeDecode_RoundTrip(t *testing.T) {
	cases := []Save{
		{
			Slot: 0, Character: Leon, Scenario: ScenarioA,
			Health: 1000, RoomID: 100,
			PositionX: 12.5, PositionY: -7.25,
			PlayTime: 3600, Payload: []byte{0xde, 0xad, 0xbe, 0xef},
		},
		{
			Slot: 19, Character: Claire, Scenario: ScenarioB,
			Health: 750, RoomID: 50,
			PositionX: -100.0, PositionY: 200.0,
			PlayTime: 7200, Payload: make([]byte, 0),
		},
		{
			Slot: 5, Character: Claire, Scenario: ScenarioA,
			Health: 1, RoomID: 0,
			PositionX: 0, PositionY: 0,
			PlayTime: 0, Payload: make([]byte, 256),
		},
	}
	for i, c := range cases {
		enc, err := Encode(&c)
		if err != nil {
			t.Fatalf("case %d: Encode: %v", i, err)
		}
		dec, err := Decode(enc)
		if err != nil {
			t.Fatalf("case %d: Decode: %v", i, err)
		}
		if dec.Slot != c.Slot {
			t.Errorf("case %d: Slot = %d, want %d", i, dec.Slot, c.Slot)
		}
		if dec.Character != c.Character {
			t.Errorf("case %d: Character = %v, want %v", i, dec.Character, c.Character)
		}
		if dec.Scenario != c.Scenario {
			t.Errorf("case %d: Scenario = %v, want %v", i, dec.Scenario, c.Scenario)
		}
		if dec.Health != c.Health {
			t.Errorf("case %d: Health = %d, want %d", i, dec.Health, c.Health)
		}
		if dec.RoomID != c.RoomID {
			t.Errorf("case %d: RoomID = %d, want %d", i, dec.RoomID, c.RoomID)
		}
		if dec.PositionX != c.PositionX {
			t.Errorf("case %d: PositionX = %v, want %v", i, dec.PositionX, c.PositionX)
		}
		if dec.PositionY != c.PositionY {
			t.Errorf("case %d: PositionY = %v, want %v", i, dec.PositionY, c.PositionY)
		}
		if dec.PlayTime != c.PlayTime {
			t.Errorf("case %d: PlayTime = %d, want %d", i, dec.PlayTime, c.PlayTime)
		}
		if !bytes.Equal(dec.Payload, c.Payload) {
			t.Errorf("case %d: Payload = %v, want %v", i, dec.Payload, c.Payload)
		}
	}
}

func TestEncode_RejectsBadSlot(t *testing.T) {
	cases := []int{-1, 20, 100}
	for _, slot := range cases {
		s := &Save{Slot: slot}
		_, err := Encode(s)
		if err == nil {
			t.Errorf("Encode(slot=%d) err=nil, want error", slot)
		}
	}
}

func TestDecode_RejectsShort(t *testing.T) {
	_, err := Decode(make([]byte, HeaderSize-1))
	if err == nil {
		t.Fatal("err = nil, want short-input error")
	}
}

func TestDecode_RejectsBadCharacter(t *testing.T) {
	s := &Save{Slot: 0}
	enc, _ := Encode(s)
	enc[1] = 99 // bad character
	_, err := Decode(enc)
	if err == nil {
		t.Fatal("err = nil, want bad-character error")
	}
}

func TestDecode_RejectsBadScenario(t *testing.T) {
	s := &Save{Slot: 0}
	enc, _ := Encode(s)
	enc[2] = 99 // bad scenario
	_, err := Decode(enc)
	if err == nil {
		t.Fatal("err = nil, want bad-scenario error")
	}
}

func TestDecode_RejectsBadChecksum(t *testing.T) {
	s := &Save{Slot: 0, Health: 500}
	enc, _ := Encode(s)
	// Corrupt one header byte after checksum was computed.
	enc[4] ^= 0xFF
	_, err := Decode(enc)
	if err == nil {
		t.Fatal("err = nil, want checksum mismatch")
	}
}

func TestEncode_PayloadLengthField(t *testing.T) {
	s := &Save{Slot: 0, Payload: make([]byte, 100)}
	enc, _ := Encode(s)
	got := binary.LittleEndian.Uint16(enc[22:24])
	if got != 100 {
		t.Errorf("payload length field = %d, want 100", got)
	}
}

func TestChecksum16(t *testing.T) {
	cases := []struct {
		in   []byte
		want uint16
	}{
		{[]byte{0}, 0},
		{[]byte{1, 2, 3}, 6},
		{[]byte{0xFF, 0xFF}, 0x1FE},
		{[]byte{0xFF, 0xFF, 0xFF, 0xFF}, 0x3FC},
	}
	for _, c := range cases {
		if got := checksum16(c.in); got != c.want {
			t.Errorf("checksum16(%v) = 0x%x, want 0x%x", c.in, got, c.want)
		}
	}
}
