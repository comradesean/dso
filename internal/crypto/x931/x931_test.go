package x931

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"testing"
)

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	// A fixed-size 2048-bit key; generation is deterministic enough for tests.
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestPadFormat(t *testing.T) {
	msg := []byte{0x01, 0x02, 0x03}
	em, err := pad(msg, 256)
	if err != nil {
		t.Fatal(err)
	}
	if len(em) != 256 {
		t.Fatalf("padded length = %d, want 256", len(em))
	}
	if em[0] != 0x6B {
		t.Errorf("leading byte = %#x, want 0x6B", em[0])
	}
	if em[len(em)-1] != 0xCC {
		t.Errorf("trailer = %#x, want 0xCC", em[len(em)-1])
	}
	// The byte immediately before the message must be the 0xBA separator.
	sep := len(em) - 1 - len(msg) - 1
	if em[sep] != 0xBA {
		t.Errorf("separator = %#x, want 0xBA", em[sep])
	}
	if !bytes.Equal(em[sep+1:len(em)-1], msg) {
		t.Errorf("embedded message mismatch")
	}
	got, err := unpad(em)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, msg) {
		t.Errorf("unpad = %x, want %x", got, msg)
	}
}

func TestPrivateEncryptPublicDecryptRoundTrip(t *testing.T) {
	key := testKey(t)
	for _, n := range []int{0, 1, 16, 100, 254} {
		msg := make([]byte, n)
		for i := range msg {
			msg[i] = byte(i + 1)
		}
		sig, err := PrivateEncrypt(key, msg)
		if err != nil {
			t.Fatalf("PrivateEncrypt(len %d): %v", n, err)
		}
		if len(sig) != key.Size() {
			t.Fatalf("sig length = %d, want %d", len(sig), key.Size())
		}
		got, err := PublicDecrypt(&key.PublicKey, sig)
		if err != nil {
			t.Fatalf("PublicDecrypt(len %d): %v", n, err)
		}
		if !bytes.Equal(got, msg) {
			t.Fatalf("round-trip len %d: got %x want %x", n, got, msg)
		}
	}
}

func TestPrivateEncryptRejectsOversized(t *testing.T) {
	key := testKey(t)
	if _, err := PrivateEncrypt(key, make([]byte, 255)); err == nil {
		t.Fatal("expected error for oversized message")
	}
}

// TestFullCipherPair models the actual Frpg2 login/auth exchange: the client
// sends OAEP (public-encrypt) and the server decrypts with the private key;
// the server replies with X9.31 (private-encrypt) and the client recovers it
// with the public key.
func TestFullCipherPair(t *testing.T) {
	key := testKey(t)
	pub := &key.PublicKey

	// client -> server (OAEP)
	request := []byte("RequestQueryLoginServerInfo payload")
	ct, err := rsa.EncryptOAEP(sha1.New(), rand.Reader, pub, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := rsa.DecryptOAEP(sha1.New(), rand.Reader, key, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recovered, request) {
		t.Fatalf("OAEP recovered %q, want %q", recovered, request)
	}

	// server -> client (X9.31)
	response := []byte("RequestQueryLoginServerInfoResponse: auth 10.0.0.5:50000")
	sig, err := PrivateEncrypt(key, response)
	if err != nil {
		t.Fatal(err)
	}
	clientGot, err := PublicDecrypt(pub, sig)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(clientGot, response) {
		t.Fatalf("X9.31 recovered %q, want %q", clientGot, response)
	}
}
