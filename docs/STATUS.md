# Status — 2026-08-05 (updated)

Where the project actually is, what is proven versus assumed, and what to pick up next.
Complements `RECOVERED_PLAN.md`, which is the original plan and is now partly overtaken.

## Headline

**Two real Dark Souls 2 PS3 clients on separate machines play together against this server**, and
every named multiplayer mode now has an implementation.

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
| World death counter (4 opcodes) | **Working** | Counts live in-game; persists across restarts |
| Management text push (`0x0389`) | **Working** | Renders as the post-login banner, upper left |
| HTTP bootstrap (manifest + payload) | Working | Both files served and length-verified live |
| Mirror Knight (7 opcodes) | **Working** | Two full fights completed live, both outcomes |
| Reliable-UDP under load | **Working** | 3-minute fight, no stalls; NACK-driven retransmit |
| Visitors (3 opcodes) | Implemented | Unit-tested; **push ids unverified** |
| Duelling arenas (6 opcodes) | **Working** | A full duel completed at Undead Purgatory |
| Champion's Tablet ranking (4 opcodes) | Implemented | Unit-tested; persisted |
| Keepalive + bandwidth (4 opcodes) | Implemented | `ServerPing` had been going unanswered |
| Telemetry notifies (8 opcodes) | Implemented | Kill/purchase counters persisted |
| **Serving our own calibrations** | **Working** | Client applied our regulation_0107 and 0113; save matches byte for byte |
| Persistence (SQLite) | Working | Messages, counters, **players, characters, rankings** survive restart |
| **Stable player ids** | **Working** | Same PSN account keeps its id across restarts |
| Player status blob | Recorded | Persisted; **nothing consumes it, so no matchmaking filters** |

## Not built

**65 of 95 live opcodes are dispatched or emitted.** Every named multiplayer mode now has an
implementation. What remains is not really features:

- **Matchmaking filters.** The status blob carrying soul memory, area and covenant is persisted
  but unread, so every listing offers every online player. This is the largest functional gap.
- **Three pushes we never send** — `0x038B` regulation update, `0x038C` player-info config,
  `0x03EF` session disconnect.
- **Four unidentified opcodes** — `0x0387`, `0x0388`, `0x038A`, `0x0390`. None observed live.
- **28 unused push aliases**, which belong to message types we already send.

Per-opcode detail is in `tasks/remaining-opcodes.md`; the prioritised plan is in
`tasks/remaining-features.md`; **`docs/features.md` maps every opcode to what a player actually
does in game**, and is the best starting point for anyone new to the protocol.

**Authoring our own calibration payloads now works.** The 256-byte RSA header is solved and
specified in `docs/regulation-format.md`, with a verified reader/writer at
`tools/calibration/calibration.py`. The blocker was a sign convention: the plaintext is
`n - (header^3 mod n)`, not `header^3 mod n`, which is why OAEP, PKCS#1 v1.5 and constant-XOR all
failed. Repacking calibration 0114 reproduces FromSoftware's own `SizeOrg`, `SizeEnc` and
`DIGEST` byte for byte. Delivering an authored payload additionally needs the client patched to
trust our key — `tools/rpcs3/dso.yml`, the calibration key at `0x189AB48` / `0x1910670`.

## Things that cost real time, recorded so they are not re-learned

**The CWC test vectors are the wrong oracle.** `internal/crypto/cwc/testdata/cwc.1` produces
tags the game rejects. The variant the game speaks is the one in the reference C implementation;
matching the published vectors is actively wrong. CP0 being green was false confidence. The
authoritative oracle is now `TestConsoleCapture` (real console bytes) plus `TestGameVectors`
(generated from the C build). The actual bug was byte order in the block load — the reference
byte-swaps `Z` but *not* the data block, so each 4-byte group is reversed relative to a plain
big-endian read. Ciphertext was always correct; only tags differed.

**The EBOOT holds two RSA keys and only one is the login key.** On v1.00, `0x17FB338` is the login
key (byte-identical to the PC one) and `0x189AB48` is the **calibration** key — it verifies the
`contents_NNNN.bin` header, loaded by the request builder at `0x01673D5C` from mini-TOC slot
`0x1CC27E8`. Independently corroborated: the UTF-16 `Patch.List.*` format strings sit immediately
after it in rodata. Patching the wrong one gives a client that connects but whose RSA block never
decrypts.

**A title update moves the keys and invalidates the patch, silently.** RPCS3 patches are keyed by
PPU executable hash, so a launch-disc patch simply stops applying after an update — no emulator
warning, just `decrypt: crypto/rsa: decryption error` on our side and a game that hangs on login.
The keys themselves do not change; only their addresses do (v1.10: login `0x186E100`, calibration
`0x1910670`). `tools/rpcs3/dso.yml` carries a block per version and documents how to add more.

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

**A server restart invalidates every auth token.** The registry is in-memory, so a client
mid-session will spin on `unknown or expired auth token`. Back out to the menu and re-enter
online mode rather than retrying in place.

