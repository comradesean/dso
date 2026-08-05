# Status — 2026-08-05

Where the project actually is, what is proven versus assumed, and what to pick up next.
Complements `RECOVERED_PLAN.md`, which is the original plan and is now partly overtaken.

## Headline

**A real Dark Souls 2 PS3 client (BLUS41045) reaches in-game play against this server.**

Login, the four-stage auth handshake, and the reliable-UDP game session all work end to end
against retail hardware under RPCS3. That clears **M1** and its stretch goal **CP4** as defined
in `RECOVERED_PLAN.md`: the milestone was "a real DS2 client under RPCS3 completes login + auth
and receives a well-formed `Frpg2GameServerInfo`", with reaching the UDP boot `player_id` as the
stretch.

## What works, and how well it is proven

| Area | State | Evidence |
|---|---|---|
| Login service (TCP) | Working | Byte-verified against the console |
| Auth handshake, 4 stages (TCP) | Working | Byte-verified; full handshake |
| AES-CWC cipher | Working | Tag matches the console byte-for-byte |
| `Frpg2GameServerInfo` (56-byte PS3 struct) | Working | Client opens its UDP session |
| Reliable-UDP session | Working | Reaches Established with a real client |
| Boot: login → announcements → character slot | Working | Client reaches in-game |
| Blood messages (6 opcodes) | Implemented | Unit-tested; **not yet driven by the console** |
| Ghosts (create, list) | Implemented | **Not yet confirmed in-game** |
| Player status / character uploads | Recorded | Accepted by the client; nothing consumes them |

Six opcodes have been driven by a real console: `0x0386`, `0x03A8`, `0x03B2`, `0x03B6`,
`0x03B8`, `0x03EC`.

## Not built

Signs/summoning, invasions, visitors, quick match, ranking, telemetry, Mirror Knight, and all
server→client **pushes**. Nothing is persisted — player ids, character ids, messages and ghosts
are per-run and in memory.

## Things that cost real time, recorded so they are not re-learned

**The CWC test vectors are the wrong oracle.** `internal/crypto/cwc/testdata/cwc.1` produces
tags the game rejects. The variant the game speaks is the one in the reference C implementation;
matching the published vectors is actively wrong. CP0 being green was false confidence. The
authoritative oracle is now `TestConsoleCapture` (real console bytes) plus `TestGameVectors`
(generated from the C build). The actual bug was byte order in the block load — the reference
byte-swaps `Z` but *not* the data block, so each 4-byte group is reversed relative to a plain
big-endian read. Ciphertext was always correct; only tags differed.

**The EBOOT holds two RSA keys and only one is the login key.** `0x17FB338` is the login key
(byte-identical to the PC one); `0x189AB48` is something else. Patching the wrong one gives a
client that connects but whose RSA block never decrypts.

**`Frpg2GameServerInfo` is 56 bytes on PS3, not the PC's 184.** The client checks the length
with a hard equality and, on mismatch, skips the entire struct copy *silently* — no error, no
log — then binds `0.0.0.0:0` and never sends. Symptom: auth completes perfectly and the client
appears to simply choose not to play.

**The PC-derived protocol map is unsafe past auth.** `docs/protocol-map.md` is broader on
message payloads but wrong for PS3 in fourteen places. `docs/protocol-map-ps3.md` is
decompilation-derived and wins for PS3. Most dangerous divergence: BreakIn pushes are
`0x03B9`–`0x03C8`, not the `0x03FB`/`0x03FC`/`0x03FD` the PC map lists — values with no code at
all in this client.

**The client dictates handler order.** It will not open other online UI while a request/response
message is unanswered; it silently retries instead. The Message menu appeared broken purely
because an unanswered `RequestCreateGhostData` was outstanding.

**A server restart invalidates every auth token.** The registry is in-memory, so a client mid-session
will spin on `unknown or expired auth token`. Back out to the menu and re-enter online mode
rather than retrying in place.

**Run the server from WSL, not Windows.** Windows cannot bind TCP on the LAN address for any
port; WSL shares the same IP and works. See the `dso-run-server-from-wsl` note.

## Open questions

- **Push transport is unresolved on PS3.** The PC model is opcode `0x0320` with
  `msg_index 0xFFFFFFFF`, disambiguated by the first protobuf field. Decompilation could not
  establish whether the PS3 dispatcher keys on the transport header or a parsed field. Nothing
  should be built on the PC model until one push has been driven end to end.
- **Push alias blocks are unmapped.** BreakIn registers 16 handlers for 4 message types,
  Visitor 9 for 3, QuickMatch 8 for 4. Which alias means which is unknown; static analysis
  cannot separate them. One live invasion capture would resolve all sixteen.
- **Four opcodes are unidentified**: `0x0387`, `0x0388`, `0x038A`, `0x0390`.
- **The UDP `msg_index` byte order** looks big-endian where TCP is little-endian. It round-trips
  correctly because the server echoes the same bytes, so it is cosmetic — until something
  depends on ordering.
- **The login reply may have the same ASCII-vs-binary address bug** the 56-byte struct had: a
  client was observed dialling the auth server at `0.0.0.0:50000`, correct port and zero
  address, rescued only by RPCS3 redirecting it. **This would fail outright on real hardware**
  and should be fixed before touching a real PS3.

## Suggested next steps

1. Confirm blood messages and ghosts actually work in-game — both are implemented but only
   unit-tested.
2. Fix the login-reply address bug above; it is the one known blocker for real hardware.
3. Drive an invasion to resolve the BreakIn push aliases, which unblocks all matchmaking work.
4. Persistence, so messages and characters survive a restart.
