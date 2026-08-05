package game

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The decompilation-derived PS3 map is the blueprint. docs/protocol-map.md is
// PC/SOTFS-derived and lists opcodes this client does not contain, so anything
// built from it — or from ref/ds3os, which targets the PC protocol — can end up
// implementing messages that will never arrive and sending pushes that will never
// dispatch.
const ps3MapPath = "../../../docs/protocol-map-ps3.md"

// forbidden are the six opcodes docs/protocol-map-ps3.md §"DO NOT IMPLEMENT"
// calls out: present in the PC map, absent from this binary. The maximum opcode
// across all 132 send/register call sites is 0x03F9, and no `li r4` with any of
// these values exists anywhere in .text.
var forbidden = map[uint32]string{
	0x03FA: "RequestGetRightMatchingArea",
	0x03FB: "PushRequestBreakInTarget (PS3 uses 0x03B9-0x03C8)",
	0x03FC: "PushRequestRejectBreakInTarget",
	0x03FD: "PushRequestAllowBreakInTarget",
	0x03FF: "RequestGetAreaBloodMessageList",
	0x0400: "RequestGetAreaBloodstainList",
}

// dispatchedOpcodes is every opcode boot.go actually routes, read from the source
// so the test cannot drift from the dispatcher.
func dispatchedOpcodes(t *testing.T) map[string]uint32 {
	t.Helper()
	src, err := os.ReadFile("boot.go")
	if err != nil {
		t.Fatal(err)
	}
	cases := regexp.MustCompile(`case (op[A-Za-z0-9_]+):`).FindAllSubmatch(src, -1)
	if len(cases) == 0 {
		t.Fatal("no dispatch cases found; the parser has drifted from boot.go")
	}

	// Constant values live across the package's files.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]uint32{}
	decl := regexp.MustCompile(`(op[A-Za-z0-9_]+)\s+(?:uint32\s+)?=\s+0x([0-9A-Fa-f]+)`)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		b, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range decl.FindAllSubmatch(b, -1) {
			v, err := strconv.ParseUint(string(m[2]), 16, 32)
			if err != nil {
				continue
			}
			values[string(m[1])] = uint32(v)
		}
	}

	out := map[string]uint32{}
	for _, c := range cases {
		name := string(c[1])
		v, ok := values[name]
		if !ok {
			t.Errorf("dispatch case %s has no opcode constant", name)
			continue
		}
		out[name] = v
	}
	// A guard that examines nothing passes silently, which is worse than no
	// guard. Both callers depend on this being a real list.
	if len(out) < 20 {
		t.Fatalf("only resolved %d dispatched opcodes; the constant or case parser "+
			"has drifted and these guards are not actually checking anything", len(out))
	}
	return out
}

// TestNoForbiddenOpcodes is the guard that matters most. Implementing any of
// these is silent: the message never arrives, or the push is never dispatched, so
// the feature simply does nothing and looks like a bug elsewhere.
func TestNoForbiddenOpcodes(t *testing.T) {
	for name, op := range dispatchedOpcodes(t) {
		if why, bad := forbidden[op]; bad {
			t.Errorf("%s dispatches %#04x, which does not exist in BLUS41045 (%s). "+
				"See the DO NOT IMPLEMENT section of docs/protocol-map-ps3.md.",
				name, op, why)
		}
	}
}

// TestDispatchedOpcodesAreInThePS3Map asserts every opcode we route was actually
// found in the client's own binary, rather than inherited from the PC map or from
// ref/ds3os.
func TestDispatchedOpcodesAreInThePS3Map(t *testing.T) {
	doc, err := os.ReadFile(ps3MapPath)
	if err != nil {
		t.Skipf("PS3 map not readable (%v); skipping blueprint check", err)
	}

	// Rows in the confirmed table look like:  | `0x03EC` | R/R | `Request...`
	// Ranges look like:                       | `0x03B9`-`0x03C8` | P x16 | ...
	present := map[uint32]bool{}
	row := regexp.MustCompile("(?m)^\\| *`(0x[0-9A-Fa-f]{4})`(?:[-–]`(0x[0-9A-Fa-f]{4})`)? *\\|([^|]*)\\|")
	for _, m := range row.FindAllSubmatch(doc, -1) {
		if strings.Contains(string(m[3]), "Absent") {
			continue
		}
		lo, _ := strconv.ParseUint(strings.TrimPrefix(string(m[1]), "0x"), 16, 32)
		hi := lo
		if len(m[2]) > 0 {
			hi, _ = strconv.ParseUint(strings.TrimPrefix(string(m[2]), "0x"), 16, 32)
		}
		for v := lo; v <= hi; v++ {
			present[uint32(v)] = true
		}
	}
	if len(present) < 50 {
		t.Skipf("only parsed %d opcodes from the map; format has changed, "+
			"fix the parser rather than trusting this test", len(present))
	}

	for name, op := range dispatchedOpcodes(t) {
		if !present[op] {
			t.Errorf("%s dispatches %#04x, which is not listed as present in %s. "+
				"Either the opcode does not exist on PS3, or the map needs updating — "+
				"do not assume the PC map or ds3os is right about it.",
				name, op, ps3MapPath)
		}
	}
}
