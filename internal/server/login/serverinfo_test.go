package login

import (
	"encoding/hex"
	"testing"
)

// TestEncodeServerInfoPS3 pins the exact bytes recovered from the BLUS41045
// parser at vaddr 0x15E0370.
//
// The reference byte string below was derived independently from the
// disassembly for port 50000 / 192.168.1.100. It matters that field 2 is tag
// 0x10 (varint) and NOT 0x12 (length-delimited): the client gates every field on
// wiretype 0 and silently skips anything else, leaving the address at zero. That
// failure is invisible — the client dials 0.0.0.0 with the correct port — so a
// regression here would look like a network problem, not a protocol one.
func TestEncodeServerInfoPS3(t *testing.T) {
	got, err := encodeServerInfoPS3("192.168.1.100", 50000)
	if err != nil {
		t.Fatal(err)
	}
	const want = "08d0860310e482a0850c"
	if hex.EncodeToString(got) != want {
		t.Errorf("encoding\n got %s\nwant %s", hex.EncodeToString(got), want)
	}

	// Field 2's tag must be varint. 0x12 would be the string form that fails.
	if got[4] != 0x10 {
		t.Errorf("field 2 tag is %#x, want 0x10 (varint); 0x12 is the string form the client skips", got[4])
	}
}

// TestEncodeServerInfoPS3ByteOrder checks the address is packed
// (a<<24)|(b<<16)|(c<<8)|d, which is what reaches sin_addr unconverted.
func TestEncodeServerInfoPS3ByteOrder(t *testing.T) {
	got, err := encodeServerInfoPS3("1.2.3.4", 1)
	if err != nil {
		t.Fatal(err)
	}
	// Locate field 2's tag rather than assuming an offset — the port varint's
	// length varies with its value.
	v, ok := addressField(got)
	if !ok {
		t.Fatalf("no field 2 varint found in % x", got)
	}
	if v != 0x01020304 {
		t.Errorf("address value: got %#x, want %#x (a<<24|b<<16|c<<8|d)", v, 0x01020304)
	}
}

// addressField skips field 1's varint and decodes field 2's.
func addressField(b []byte) (uint64, bool) {
	if len(b) < 2 || b[0] != 0x08 {
		return 0, false
	}
	i := 1
	for i < len(b) && b[i]&0x80 != 0 { // skip field 1's varint
		i++
	}
	i++ // last byte of field 1
	if i >= len(b) || b[i] != 0x10 {
		return 0, false
	}
	i++
	var v uint64
	var shift uint
	for ; i < len(b); i++ {
		v |= uint64(b[i]&0x7f) << shift
		if b[i]&0x80 == 0 {
			return v, true
		}
		shift += 7
	}
	return 0, false
}

func TestEncodeServerInfoPS3RejectsNonIPv4(t *testing.T) {
	for _, bad := range []string{"", "localhost", "example.com", "::1"} {
		if _, err := encodeServerInfoPS3(bad, 50000); err == nil {
			t.Errorf("encodeServerInfoPS3(%q) succeeded; want an error", bad)
		}
	}
}
