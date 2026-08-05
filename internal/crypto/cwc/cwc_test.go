package cwc

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

type vector struct {
	name          string
	key, iv       []byte
	hdr, ptx, ctx []byte
	tag           []byte
}

// parseVectors reads the MODETEST-format vector file (KEY/IV/HDR/PTX/CTX/TAG).
func parseVectors(t *testing.T, path string) []vector {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open vectors: %v", err)
	}
	defer f.Close()

	unhex := func(s string) []byte {
		b, err := hex.DecodeString(strings.TrimSpace(s))
		if err != nil {
			t.Fatalf("bad hex %q: %v", s, err)
		}
		return b
	}

	var vecs []vector
	var cur *vector
	flush := func() {
		if cur != nil {
			vecs = append(vecs, *cur)
			cur = nil
		}
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "VEC"):
			flush()
			cur = &vector{name: line}
		case cur == nil:
			continue
		case strings.HasPrefix(line, "KEY"):
			cur.key = unhex(line[3:])
		case strings.HasPrefix(line, "IV"):
			cur.iv = unhex(line[2:])
		case strings.HasPrefix(line, "HDR"):
			cur.hdr = unhex(line[3:])
		case strings.HasPrefix(line, "PTX"):
			cur.ptx = unhex(line[3:])
		case strings.HasPrefix(line, "CTX"):
			cur.ctx = unhex(line[3:])
		case strings.HasPrefix(line, "TAG"):
			cur.tag = unhex(line[3:])
		case strings.HasPrefix(line, "END"):
			flush()
		}
	}
	flush()
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return vecs
}

func TestVectors(t *testing.T) {
	vecs := parseVectors(t, "testdata/cwc.1")
	if len(vecs) == 0 {
		t.Fatal("no vectors parsed")
	}
	for _, v := range vecs {
		t.Run(v.name, func(t *testing.T) {
			c, err := NewContext(v.key)
			if err != nil {
				t.Fatalf("NewContext: %v", err)
			}
			// Only the ciphertext is checked against this file.
			//
			// cwc.1's TAG column comes from a different CWC variant than the
			// one FromSoftware's clients speak, so it is NOT a valid oracle for
			// the tag: building ref/ds3so's aes_modes/cwc.c and running these
			// same inputs through it reproduces the CTX column but yields
			// different tags — and that C build matches a real DS2 PS3 client
			// byte-for-byte. Tags are pinned by TestGameVectors (generated from
			// that C build) and TestConsoleCapture (captured off the wire).
			ct, tag := c.EncryptMessage(v.iv, v.hdr, v.ptx, len(v.tag))
			if !bytes.Equal(ct, v.ctx) {
				t.Errorf("ciphertext mismatch\n got %x\nwant %x", ct, v.ctx)
			}
			// Round-trip through our own tag, which must self-verify.
			pt, ok := c.DecryptMessage(v.iv, v.hdr, ct, tag)
			if !ok {
				t.Fatalf("DecryptMessage: tag failed to verify")
			}
			if !bytes.Equal(pt, v.ptx) {
				t.Errorf("plaintext mismatch\n got %x\nwant %x", pt, v.ptx)
			}
		})
	}
	t.Logf("validated %d CWC vectors", len(vecs))
}

func TestDecryptRejectsTamperedTag(t *testing.T) {
	c, err := NewContext(make([]byte, 16))
	if err != nil {
		t.Fatal(err)
	}
	iv := make([]byte, 11)
	ct, tag := c.EncryptMessage(iv, nil, []byte("hello world payload"), 16)
	tag[0] ^= 0x01
	if _, ok := c.DecryptMessage(iv, nil, ct, tag); ok {
		t.Fatal("expected tag verification to fail on tampered tag")
	}
}

