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
			ct, tag := c.EncryptMessage(v.iv, v.hdr, v.ptx, len(v.tag))
			if !bytes.Equal(ct, v.ctx) {
				t.Errorf("ciphertext mismatch\n got %x\nwant %x", ct, v.ctx)
			}
			if !bytes.Equal(tag, v.tag) {
				t.Errorf("tag mismatch\n got %x\nwant %x", tag, v.tag)
			}
			// Round-trip: decrypt must recover plaintext and verify the tag.
			pt, ok := c.DecryptMessage(v.iv, v.hdr, v.ctx, v.tag)
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
