package main

import (
	"encoding/hex"
	"testing"

	"github.com/sstreight/dso/internal/crypto/frpgcipher"
)

// TestOpenBothDirections pins the direction mapping and, more importantly, the
// success condition.
//
// Both were wrong when this tool was first written. The ciphers are named by
// ROLE, so ServerUDPCipher.Open reads client->server datagrams — mixing those up
// gives a tag failure indistinguishable from a bad key. And Open returns
// (plaintext, connectionPrefix, error): treating that bool as "ok" made a
// correct key report "tag failed" while handing back correct plaintext. Both
// mistakes would have been debugged mid-session against a live server, with a
// real key in hand, believing it was the wrong one.
func TestOpenBothDirections(t *testing.T) {
	key, err := hex.DecodeString("000102030405060708090a0b0c0d0e0f")
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0xF5, 0x02, 0x00, 0x01, 0x02, 0x03, 0x04, 0xAA, 0xBB}

	server, err := frpgcipher.NewServerUDPCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	client, err := frpgcipher.NewClientUDPCipher(key, [8]byte{1, 2, 3, 4, 5, 6, 7, 8})
	if err != nil {
		t.Fatal(err)
	}

	s2c, err := server.Seal(want, false)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := open(key, s2c, "s2c")
	if err != nil {
		t.Fatalf("s2c with the correct key must verify: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("s2c plaintext = %x, want %x", got, want)
	}

	c2s, err := client.Seal(want, false)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err = open(key, c2s, "c2s")
	if err != nil {
		t.Fatalf("c2s with the correct key must verify: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("c2s plaintext = %x, want %x", got, want)
	}
}

// TestOpenRejectsWrongKey — the whole point of this tool is that a wrong key is
// distinguishable from a right one.
func TestOpenRejectsWrongKey(t *testing.T) {
	good, _ := hex.DecodeString("000102030405060708090a0b0c0d0e0f")
	bad, _ := hex.DecodeString("ffffffffffffffffffffffffffffffff")

	server, err := frpgcipher.NewServerUDPCipher(good)
	if err != nil {
		t.Fatal(err)
	}
	s2c, err := server.Seal([]byte{0xF5, 0x02, 0x00}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := open(bad, s2c, "s2c"); err == nil {
		t.Error("a wrong key verified a datagram; the tool proves nothing")
	}
}