This is worse than an inconvenience during testing: it drops **both** clients at once, and from
the game it looks like a crash in whatever feature happened to be running. It cost a false bug
report against Mirror Knight — the summon had worked perfectly two minutes earlier and a deploy
landed underneath the players. **Use `./restart.sh`**, which refuses to restart while any client
has been active in the last 90 seconds (`--force` to override, `--check` to just look).

**Run the server from WSL, not Windows.** Windows cannot bind TCP on the LAN address for any
port; WSL shares the same IP and works. See the `dso-run-server-from-wsl` note.

## Delivering a calibration, end to end

**This works.** A real client was moved from calibration 1.01 to 1.06 by serving our own payload,
and the copy it stored is byte-identical to our `regulation_0107` (sha1 `1c672626`). The loop:

1. `DSO_CALIBRATION_VERSION=NNNN` answers the hardcoded `contents_0101.bin` request with that
   version's manifest.
2. The client reads the regulation filename out of the manifest and fetches it by name.
3. It stores the result in save data, **not** on `dev_hdd0`: slot `15USER.DAT` (1 MiB fixed),
   backed up to `115USER.DAT`. Format is an 8-byte header (u32 compressed size, u32 uncompressed
   size, both big-endian) followed by a raw zlib stream of the BND4.

Three things that make it look broken when it is not:

- **It only checks on a cold start from the dashboard**, or roughly every two hours online.
  Re-entering online mode is not enough; the game must be relaunched.
- **The commit needs a clean exit.** The game deletes `15USER.DAT` before writing the new one, so
  a crash or a killed emulator leaves the slot empty and the old calibration in the backup. Both
  earlier failures looked like "it didn't download" when the download had in fact completed.
- **0104 reports the same version as 0101** (`00010100`), so applying it changes nothing visible
  even though it alters seven params.

Version stamps, read from BND4+24: disc `00010000`, 0101/0104 `00010100`, 0107 `00010600`,
0110 `00010810`, 0114 `01001500`.

**Known ceiling:** 0114 downloads in full and then crashes RPCS3 on a `BLUS41045_v01.00` install.
It is an April 2015 payload in a different stamp format and expects a matching title update.
Nothing is written when it fails, so the install survives.

**The event-item chest is NOT a calibration problem.** Calibration 0114 — the only payload that
changes `ItemLotParam2_SvrEvent.param`, which is byte-identical across 0101-0113 — installed
successfully on a v1.10 client and the chest was still empty. The lots are present in the
client's regulation and nothing appears, so something must *select* an active lot and that
selection is not in the regulation. Candidates are all server-side: an event flag, an
unimplemented opcode (`RequestNotifyRingBell` `0x03EE` is suggestively named), or the
`PlayerInfoUploadConfigPushMessage` push `0x038C` that we never send.

## The random-disconnect bug, and what it really was

**FIXED AND CONFIRMED LIVE (2026-08-05).** Two full Mirror Knight fights completed back to back —
one ending in the host's death, one in the boss's — with zero pump failures and zero retransmit
stalls. The second ran just under three minutes, which is the same duration band in which the
previous two attempts died, so it is a real test rather than a short one.

Sessions had been dying mid-fight with `rudp: connection died (max retransmits)` while the client
was still actively sending messages seconds earlier. Three independent faults, all now fixed.

**0. RACK is a NACK, and ignoring it was the main fault.** The reference calls it "Reject ACK",
says it is "95% sure" that is what it means, and ignores it. The PS3 binary says otherwise: RACK
is a periodic *rejected-packet statistics report* from the client's transmit pump (EBOOT
`0xEA8684` v1.10), carrying a u32 count and u32 byte total, emitted whenever its rejected-bytes
counter is non-zero.

It is the protocol's only NACK, and the situation it reports is unrecoverable by blind
retransmission:

- The client rejects any DAT whose sequence is not exactly `RemoteSequenceIndex + 1` — a **gap**.
- While that gap is open it is **structurally unable to acknowledge anything**: its ack scheduler
  fires only when its last-sent ack differs from its receive index, and the receive index cannot
  advance past a hole. That is the observed "never acknowledges again".
- A retransmission of a sequence it has **already buffered** is discarded in total silence — no
  ack, no RACK, no counter.

So the only packet that can restart the connection is `their_ack + 1`. We were retransmitting the
head of our buffer instead, and spending the whole 32-second budget on packets the peer threw
away without a word. Retransmission now always targets the peer's actual hole, RACK triggers an
immediate fast retransmit, and RACK's ack is consumed — it and ACK are the only opcodes whose ack
counters the client populates when it has nothing new to report, which is why captured RACKs read
`their_ack=1381` while the DATs around them read 0.

Verified identical in v1.00 and v1.10 by byte-diffing the five relevant functions with the region
deltas applied: every differing word is a TOC-relative load or a long branch, zero logic changes.

**1. The ack comparison had a hole at the sequence wrap.** Sequence numbers are 12 bits and wrap
at 4096; the comparison special-cased a wrap only when the current ack sat in the top quarter:

