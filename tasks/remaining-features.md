# Remaining features

> A per-opcode catalogue of everything still unimplemented, grouped by category with short
> summaries, is in **`tasks/remaining-opcodes.md`**. This file is the prioritised plan; that one
> is the reference.

Derived from `docs/protocol-map-ps3.md` (decompilation-derived, authoritative for BLUS41045)
cross-referenced against `ref/ds3os`, and against what `internal/server/game/boot.go` actually
dispatches.

**65 of 95 live opcodes are implemented.** Everything below is present in the retail client and
currently unanswered. An unanswered request/response opcode is not harmless: the client retries
silently and **will not open other online UI while one is outstanding**, which is how several
"broken menu" symptoms were eventually explained.

Ordering below is roughly by value-for-effort.

---

## 1. ~~Connection keepalive and bandwidth~~ — DONE (2026-08-05)

| Opcode | Message | Kind |
|---|---|---|
| `0x038D` | `ServerPing` | R/R |
| `0x038E` | `RequestMeasureUploadBandwidth` | R/R |
| `0x038F` | `RequestMeasureDownloadBandwidth` | R/R |
| `0x03B7` | `RequestBenchmarkThroughput` | R/R |

Implemented in `internal/server/game/connection.go`. **ds3os does not implement any of these for
DS2**, so there was no reference to copy — but all eight messages involved (request and response)
are defined with no fields at all, so an empty reply is the complete answer rather than a stub.

An unanswered `ServerPing` remains a plausible cause of the periodic disconnects that have never
been investigated; now that it is answered, that hypothesis is testable by simply leaving a
client connected.

## 2. ~~Telemetry notifies~~ — DONE (2026-08-05)

All fire-and-forget (`M`): no reply, but they must be marked handled in `handledOpcodes` or they
pollute the "no handler" log that is our main discovery signal.

| Opcode | Message |
|---|---|
| `0x03E8` / `0x03E9` | `RequestNotifyJoinGuestPlayer` / `LeaveGuestPlayer` |
| `0x03EA` / `0x03EB` | `RequestNotifyJoinSession` / `LeaveSession` |
| `0x03EE` | `RequestNotifyRingBell` |
| `0x03F6` | `RequestNotifyKillEnemy` |
| `0x03F7` | `RequestNotifyBuyItem` |
| `0x03F9` | `RequestNotifyDisconnectSession` |

Implemented in `internal/server/game/telemetry.go`. `RequestNotifyKillEnemy` and
`RequestNotifyBuyItem` feed the `counters` table alongside the death counter, reusing its clamp on
client-supplied counts.

**`RequestNotifyRingBell` (`0x03EE`) is NOT the event-chest trigger** — that hypothesis is dead.
DS2 has exactly two bells (Belfry Luna, Belfry Sol) and each is a lever that opens a gate.
Ringing does not trigger or affect Bell Keeper invasions, which fire on the host's presence in the
belfry, and the Majula mansion has no bell at all. DS2 dropped DS1's persistent bell-flag model.
It is almost certainly plain telemetry, in the same family as `NotifyKillEnemy`/`NotifyBuyItem`.
See `docs/features.md`.

## 3. ~~Player characters + persistence~~ — DONE (2026-08-05)

| Opcode | Message | Kind |
|---|---|---|
| `0x03A9` | `RequestGetPlayerCharacter` | R/R |
| `0x03B5` | `RequestGetPlayerCharacterList` | R/R |

Players and characters are now persisted, and **player ids are stable per PSN account** across
restarts. That retires the id-reuse hazard for players: ids are `AUTOINCREMENT` from 100000, so
they are never reused even after deletes, and the same account always resolves to the same id.

`character_id` is the **client's local slot number**, not a global id — every player has a
character 1 — so characters are keyed `(player_id, character_id)`. Finding that also fixed a live
bug in the leaderboard, which had been keyed on `character_id` alone and would have merged every
player's first character into one board entry.

Slot allocation now consults the store as well as the ids the client volunteers: the client only
knows its own local slots, so allocating from that alone could hand back an id already recorded
for this player and silently merge two characters.

`RequestGetPlayerCharacterList`'s response message has no fields at all, so an empty reply is the
complete answer. Its request shape reads more like an update than a query and is logged in full
pending a capture.

## 4. ~~Visitors~~ — DONE and CONFIRMED LIVE (2026-08-05)

