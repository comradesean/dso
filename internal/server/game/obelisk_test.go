package game

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
	"testing"
	"unicode/utf16"
)

// TestObeliskFMGMatchesStockFile is the anchor for the whole builder.
//
// data/regpush/stock/regulationEnglish.fmg is the real file, extracted from the
// game's own regulation archive, and its bytes were compared word for word
// against the client's live buffer at guest 0x312883F0. If the builder can
// reproduce it exactly from its own text, every field it writes is right.
func TestObeliskFMGMatchesStockFile(t *testing.T) {
	stock, err := os.ReadFile("../../../data/regpush/stock/regulationEnglish.fmg")
	if err != nil {
		t.Skipf("stock FMG not available: %v", err)
	}

	got, err := buildObeliskFMG("The letters are worn beyond recognition.")
	if err != nil {
		t.Fatalf("buildObeliskFMG: %v", err)
	}
	if !bytes.Equal(got, stock) {
		t.Errorf("built FMG differs from the stock file\n got %d bytes: % x\nwant %d bytes: % x",
			len(got), got, len(stock), stock)
	}
}

func TestObeliskFMGStructure(t *testing.T) {
	const text = "Hello"
	got, err := buildObeliskFMG(text)
	if err != nil {
		t.Fatalf("buildObeliskFMG: %v", err)
	}

	if n := binary.BigEndian.Uint32(got[0x04:]); int(n) != len(got) {
		t.Errorf("size field = %d, want %d", n, len(got))
	}
	// The applier relocates exactly this field (*(dst+20) += dst) and nothing
	// else, so it has to be the offset of the string-offset table.
	if n := binary.BigEndian.Uint32(got[0x14:]); n != 0x28 {
		t.Errorf("offset-table pointer = %#x, want 0x28", n)
	}
	if n := binary.BigEndian.Uint32(got[0x28:]); n != obeliskHeaderLen {
		t.Errorf("string offset = %#x, want %#x", n, obeliskHeaderLen)
	}
	for _, off := range []int{0x20, 0x24} {
		if n := binary.BigEndian.Uint32(got[off:]); n != obeliskStringID {
			t.Errorf("id at +%#x = %d, want %d", off, n, obeliskStringID)
		}
	}

	want := utf16.Encode([]rune(text))
	for i, u := range want {
		if n := binary.BigEndian.Uint16(got[obeliskHeaderLen+i*2:]); n != u {
			t.Fatalf("unit %d = %#04x, want %#04x", i, n, u)
		}
	}
	if n := binary.BigEndian.Uint16(got[obeliskHeaderLen+len(want)*2:]); n != 0 {
		t.Errorf("string is not NUL terminated: %#04x", n)
	}
}

// TestObeliskFMGRejectsOversize guards the one mistake that corrupts the client
// rather than merely failing. Neither apply route compares the payload against
// the destination, and the destination for this resource is 1024 bytes.
func TestObeliskFMGRejectsOversize(t *testing.T) {
	if _, err := buildObeliskFMG(strings.Repeat("A", 4096)); err == nil {
		t.Fatal("oversize text was accepted; it must be rejected, never truncated")
	}
	// The largest text that still fits must be accepted, so the cap is not
	// quietly stricter than advertised.
	fits := (obeliskMaxBytes-obeliskHeaderLen)/2 - 1
	got, err := buildObeliskFMG(strings.Repeat("A", fits))
	if err != nil {
		t.Fatalf("%d characters should fit in %d bytes: %v", fits, obeliskMaxBytes, err)
	}
	if len(got) > obeliskMaxBytes {
		t.Errorf("built %d bytes, over the %d cap", len(got), obeliskMaxBytes)
	}
}

func TestObeliskFMGNewlineEscape(t *testing.T) {
	got, err := buildObeliskFMG(`one\ntwo`)
	if err != nil {
		t.Fatalf("buildObeliskFMG: %v", err)
	}
	want := utf16.Encode([]rune("one\ntwo"))
	for i, u := range want {
		if n := binary.BigEndian.Uint16(got[obeliskHeaderLen+i*2:]); n != u {
			t.Fatalf("unit %d = %#04x, want %#04x (\\n was not unescaped)", i, n, u)
		}
	}
}