func TestRoundTripLengths(t *testing.T) {
	c, err := NewContext([]byte("0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	iv := []byte("abcdefghijk") // 11 bytes
	for n := 0; n <= 300; n++ {
		msg := make([]byte, n)
		for i := range msg {
			msg[i] = byte(i * 7)
		}
		hdr := make([]byte, n%40)
		for i := range hdr {
			hdr[i] = byte(i)
		}
		ct, tag := c.EncryptMessage(iv, hdr, msg, 16)
		pt, ok := c.DecryptMessage(iv, hdr, ct, tag)
		if !ok || !bytes.Equal(pt, msg) {
			t.Fatalf("round-trip failed at len %d (ok=%v)", n, ok)
		}
	}
}

// TestConsoleCapture pins the tag against a real Dark Souls 2 PS3 client.
//
// This is the authoritative oracle for the tag. testdata/cwc.1's TAG values
// come from a different CWC variant and do NOT match what the game speaks:
// building ref/ds3so's aes_modes/cwc.c and running vector 1 through it yields
// the vector's ciphertext but a different tag. Both that C build and this
// package reproduce the bytes below, which were captured off the wire during
// the auth stage-2 GetServiceStatus exchange.
func TestConsoleCapture(t *testing.T) {
	unhex := func(s string) []byte {
		b, err := hex.DecodeString(s)
		if err != nil {
			t.Fatalf("bad hex %q: %v", s, err)
		}
		return b
	}

	key := unhex("c142d4d3e22dccd082686e78580256a1")
	iv := unhex("52bf828e8a358d6ac1ba17")
	pt := unhex("0801120b636f6d726164657365616e2080a28808")
	wantCT := unhex("8bf6d8f1905e0c85901e4ff20da658ea712fdbbf")
	wantTag := unhex("ba9379561bf8778e799dc884d9968bd7")

	c, err := NewContext(key)
	if err != nil {
		t.Fatal(err)
	}

	// The TCP framing authenticates the IV as the header.
	ct, tag := c.EncryptMessage(iv, iv, pt, len(wantTag))
	if !bytes.Equal(ct, wantCT) {
		t.Errorf("ciphertext\n got %x\nwant %x", ct, wantCT)
	}
	if !bytes.Equal(tag, wantTag) {
		t.Errorf("tag\n got %x\nwant %x", tag, wantTag)
	}

	got, ok := c.DecryptMessage(iv, iv, wantCT, wantTag)
	if !ok {
		t.Fatal("DecryptMessage rejected a tag captured from a real client")
	}
	if !bytes.Equal(got, pt) {
		t.Errorf("plaintext\n got %x\nwant %x", got, pt)
	}
}

// TestGameVectors validates tags against testdata/cwc-game.txt, generated from
// ref/ds3so's aes_modes/cwc.c — the implementation that reproduces a real DS2
// PS3 client's tag. This is the authoritative tag oracle for this package;
// cwc.1's TAG column is a different variant. See TestConsoleCapture.
func TestGameVectors(t *testing.T) {
	f, err := os.Open("testdata/cwc-game.txt")
	if err != nil {
		t.Fatalf("open game vectors: %v", err)
	}
	defer f.Close()

	unhex := func(s string) []byte {
		if s == "-" {
			return nil
		}
		b, err := hex.DecodeString(s)
		if err != nil {
			t.Fatalf("bad hex %q: %v", s, err)
		}
		return b
	}

	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 9 {
			t.Fatalf("malformed line (%d fields): %q", len(fields), line)
		}
		key, iv := unhex(fields[3]), unhex(fields[4])
		hdr, ptx := unhex(fields[5]), unhex(fields[6])
		wantCT, wantTag := unhex(fields[7]), unhex(fields[8])

		c, err := NewContext(key)
		if err != nil {
			t.Fatalf("NewContext: %v", err)
		}
		ct, tag := c.EncryptMessage(iv, hdr, ptx, len(wantTag))
		if !bytes.Equal(ct, wantCT) {
			t.Errorf("klen=%s hlen=%s mlen=%s ciphertext\n got %x\nwant %x",
				fields[0], fields[1], fields[2], ct, wantCT)
		}
		if !bytes.Equal(tag, wantTag) {
			t.Errorf("klen=%s hlen=%s mlen=%s tag\n got %x\nwant %x",
				fields[0], fields[1], fields[2], tag, wantTag)
		}
		if _, ok := c.DecryptMessage(iv, hdr, wantCT, wantTag); !ok {
			t.Errorf("klen=%s hlen=%s mlen=%s: DecryptMessage rejected the reference tag",
				fields[0], fields[1], fields[2])
		}
		n++
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no game vectors loaded")
	}
	t.Logf("validated %d CWC game vectors", n)
}
