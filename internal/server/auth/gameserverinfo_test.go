package auth

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/sstreight/dso/internal/server/authtoken"
)

// TestGameServerInfoSize pins the length the client demands.
//
// BLUS41045 compares the payload length against 56 with a hard equality check
// (vaddr 0x167091c) and silently skips the whole struct copy on any mismatch —
// no error, no log, the client just never opens its UDP session. A regression
// here is therefore invisible at runtime, which is why it gets its own test.
func TestGameServerInfoSize(t *testing.T) {
	var tok authtoken.Token
	got, err := encodeGameServerInfo(tok, "192.168.1.100", 50010)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 56 {
		t.Fatalf("payload is %d bytes; the client requires exactly 56", len(got))
	}
}

func TestGameServerInfoLayout(t *testing.T) {
	tok := authtoken.Token{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04}
	buf, err := encodeGameServerInfo(tok, "192.168.1.100", 50010)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(buf[0:8], tok[:]) {
		t.Errorf("auth_token: got %x, want %x (must be copied verbatim, not swapped)", buf[0:8], tok[:])
	}

	// The IP is a raw binary u32, not an ASCII string: 192.168.1.100 -> C0 A8 01 64.
	wantIP := []byte{0xC0, 0xA8, 0x01, 0x64}
	if !bytes.Equal(buf[8:12], wantIP) {
		t.Errorf("game_server_ip: got %x, want %x", buf[8:12], wantIP)
	}

	if p := binary.BigEndian.Uint16(buf[12:14]); p != 50010 {
		t.Errorf("game_port: got %d, want 50010", p)
	}
	if pad := binary.BigEndian.Uint16(buf[14:16]); pad != 0 {
		t.Errorf("padding: got %d, want 0", pad)
	}

	// Ten trailing uint32s, big-endian. The first two are applied verbatim as
	// SO_SNDBUF and SO_RCVBUF, so a zero here bricks the client's socket.
	for i, want := range tuning {
		off := 16 + i*4
		if got := binary.BigEndian.Uint32(buf[off : off+4]); got != want {
			t.Errorf("tuning[%d] at offset %d: got %#x, want %#x", i, off, got, want)
		}
	}
	if got := binary.BigEndian.Uint32(buf[16:20]); got == 0 {
		t.Error("tuning[0] (SO_SNDBUF) is zero; the client would create a zero-sized send buffer")
	}
	if got := binary.BigEndian.Uint32(buf[20:24]); got == 0 {
		t.Error("tuning[1] (SO_RCVBUF) is zero; the client would create a zero-sized receive buffer")
	}
}

// TestGameServerInfoRejectsNonIPv4 guards the 4-byte address field: a hostname or
// an IPv6 address cannot be represented and must fail loudly rather than being
// silently truncated into a wrong address.
func TestGameServerInfoRejectsNonIPv4(t *testing.T) {
	var tok authtoken.Token
	for _, bad := range []string{"", "localhost", "example.com", "::1", "2001:db8::1"} {
		if _, err := encodeGameServerInfo(tok, bad, 50010); err == nil {
			t.Errorf("encodeGameServerInfo(%q) succeeded; want an error", bad)
		}
	}
}
