# Kaitai Struct definitions — PS3 / original Dark Souls 2 (BLUS41045)

Decompilation-derived and live-confirmed specs for what a real PS3 client actually
speaks. Written 2026-08-05.

## Why this exists separately from `../pc/`

`dev/proto/pc/` was derived from the DS3OS reference, which targets **PC / Scholar
of the First Sin**. Our client is **PS3 / original Dark Souls 2**, differing on both
platform and edition — and the two genuinely diverge.

**Where these disagree, `ps3/` wins.** This directory is built from disassembly of
the retail EBOOT plus traffic captured from a real console, not from a
reimplementation of a different edition.

## Files

| File | Covers |
|---|---|
| `outgoing/frpg2_auth_out_ps3.ksy` | The 56-byte `game_server_info` struct (auth stage 4) |
| `incoming/frpg2_game_in_ps3.ksy` | Game-service datagram, rUDP, fragment, message + the full PS3 opcode enum |
| `outgoing/frpg2_game_out_ps3.ksy` | Server-to-client datagram + the push opcode enum |

Login and auth *framing* is unchanged from PC and is byte-verified for both — use
`../pc/` for those, and this directory's auth file only for the stage-4 struct.

## The differences that will bite you

**1. `game_server_info` is 56 bytes, not 184.** Hard equality check at vaddr
`0x167091c`. On mismatch the client skips the entire struct copy silently — no
error, no log — then binds `0.0.0.0:0` and never sends. It has a binary u32 IP (not
ASCII), no `stack_data` block, and ten trailing u32 rather than eleven.

**2. Six PC opcodes do not exist here.** `0x03FA`, `0x03FB`, `0x03FC`, `0x03FD`,
`0x03FF`, `0x0400` have no code in this client. The opcode space is `0x0320` plus
`0x0386`–`0x03F9` and nothing above.

**3. BreakIn pushes are `0x03B9`–`0x03C8`**, sixteen aliases — *not* the
`0x03FB`/`0x03FC`/`0x03FD` the PC reference lists. A server built from the PC map
would never get an invasion push dispatched.

**4. Sixteen opcodes register no response callback**, where the PC reference
classifies most as request/response. Confirmed live: the client reached in-game
while `0x03A8` and `0x03B8` went unanswered.

**5. Five "DS3-only" messages are present in DS2 here**, two of them at different
opcodes than DS3 uses (`0x03B5`, `0x03B7`).

## Confidence

Opcodes marked **LIVE** in the enums were driven end to end by a real BLUS41045
client against this server: `0x0386`, `0x03A8`, `0x03B2`, `0x03B6`, `0x03B8`,
`0x03EC`. Everything else is decomp-derived at the confidence levels recorded in
`docs/protocol-map-ps3.md`.

Two things are explicitly **unresolved** and marked as such in the specs rather
than guessed:

- Which alias within each push block maps to which message type. A single live
  invasion, visit or arena capture would settle it.
- Whether pushes really use transport opcode `0x0320` with `msg_index 0xFFFFFFFF`
  as on PC. The client's dispatcher keys on a value that static analysis could not
  trace to either the header or a protobuf field.

## See also

- `docs/protocol-map-ps3.md` — the full decompilation report, with addresses,
  disassembly evidence and per-finding confidence.
- `docs/protocol-map.md` — the PC/SOTFS map. Broader on message *payloads*, but
  actively misleading on PS3 opcodes past auth. Cross-reference before trusting it.