| Opcode | Message |
|---|---|
| `0x03D5` | `RequestGetVisitorList` |
| `0x03D6` | `RequestVisit` |
| `0x03D7` | `RequestRejectVisit` |
| `0x03C9`–`0x03D1` | Visitor push block (9 aliases, unassigned) |

Implemented in `internal/server/game/visitor.go` — the covenant auto-summon systems (Bell
Keepers, Rat King, Blue Sentinels). Structurally the invasion flow, not the sign flow: nothing is
stored, the server brokers between two live sessions and steps out.

**Push ids confirmed live for two of three covenants.** A Bell Keeper summon pushed `0x03CC`,
exactly `0x3C9 + 3*mode + role` for mode 1 role 0, and the host received it and joined; the
rejection path was exercised too (`0x03CD`). On 2026-08-05 the **Rat King** covenant was confirmed
end-to-end as well — `0x03CF` (mode 2, visit) produced a completed session, and `0x03D0` renders
the client's "summoning failed" text. Only Blue Sentinels (`0x03C9`/`0x03CA`) remains untested.

**The Rat King flow inverts the Bell Keeper one.** The covenant member is the HOST, and the victim
is pulled into *their* world as a grey phantom — the opposite of Bell Keepers, where the covenant
member travels. In both cases the covenant member is the one who sends `RequestVisit`, and the
client works out who actually travels from `type`, so the same push shape serves both. Session
kind is 15 for rat prey.

**`reason=2` on a rejection is normal, not an error.** It is client-authored and means the target
is not currently invadable; resting at a bonfire is one such state. Proven twice: an identical
request refused at 18:20:44 was accepted at 18:21:05, and a rat summon refused 15 times while the
target sat at a bonfire succeeded within one poll of them walking away.

**The covenant auto-summon is a client poll on a fixed ~20.5s timer.** It re-asks for as long as
the crest is equipped, so an ineligible target used to be re-offered indefinitely — the player saw
"summoning failed" every twenty seconds. Fixed by matchmaking filters, below.

## 4b. Matchmaking filters — DONE and CONFIRMED LIVE (2026-08-05)

**Confirmed at Grave of Saints.** With filtering on, the target list flipped from `returned=0
skipped_wrong_pool=1` to `returned=1` in the same poll that the prey entered the summonable zone,
and the summon went straight to a joined session on the first attempt. **Zero rejections since the
fix**, against 54 of 55 before it — the clearest evidence that we had been offering targets that
were always going to refuse.

Two behaviours worth recording, both CLIENT-side and neither ours to tune:

- The auto-summon poll runs at ~20s, but backs off **4½–6 minutes after a summon that becomes a
  session** (366s and 276s observed). A summon the prey *escapes* costs nothing: polling resumes at
  the normal 20s. Plausibly anti-farm behaviour.
- Escaping into a safe spot genuinely prevents the summon, and is cheap to do.


`internal/server/game/matchmaking.go`. Target lists are no longer "every online player".

The data was arriving all along: the `AllStatus` blob on the status heartbeat (`0x03B8`) is
**ordinary protobuf, not an opaque payload** — it was simply never parsed. It carries soul memory,
covenant, the equipped covenant seals, and `online_activity_area_id`.

**`online_activity_area_id` is the field that matters.** It is 0 whenever the player is not
somewhere the game hosts sessions — resting at a bonfire is the case we hit — and becomes the cell
id when they are. That single field explains the rat summon that failed 15 consecutive times and
then succeeded with no server change: the target was at a bonfire, then walked into the trap zone.

**The visitor "pool" is a property of the target, and the Rat King one is inverted.** You are
rat-summonable precisely because you are *not* a rat, which the in-game text states outright.
Getting this backwards would break the mode entirely, so it is pinned by test.

**The client sends PARTIAL status updates** — an occasional full blob (~1336 bytes) and a stream of
28–52 byte deltas. The profile must therefore be *merged per field*, using proto2 presence, not
rebuilt per message. The first version rebuilt it, so every delta blanked whatever it did not
mention; players flickered in and out of their pools several times a minute and were never offered
to anyone. It looked exactly like an over-strict filter. Regression test:
`TestPartialStatusDoesNotWipeProfile`.

`MatchingParameter`, which every list request carries and whose fields are all `required`, is used
as a fallback so a player is matchable before their first full status blob arrives.

### Confidence in the numbers

