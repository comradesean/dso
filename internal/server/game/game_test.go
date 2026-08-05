package game

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/sstreight/dso/internal/config"
	"github.com/sstreight/dso/internal/crypto/frpgcipher"
	"github.com/sstreight/dso/internal/frpg/rudp"
	"github.com/sstreight/dso/internal/server/authtoken"
	"github.com/sstreight/dso/internal/server/core"
	"github.com/sstreight/dso/internal/server/store"
)

// newTestService starts a game service on an ephemeral UDP port and returns it
// along with the address to dial. It mirrors what the auth service does: mint a
// token, register it against a CWC key.
func newTestService(t *testing.T) (*core.Server, authtoken.Token, []byte, *net.UDPAddr) {
	t.Helper()

	// Pick a free UDP port by binding and releasing one.
	probe, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.LocalAddr().(*net.UDPAddr).Port
	_ = probe.Close()

	cfg := config.Default()
	cfg.BindAddress = "127.0.0.1"
	cfg.GamePort = port

	srv := &core.Server{
		Config: cfg,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Tokens: authtoken.NewRegistry(),
	}

	var tok authtoken.Token
	if _, err := rand.Read(tok[:]); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	srv.Tokens.Add(tok, key)

	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := New(srv, st)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = svc.Serve(ctx) }()

	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port}
	waitForListener(t, addr)
	return srv, tok, key, addr
}

// waitForListener blocks until something is bound to addr. A UDP write cannot
// be used as the probe: it succeeds locally even when nothing is listening, so
// it would return before Serve's ListenPacket had run and the first real
// datagram would be dropped. Instead, try to bind the port ourselves — that
// fails precisely while the service holds it.
func waitForListener(t *testing.T, addr *net.UDPAddr) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pc, err := net.ListenPacket("udp", addr.String())
		if err != nil {
			return // the port is taken, i.e. the service is up
		}
		_ = pc.Close()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("game service did not start listening")
}

// TestSessionEstablishes drives the reliable-UDP handshake from a client that
// holds a valid token, and asserts the server answers the SYN.
func TestSessionEstablishes(t *testing.T) {
	_, tok, key, addr := newTestService(t)

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	cipher, err := frpgcipher.NewClientUDPCipher(key, tok)
	if err != nil {
		t.Fatal(err)
	}

	sess := rudp.NewClientSession("testclient", func(reliable []byte, prefix bool) error {
		sealed, err := cipher.Seal(reliable, prefix)
		if err != nil {
			return err
		}
		_, err = conn.Write(sealed)
		return err
	})
	sess.Connect()

	// Drive the handshake to completion. Established is not reached on the
	// first SYNACK — handleSYNACK moves both peers to SynReceived and the ACK
	// exchange that follows is what completes it, so this pumps and feeds until
	// the client settles or the deadline passes.
	buf := make([]byte, 4096)
	gotReply := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && sess.State() != rudp.StateEstablished {
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, err := conn.Read(buf)
		if err != nil {
			if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
				t.Fatalf("read failed: %v", err)
			}
			if err := sess.Pump(); err != nil {
				t.Fatalf("client pump failed: %v", err)
			}
			continue
		}
		pt, _, err := cipher.Open(buf[:n])
		if err != nil {
			t.Fatalf("server datagram failed to decrypt: %v", err)
		}
		if len(pt) == 0 {
			t.Fatal("server datagram decrypted to an empty payload")
		}
		gotReply = true
		sess.Feed(pt)
		if err := sess.Pump(); err != nil {
			t.Fatalf("client pump failed: %v", err)
		}
	}

	if !gotReply {
		t.Fatal("server never answered the SYN")
	}
	if got := sess.State(); got != rudp.StateEstablished {
		t.Fatalf("client session did not establish: state=%v", got)
	}
}

// TestUnknownTokenIsIgnored checks that a peer without a registered token gets
// no reply at all — it should not be able to probe for the service.
func TestUnknownTokenIsIgnored(t *testing.T) {
	_, _, _, addr := newTestService(t)

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	var bogus authtoken.Token
	if _, err := rand.Read(bogus[:]); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 16)
	cipher, err := frpgcipher.NewClientUDPCipher(key, bogus)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := cipher.Seal([]byte("hello"), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(sealed); err != nil {
		t.Fatal(err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if n, err := conn.Read(make([]byte, 1024)); err == nil {
		t.Fatalf("server replied (%d bytes) to an unregistered token", n)
	}
}

// TestShortDatagramIsIgnored guards the token-extraction bounds check.
func TestShortDatagramIsIgnored(t *testing.T) {
	_, _, _, addr := newTestService(t)

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if n, err := conn.Read(make([]byte, 1024)); err == nil {
		t.Fatalf("server replied (%d bytes) to a truncated datagram", n)
	}
}
