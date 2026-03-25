package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"
)

func TestMarshalGzipRoundtrip(t *testing.T) {
	largeState := &ServerMessage{
		Type: "state",
		State: &RoomSnapshot{
			RoomID:   "arena",
			Width:    80,
			Height:   28,
			MapTitle: "test",
			Players: []PlayerState{
				{ID: "a", Name: "one", X: 1, Y: 2, Angle: 0.5, HP: 100, Armor: 0, PistolMag: 10, RifleMag: 30, RifleReserve: 60, Money: 100},
				{ID: "b", Name: "two", X: 3, Y: 4, Angle: 1.5, HP: 80, Armor: 20, PistolMag: 10, RifleMag: 15, RifleReserve: 40, Money: 200},
			},
			KillFeed: []KillFeedEntry{{Killer: "one", Victim: "two"}},
		},
	}
	raw, err := MarshalServerLine(largeState)
	if err != nil {
		t.Fatal(err)
	}
	// Принудительно большой JSON: повторяем поле (только для теста размера).
	for len(raw) < 200 {
		largeState.State.MapTitle += "x"
		raw, err = MarshalServerLine(largeState)
		if err != nil {
			t.Fatal(err)
		}
	}
	z := MaybeGzipServerLine(raw)
	if len(z) <= len(raw) {
		t.Fatalf("expected gzip smaller than raw: raw=%d z=%d", len(raw), len(z))
	}
	if z[0] != LineMagicGzip {
		t.Fatalf("expected magic byte, got %q", z[0])
	}
	br := bufio.NewReader(bytes.NewReader(z))
	msg, err := ReadServerMessage(br)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != "state" || msg.State == nil {
		t.Fatalf("bad msg: %+v", msg)
	}
	if msg.State.MapTitle != largeState.State.MapTitle {
		t.Fatalf("map title mismatch")
	}
}

func TestDecodePlainJSON(t *testing.T) {
	b := []byte(`{"type":"ping"}` + "\n")
	msg, err := DecodeServerLine(b)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != "ping" {
		t.Fatalf("got %q", msg.Type)
	}
}

func TestJSONMarshalMatchesEncoder(t *testing.T) {
	m := ServerMessage{Type: "ping"}
	b1, err := MarshalServerLine(&m)
	if err != nil {
		t.Fatal(err)
	}
	var aux map[string]any
	if err := json.Unmarshal(b1[:len(b1)-1], &aux); err != nil {
		t.Fatal(err)
	}
	if aux["type"] != "ping" {
		t.Fatal(aux)
	}
}