**Patch 1.10 is the Scholar of the First Sin update** for the original game (Feb 2015, free to
vanilla owners). The vanilla/SOTFS distinction therefore mostly collapses for us — the real axis is
pre-1.10 vs 1.10+, and we are 1.10+. Notably 1.10 removed the NG/NG+ matchmaking divide and added
the Agape Ring.

Windows are measured in tiers either side of the **item user's** band, and select who they may
connect to. The reference server builds its window around the other party, which for invasions
inverts DS2's documented "never invade below your soul memory" rule — we take its constants but not
its argument order.

| Piece | Confidence |
|---|---|
| Tier bucket model (not a percentage window) | High — testing explicitly debunked the ±% rumours |
| Tier bands 1–43 | High — every source agrees exactly |
| The 359,999,999 band | Medium — majority reading, no published test, only affects >45M |
| Bell Keeper 1/3, Rat 1/3 | Good, though Rat is flagged as not re-tested after 1.10 |
| Blue Sentinels 5/4 | **Disputed** — sources split 5/4 vs 7/6; untested live |
| Activity-area cell ids | 103410 confirmed on our own wire; the second rat cell is unknown |

Apparent unanimity across community sources is misleading: they descend from about **two** lineages
of black-box testing, not five independent ones, and one widely-copied table is a stale snapshot
with every lower bound off by one.

A previous version of this file claimed the Rat window was authoritative because the covenant item
says so in game. It does not — the item text carries no numbers, and the quoted sentence is wiki
boilerplate repeated across unrelated items. The value stands on testing, not on that.

`DSO_MATCHMAKING_FILTERS=0` disables the lot.

**Not yet applied to sign lists, break-in targets or quick match** — deliberately. Those work
today, their ranges are unknown, and a wrong filter there would break confirmed features.

Historical note on how they were chosen: Nine aliases across `0x03C9`-`0x03D1` for three message
types. We send `0x03CF`/`0x03D0`/`0x03D1`, which is better supported than a guess: the PC protos
assign exactly those to exactly those types, and unlike the BreakIn pushes (PC says
`0x03FB`-`0x03FD`, absent from this binary) the decompilation confirms all three exist here.
Three consecutive values hitting three different types also fits interleaved aliasing, which is
documented for the QuickMatch block.

**If a visit silently does nothing in-game, flip to `0x03C9`/`0x03CC`/`0x03CF`** — the first
alias of each contiguous group, the shape that proved right for BreakIn.

**`PushRequestRemoveVisitor` is deliberately not sent.** Telling a host their visitor left needs
to know which host a departing player was in, and no visit session is tracked. The phantom clears
on the clients' own timeout instead — the natural companion to the visit-session concept
QuickMatch will need anyway.

## 5. ~~Quick match~~ — DONE (2026-08-05)

| Opcode | Message |
|---|---|
| `0x03D9`–`0x03DE` | Register / Unregister / Update / Search / Join / Reject |
| `0x03E0`–`0x03E7` | QuickMatch push block (8 aliases, unassigned) |

Implemented in `internal/server/game/quickmatch.go`. DS2's 1v1 duelling arenas — "Undead Match"
is DS3 terminology and does not apply. `QuickMatchGameMode` names a **venue**, not a format: Blue
(0) is the Cathedral of Blue, Brotherhood (1) is Undead Purgatory.

Unlike every other mode this one is *advertised* rather than brokered, so it keeps state: a player
registers and stays registered until they withdraw or disconnect. Filtering is by area, cell and
mode, which is the whole filter — Soul Memory is ignored in arenas.

Push ids are much better evidenced than the Visitor block: the PC enum's four values
(`0x03E1`/`0x03E3`/`0x03E5`/`0x03E7`) are **all odd**, and the decompilation records the odds as a
complete registration pass. `PushRequestAllowQuickMatch` is deliberately never sent — acceptance
is built by the host's client and tunnelled through the `0x0320` relay, as with invasions.

**Confirmed live** — a full duel completed at Undead Purgatory, push id `0x03E1`, no transport
stalls.

**The three statues are handled.** Each arena has three statues by the bonfire, each a vote for a
different map: on the Brotherhood side a long bridge over a lethal drop, a two-level labyrinth,
and a large circular scaffolded stage. You are matched at your own map when someone is queued
there and paired across maps when nobody is, with the higher covenant rank deciding which map is
used. Matching therefore orders by cell rather than filtering on it — a strict filter left two
players at different statues visible to each other and never able to meet.

