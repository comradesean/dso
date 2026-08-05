# Status — 2026-08-05 (updated)

Where the project actually is, what is proven versus assumed, and what to pick up next.
Complements `RECOVERED_PLAN.md`, which is the original plan and is now partly overtaken.

## Headline

**Two real Dark Souls 2 PS3 clients on separate machines play together against this server.**

They see each other's blood messages, rate them and receive live notifications, summon
signs broker host-to-summoner, and **invasions work end to end** — one player invaded
the other successfully.

Login, the four-stage auth handshake, and the reliable-UDP game session all work end to end
against retail hardware under RPCS3. That clears **M1** and its stretch goal **CP4** as defined
in `RECOVERED_PLAN.md`: the milestone was "a real DS2 client under RPCS3 completes login + auth
and receives a well-formed `Frpg2GameServerInfo`", with reaching the UDP boot `player_id` as the
stretch.

## What works, and how well it is proven

| Area | State | Evidence |
|---|---|---|
| Login service (TCP) | Working | Byte-verified; address fix needed for a second machine |
| Auth handshake, 4 stages (TCP) | Working | Byte-verified |
| AES-CWC cipher | Working | Tag matches the console byte-for-byte |
| `Frpg2GameServerInfo` (56-byte PS3 struct) | Working | Client opens its UDP session |
| Reliable-UDP session | Working | Established with real clients |
| Boot: login → announcements → character slot | Working | Clients reach in-game |
| **Server→client pushes** | **Working** | Rating notification delivered live to the author |
| Blood messages (6 opcodes) | Working | Placed, listed, rated cross-client, persisted |
| Bloodstains (3 opcodes) | Working | In-game, memory-only |
| Ghosts (create, list) | Working | In-game, memory-only |
| Summon signs (6 opcodes) | Working | Brokering confirmed host↔summoner |
| Invasions (3 opcodes + relay) | Working | A real invasion completed between two clients |
| Client-to-client relay (`0x0320`) | Working | Carries the host's "allow" back to the invader |
| World death counter (4 opcodes) | Implemented | Unit-tested; not yet confirmed in-game |
| Persistence (SQLite) | Working | Blood messages and counters survive restart |
| Player status / character uploads | Recorded | Accepted; nothing consumes them yet |

## Not built

Visitors, quick match, ranking, telemetry, Mirror Knight.

Player ids and character ids are still per-run and in memory, which is the same id-reuse hazard
described below waiting to happen once anything caches them.

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

- **~~Push transport~~ RESOLVED.** The PC model — opcode `0x0320`, `msg_index 0xFFFFFFFF`,
  identity in the first protobuf field — is **correct on PS3**, proven by a rating notification
  arriving live and by summon brokering working. Decompilation could not settle this; only a
  live test could.
- **Push alias blocks are PARTLY mapped.** BreakIn registers 16 handlers for 4 message types,
  Visitor 9 for 3, QuickMatch 8 for 4, and static analysis cannot separate them. A live
  invasion confirmed **`0x03B9` is the BreakIn-target push** (group 3 of the four registration
  groups), so the guess was right first time. The other three BreakIn groups —
  reject/allow/remove — remain unassigned, as do all the Visitor and QuickMatch aliases.
  `pushBreakInRejected` assumes the next alias in sequence and is **unverified**; a declined
  invasion may not notify the invader.
- **Four opcodes are unidentified**: `0x0387`, `0x0388`, `0x038A`, `0x0390`.
- **The death counter's scope is assumed world-wide.** `RequestGetTotalDeathCount` (`0x03F0`)
  carries a zero-byte payload — no area, no character, no scope of any kind — so a single global
  total is the only thing it can be asking for. That is an inference from the request being
  empty, not a confirmed reading of what the client draws.
- **The UDP `msg_index` byte order** looks big-endian where TCP is little-endian. It round-trips
  correctly because the server echoes the same bytes, so it is cosmetic — until something
  depends on ordering.
- **~~Login reply address bug~~ FIXED.** It was the same class as the 56-byte struct: the PS3
  client parses `RequestQueryLoginServerInfoResponse` as all-varint fields with no string, so
  our protobuf string was skipped and the address stayed zero. A second machine could not
  connect at all; the first only worked because RPCS3 rescues `0.0.0.0` to `127.0.0.1`.

## The id-reuse trap, which has bitten once

Clients cache state keyed by **server-assigned ids, across sessions**. Reusing an id makes a
client apply stale state to new content.

This surfaced when blood messages moved to SQLite and numbering restarted at 1: a fresh message
was handed an id a client had already rated, so the client greyed out its rate option for a
message it had never seen — and never sent the evaluate at all. It looked exactly like a server
bug.

Message and sign ids now start at 100000 and never repeat. **Player and character ids do not yet
have this protection.**

## Suggested next steps

1. Verify a *declined* invasion, which is the one BreakIn path still resting on an unverified
   alias guess.
2. Visitors and quick match, which follow the same brokering shape as signs and invasions. Their
   push aliases are unmapped, but the invasion result suggests the lowest value in each
   registration group is a reasonable first guess.
3. Persist players and characters, both for continuity and to close the id-reuse hazard.
4. Consume the player status blob, which is what every matchmaking filter needs.
