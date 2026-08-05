package game

// Opcode ownership by game version.
//
// dso supports more than one build of BLUS41045, and the opcode set is NOT the
// same across them. `docs/protocol-map-ps3.md` is a decompilation of the **v1.00
// launch disc**, and is accurate for it — but the v1.10 title update adds
// opcodes the launch build genuinely does not contain.
//
// This cost real debugging time once already. `0x03FA` was listed as "absent,
// do not implement" on the strength of the v1.00 scan; two live v1.10 clients
// then sent it at boot, and it went unanswered for months of session time. The
// evidence is unambiguous once you look at the right binary — the `li r4,imm`
// encoding `38 80 iiii` appears 0 times in the v1.00 EBOOT and twice in v1.10.
//
// So: "absent" is always absent *from a particular build*. Record which.

// GameVersion identifies a BLUS41045 build.
type GameVersion string

const (
	// VersionV100 is the launch disc, and the build docs/protocol-map-ps3.md
	// describes. EBOOT 30,756,464 bytes, PPU hash
	// PPU-efd8e00902c586ebf88f5c97dcbdfe27bdce3bcc.
	VersionV100 GameVersion = "1.00"
	// VersionV110 is the title update. EBOOT 31,301,872 bytes, PPU hash
	// PPU-e2da49d8c8b32cdddd8639ca215b6b5c00ff64b0. This is what our test clients
	// run, so where the builds differ, this is the one live behaviour follows.
	VersionV110 GameVersion = "1.10"
)

// opcodeIntroducedIn records opcodes that do NOT exist in every supported build,
// mapped to the earliest build containing them.
//
// Only exceptions are listed. Anything absent from this map is present in every
// supported version, which is the overwhelmingly common case — the two builds
// share almost the entire opcode set.
//
// Serving an opcode a client cannot dispatch is harmless (it is simply never
// sent to us), so entries here are documentation and test input rather than a
// runtime gate. There is no need to refuse to answer a 1.10 opcode on a 1.00
// client, because a 1.00 client will never ask.
var opcodeIntroducedIn = map[uint32]GameVersion{
	// CONFIRMED: `li r4,0x03fa` (encoding 38 80 03 fa) occurs 0 times in the
	// v1.00 EBOOT and twice in v1.10. Confirmed live on 2026-08-05 — two v1.10
	// clients on separate machines each sent it at boot with a 29-byte payload
	// decoding as RequestGetRightMatchingArea{matching_parameter}.
	//
	// It feeds the bonfire warp screen's population hints, which Patch 1.10 added
	// — so the opcode and the feature arrived together, which is corroboration in
	// itself.
	opRequestGetRightMatchingArea: VersionV110,
}

// IntroducedIn returns the earliest supported build containing an opcode, and
// whether it is version-specific at all.
func IntroducedIn(op uint32) (GameVersion, bool) {
	v, ok := opcodeIntroducedIn[op]
	return v, ok
}
