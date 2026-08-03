package message

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"testing"

	"github.com/sstreight/dso/internal/crypto/frpgcipher"
)

// TestRSARoundTrip exercises the full login/auth framing + RSA cipher pairing:
// the client sends an OAEP-encrypted request, the server decrypts it, replies
// with an X9.31-encrypted Reply, and the client recovers it — over a real pipe.
func TestRSARoundTrip(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	sc, cc := net.Pipe()
	defer sc.Close()
	defer cc.Close()

	server := NewStream(sc)
	sEnc, sDec := frpgcipher.NewRSAServer(key)
	server.SetCiphers(sEnc, sDec)

	client := NewStream(cc)
	cEnc, cDec := frpgcipher.NewRSAClient(&key.PublicKey)
	client.SetCiphers(cEnc, cDec)

	request := []byte("steam_id=76561198000000000;app_version=17039619")
	response := []byte("auth_server=192.168.1.50:50000")

	errCh := make(chan error, 1)
	go func() {
		// Client sends a request, then waits for the reply.
		if err := client.Send(Message{Type: RequestQueryLoginServerInfo, Index: 7, Payload: request}); err != nil {
			errCh <- err
			return
		}
		reply, err := client.Recv()
		if err != nil {
			errCh <- err
			return
		}
		if reply.Type != Reply {
			t.Errorf("client got type %v, want Reply", reply.Type)
		}
		if reply.Index != 7 {
			t.Errorf("reply index = %d, want 7 (copied from request)", reply.Index)
		}
		if !bytes.Equal(reply.Payload, response) {
			t.Errorf("reply payload = %q, want %q", reply.Payload, response)
		}
		errCh <- nil
	}()

	// Server receives the request and replies.
	got, err := server.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != RequestQueryLoginServerInfo {
		t.Fatalf("server got type %v", got.Type)
	}
	if !bytes.Equal(got.Payload, request) {
		t.Fatalf("server payload = %q, want %q", got.Payload, request)
	}
	if err := server.Send(Message{Type: Reply, Index: got.Index, Payload: response}); err != nil {
		t.Fatal(err)
	}

	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestPlaintextRoundTrip(t *testing.T) {
	sc, cc := net.Pipe()
	defer sc.Close()
	defer cc.Close()
	server := NewStream(sc)
	client := NewStream(cc)

	go func() {
		_ = client.Send(Message{Type: KeyMaterial, Index: 3, Payload: []byte{1, 2, 3, 4, 5, 6, 7, 8}})
	}()
	got, err := server.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != KeyMaterial || got.Index != 3 {
		t.Fatalf("got type=%v index=%d", got.Type, got.Index)
	}
	if !bytes.Equal(got.Payload, []byte{1, 2, 3, 4, 5, 6, 7, 8}) {
		t.Fatalf("payload mismatch: %x", got.Payload)
	}
}