**Settled: `cell_id` does NOT carry the statue.** The left and middle statues at Undead Purgatory
both register `cell_id=102350`, so the cell is fixed per venue and the map choice rides elsewhere
— almost certainly inside the opaque `MatchingParameter`. Map selection works correctly in game
regardless, so whatever channel carries it does not involve us. The cross-map ordering is
therefore a no-op in practice, kept because it costs nothing and is right if the map ever does
reach us.

**Rough edge worth watching:** if both players register and neither searches, both sit advertising
and no match forms — seen live as a ~23-second stall that broke as soon as one player re-queued.
The server is passive by design here. Whether the real server actively paired two waiting
registrations is unknown.

**Not implemented:** the higher-ranked player's map choice deciding the venue. Covenant rank is
not tracked, and map selection is most likely negotiated client-to-client after the join.

## 6. ~~Mirror Knight~~ — DONE (2026-08-05)

| Opcode | Message |
|---|---|
| `0x039E`–`0x03A4` | Create / Update / Remove / GetList / Summon / Reject |
| `0x03A5`–`0x03A7` | Summon / Reject / Remove pushes |
| `0x03D8` | `RequestNotifyMirrorKnight` (M) |

The Looking Glass Knight in King's Passage, Drangleic Castle — NOT Belfry Sol. The summoned
player is hostile and fights for the boss, volunteering via a Red Sign Soapstone sign left
anywhere in the castle, which is why the create message carries no area or cell.

Implemented in `internal/server/game/mirrorknight.go`. Reuses `signStore` with a **disjoint id
range** (`firstMirrorKnightSignID = 500000`) — both stores seed independently, so sharing the
range would hand an arena sign and a summon sign the same id, and the client caches by sign id
without distinguishing the two systems.

The structural difference from ordinary signs: **no placement**. `RequestCreateMirrorKnightSign`
carries no area or cell, so listing cannot filter by position. `SignData` still requires
`online_area_id`, `cell_id` and `sign_type`, so those go out as zero — absent required fields
would be rejected outright.

Not yet confirmed in-game: needs two clients at Drangleic Castle's King's Passage.

## 7. ~~Power stone ranking~~ — DONE (2026-08-05)

| Opcode | Message |
|---|---|
| `0x03F3` | `RequestRegisterPowerStoneData` |
| `0x03F4` | `RequestGetPowerStoneRanking` |
| `0x03F5` | `RequestGetPowerStoneMyRanking` |
| `0x03F8` | `RequestGetPowerStoneRankingRecordCount` |

Implemented in `internal/server/game/ranking.go`, persisted in `power_stone_rankings`.

Ranks are **derived on read** rather than stored, so they cannot go stale against the scores they
describe — the reference keeps them as columns and has to maintain them. `serial_rank` is a
unique 1-based position; `rank` is a competition rank where ties share a value.

The client's `offset` is **1-based**, matching the reference. The submission carries an
*increment*, not a total, and is bounded — the board is persistent, so an unvalidated increment
would let one modified client pin the top of it permanently.

**Keyed by `character_id`, which the protocol dictates** (`RequestGetPowerStoneMyRanking` looks up
by character alone). Since character ids are still per-run and in memory, a reused id would
inherit a previous character's score. That is the id-reuse hazard again, and persisting
characters is the fix.

## 8. Pushes we never send

| Opcode | Message | Note |
|---|---|---|
| `0x038B` | `RegulationFileUpdatePushMessage` | **Parked** — see `tasks/calibration-reverse-engineering.md`. Confirmed to parse; never shown to apply. |
| `0x038C` | `PlayerInfoUploadConfigPushMessage` | Never sent. ds3os sends it at login to configure what the client uploads. A candidate for the chest trigger. |
| `0x03EF` | `PushRequestNotifyRingBell` | **Not** the session-disconnect push it was long assumed to be — name read off `GetTypeName`. A server→client bell broadcast the client already has a handler for. See below. |

---

## Known-unknowns worth keeping in view

- **Four opcodes are unidentified**: `0x0387`, `0x0388`, `0x038A`, `0x0390` (the last is
  NRLogging-related). None have been observed live.
- **`pushBreakInRejected` is unverified** — it assumes the alias immediately after
  `breakInPushID`. A *declined* invasion may not notify the invader. Cheap to test.
- **Six opcodes must never be implemented**: `0x03FA`–`0x03FD`, `0x03FF`, `0x0400` are in the
  PC/SOTFS map but have no code at all in this binary. See the warning at the top of
  `docs/protocol-map-ps3.md`.
