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

## What a live capture corpus added (2026-08-07)

Reading FromSoftware's own traffic settled three things these specs had left open
or wrong, all of them in the message layer:

- **`msg_index` is little-endian**, while the two fields before it are big-endian.
  Proven 6,515/6,515. It is not cosmetic: a reply echoes its request's index, and
  since a reply carries opcode 0 that echo is the only thing that identifies it.
- **A reply's header is 28 bytes, a push's is 12.** Probing for the protobuf start
  instead of deriving it from the header put 42% of a corpus at the wrong offset,
  with the index parsed as protobuf.
- **The push model is the PC model** — `0x0320`, `msg_index = 0xFFFFFFFF`, real id
  in protobuf field 1. Previously marked UNRESOLVED here.

Also: the payload can contain protobuf **groups** (wire types 3/4), which DS2 still
uses; and zlib streams use a 4 KB window, so they begin `58 c3`, not `78 9c`.

## The differences that will bite you

**1. `game_server_info` is 56 bytes, not 184.** Hard equality check at vaddr
`0x167091c`. On mismatch the client skips the entire struct copy silently — no
error, no log — then binds `0.0.0.0:0` and never sends. It has a binary u32 IP (not
ASCII), no `stack_data` block, and ten trailing u32 rather than eleven.

**2. FIVE PC opcodes do not exist here** — `0x03FB`, `0x03FC`, `0x03FD`, `0x03FF`,
`0x0400`. On the launch disc the opcode space is `0x0320` plus `0x0386`–`0x03F9`.

> **Corrected 2026-08-07.** This used to say six, including `0x03FA`. That opcode
> **does** exist, in **v1.10 only**: `li r4,0x03fa` occurs zero times in the v1.00
> EBOOT and twice in the title update, and two real v1.10 clients were seen sending
> it at boot. It is `RequestGetRightMatchingArea`, it feeds the bonfire warp
> screen's population hints, and it is implemented. "Absent from the binary" is
> only ever true of the build it was measured on.

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