```go
if acked > topQuarter && incoming < bottomQuarter { accept }
else if incoming > acked { accept }
```

If a single ack was lost near the boundary — acked stuck at, say, 3000, next observed ack 5 —
*neither arm fired*. The ack was discarded, every packet then looked unacknowledged, and the
session died on max retransmits about 32 seconds later (160 attempts × 200ms, not the 160s the
constant name suggests). With 4096 sequence numbers and fragmented 1.5 KB ghost uploads, wraps are
frequent, which is why this looked random. Replaced with ordinary modular comparison —
`internal/frpg/rudp/sequence.go`, with wrap tests.

**2. A dropped session was unrecoverable.** Auth tokens have a 30-second TTL refreshed on lookup,
but an established session never looks its token up again — so the token expired 30s in. Harmless
while the session held; fatal once it dropped, because the reconnecting client found its token
gone and sat on `unknown or expired auth token` until it backed out and re-authenticated. The
pump now refreshes the token for every live session, so a token lives exactly as long as its
session.

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
- **~~Where `ManagementTextMessage` renders~~ RESOLVED.** Push `0x0389` draws the **banner in the
  upper left, immediately after login** — confirmed live on 2026-08-05. It is NOT the obelisk.
  Static analysis could not reach this: the listener sits at manager+72 but the code installing it
  was never found. Second time a live test beat decompilation, after the push transport itself.
- **~~The Majula obelisk~~ SOLVED, and it is not a push at all.** Its text is **string id 100 in
  `regulation<Language>.fmg`**, one of eleven per-language files inside the regulation archive,
  each holding exactly that one string. English reads "The letters are worn beyond recognition.";
  every language ships its own. That explains the Portuguese release shipping a different line —
  it is a localized game-data string, not server text. To change it the server must deliver a
  modified regulation, which makes the `0x038B` regulation push the route to the obelisk *and*
  to the event items. The string is byte-identical across all ten published calibrations, so
  FromSoftware never actually used it in these payloads.
- **The `0x038B` regulation push parses but may not apply.** Confirmed: the client special-cases
  it, constructs `RegulationFileUpdatePushMessage` and calls `ParseFromArray`. Unproven: that
  anything downstream consumes it — no param reload or file write was reached. Its
  `RegulationFileDiffData` carries `start_at`/`end_at`, which is exactly the shape of a
  time-windowed content rotation, so this is the likely event-item channel. Note the field
  *numbers* are unrecoverable from the string table and would have to come from tag immediates.
- **~~The death counter's scope~~ RESOLVED.** `RequestGetTotalDeathCount` (`0x03F0`) carries a
  zero-byte payload — no area, no character, no scope — so a single global total was the only
  reading available. Confirmed in-game: the client labels it **"deaths worldwide"**. The reply
  must be sent; the unanswered retry was the original timeout.
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

Message and sign ids start at 100000 and never repeat. **Players are now persisted too**: ids are
`AUTOINCREMENT` from 100000, so they are never reused even after deletes, and the same PSN account
always resolves to the same id.

`character_id` is different in kind — it is the **client's local slot number**, not a
server-assigned id, so every player has a character 1. Anything keyed by character must therefore
be keyed `(player_id, character_id)`. Getting that wrong is not hypothetical: the leaderboard
shipped keyed on `character_id` alone and would have merged every player's first character into a
single board entry.

## Suggested next steps

The build-out phase is essentially over; what is left is verification and depth.

1. **Live-test the three unverified push paths.** Each is minutes of play and retires real
   ambiguity that no amount of static analysis has settled:
   - a *declined* invasion (`pushBreakInRejected` assumes `breakInPushID + 1`)
   - a covenant auto-summon (the visitor push trio is an inference)
   - a Mirror Knight summon and an arena duel, neither of which has ever run
2. **Consume the status blob for matchmaking.** Soul memory, area and covenant all arrive and are
   persisted; nothing reads them, so every listing offers every online player. This is the
   difference between "the mode works" and "the mode works correctly".
3. **Fill the event chest by binding a lot to a result event that already fires.**
   `ResultEventParam.param` is the selector: each of its 82 rows carries an
   `ItemLotParam2_SvrEvent` lot id at `+0x0C`, and the 11xxx lot ids appear nowhere else in the
   archive. Calibration 0114 bound its new lots to rows 1200/2400/1100/1300, which had *zero*
   before — so the chest wiring shipped complete and the chest stayed empty because **the result
   event never fires**, not because data was missing. Now that we can author payloads, the cheap
   experiment is to bind a lot to a row known to fire and see whether the chest fills. What
   actually fires a result event is still unknown; it needs the param registry at `0x1C85BC4` /
   `0x1C85BC8` followed into `MapObjSvrEventTreasureBoxComponent` (EBOOT `0x17F6CC8`).
4. **Capture `0x0387`/`0x0388`/`0x038A`.** Patch 1.10 added population hints to the warp screen —
   server-supplied, no known opcode, and these three are emitted early in boot and never seen.
   Warping on 1.10 with full logging may catch them.
