package frpgcipher

import (
	"bytes"
	"testing"
)

func TestUDPCipherRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef")
	token := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}

	server, err := NewServerUDPCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := NewClientUDPCipher(key, token)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name       string
		payload    []byte
		connPrefix bool
	}{
		{"empty-ish", []byte{0x01}, false},
		{"syn-with-prefix", bytes.Repeat([]byte{0xAB}, 42), true},
		{"data", bytes.Repeat([]byte{0x5A}, 300), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// client -> server
			sealed, err := cl.Seal(tc.payload, tc.connPrefix)
			if err != nil {
				t.Fatal(err)
			}
			if tok, ok := TokenFromDatagram(sealed); !ok || tok != token {
				t.Fatalf("token prefix mismatch: %x", tok)
			}
			got, prefix, err := server.Open(sealed)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tc.payload) {
				t.Fatalf("client->server payload mismatch")
			}
			if prefix != tc.connPrefix {
				t.Fatalf("connection prefix = %v, want %v", prefix, tc.connPrefix)
			}

			// server -> client
			sealed2, err := server.Seal(tc.payload, false)
			if err != nil {
				t.Fatal(err)
			}
			got2, prefix2, err := cl.Open(sealed2)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got2, tc.payload) {
				t.Fatalf("server->client payload mismatch")
			}
			if prefix2 {
				t.Fatalf("server->client should not report a connection prefix")
			}
		})
	}
}

func TestUDPCipherRejectsTamper(t *testing.T) {
	key := []byte("0123456789abcdef")
	token := [8]byte{9, 9, 9, 9, 9, 9, 9, 9}
	server, _ := NewServerUDPCipher(key)
	cl, _ := NewClientUDPCipher(key, token)

	sealed, err := cl.Seal([]byte("hello game server"), false)
	if err != nil {
		t.Fatal(err)
	}
	sealed[len(sealed)-1] ^= 0x01 // flip a ciphertext byte
	if _, _, err := server.Open(sealed); err == nil {
		t.Fatal("expected tag failure on tampered datagram")
	}
}
