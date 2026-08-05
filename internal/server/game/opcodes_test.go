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

// forbidden are the opcodes docs/protocol-map-ps3.md §"DO NOT IMPLEMENT" calls
// out as present in the PC map but absent from this binary.
//
// SCOPE MATTERS: that map decompiles **v1.00**, and its evidence is sound for
// that build. It is not evidence about v1.10, which adds opcodes v1.00 does not
// have — 0x03FA was on this list until two v1.10 clients were seen sending it,
// and `li r4,0x03fa` really does occur zero times in v1.00 and twice in v1.10.
//
// So an opcode may leave this list on live evidence from a later build. The rest
// stay because nothing has been observed sending them in any version. See
// versions.go.
var forbidden = map[uint32]string{
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

// TestVersionSpecificOpcodesAreNotForbidden keeps the two lists coherent: an
// opcode known to exist in some supported build must not also be listed as one
// that exists in none.
func TestVersionSpecificOpcodesAreNotForbidden(t *testing.T) {
	for op, ver := range opcodeIntroducedIn {
		if why, bad := forbidden[op]; bad {
			t.Errorf("%#04x is recorded as introduced in v%s but also listed as "+
				"absent from every build (%s); one of the two is wrong", op, ver, why)
		}
	}
}

// TestVersionSpecificOpcodesAreDispatched — recording that an opcode exists is
// only useful if we answer it. Every entry in the version map should be routed,
// since we know a real client sends it.
func TestVersionSpecificOpcodesAreDispatched(t *testing.T) {
	dispatched := map[uint32]bool{}
	for _, op := range dispatchedOpcodes(t) {
		dispatched[op] = true
	}
	for op, ver := range opcodeIntroducedIn {
		if !dispatched[op] {
			t.Errorf("%#04x exists in v%s but nothing dispatches it; an unanswered "+
				"request/response opcode makes the client retry silently and blocks "+
				"other online UI", op, ver)
		}
	}
}
