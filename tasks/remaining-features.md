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

**Confirmed at BOTH rat zones** — Grave of Saints and Doors of Pharros. With filtering on, the
target list flipped from `returned=0 skipped_wrong_pool=1` to `returned=1` in the same poll that
the prey entered the summonable zone, and the summon went straight to a joined session on the first
attempt. **Zero rejections since the fix**, against 54 of 55 before it — the clearest evidence that
we had been offering targets that were always going to refuse.

Activity-area cells, both captured on the wire rather than guessed:

| Zone | Activity cell | Map |
|---|---|---|
| Doors of Pharros | `103320` | m10_33 |
| Grave of Saints | `103410` | m10_34 |

**The reference server's label for `103410` is wrong** — it carries only that one constant and
calls it Doors of Pharros. The value was right, so nothing broke, but the name would have sent a
search into the wrong zone. Third confirmed place the reference is wrong for PS3.

Covenant ids **5 (Rat King)** and **6 (Bell Keepers)** are likewise confirmed, read off live status
blobs for players known to be in them. The rest of that enum is still guessed.

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

### Break-in (invasion) filters

Applied 2026-08-05 after Dark Chasm of Old invasions failed instantly with "unable to find a world
to invade". The list was returning a target and the **host's client was refusing it in ~116ms**;
we relayed that refusal, and the invader read it as nobody being found.

**CONFIRMED LIVE** after the fix: `returned=1` with all three skip counters at zero, push `0x03BD`,
guest joined, session kind **7**. The client queried cell `400330` — the chasm the invader was
standing in — which also disproves an earlier guess here that the client only shops the two chasms
it is *not* in. It queries all three; we had merely sampled two.

**The cause is invadability, not location.** A player cannot be broken into while resting at a
bonfire, while their activity area is 0, or **after burning a Human Effigy** — DS2's deliberate
opt-out of invasion, which we had been ignoring entirely. Symptom to remember: the invasion becomes
possible the moment the host's state changes, with nothing changing server-side.

**RESOLVED: we were telling the host the wrong location.** The push echoed the *invader's* requested
`cell_id`, so a host standing in `400330` was told it was being invaded in `400310`. It compared
that against where it actually was, disagreed, and refused in ~100ms — read by the invader as
"unable to find a world to invade". The push now carries the **host's** area and cell, since the
invader travels to the host.

**DONE — Dark Chasm of Old invasions are fully working and signed off (2026-08-05).**

**CONFIRMED LIVE across the full matrix: 7 invasions, 7 joins, 0 rejections.** All three cells
exercised as both the requested and the pushed value, same-cell and cross-cell:

| requested | pushed | result |
|---|---|---|
| `400320` | `400330` | joined |
| `400310` | `400310` | joined |
| `400330` | `400320` | joined |
| `400320` | `400320` | joined |

One orb reaching a host in a different chasm is the documented behaviour and had never worked
before. Join latency is a near-constant ~14.5s (14.1 / 14.6 / 14.7 observed), so it is a fixed
client-side handshake rather than anything load-dependent.

No transport warnings during any of it, which also exercises the send pacing — these sessions carry
the large ghost-list replies that killed a session before the fix.

**A cell FILTER was added twice and was wrong both times.** It hid the symptom by only ever
offering same-cell hosts, but each orb use is a single query for a single cell and the client does
not keep searching within a use — so a miss burned the attempt and the player retried until the
assigned cell happened to match. That directly contradicts "a Cracked Red Eye Orb can invade any of
the three Chasms regardless of which one you're in".

The client does cycle cells across attempts, and its own cell is in the rotation — verified with
both players at `400330`, where the invader queried `400310`, `400310`, then `400330`. An earlier
note claiming the client "excludes the chasm it is standing in" was invented from a four-query
sample and is wrong.

The list logs `available_cells` beside the queried `cell_id`, and the push logs `requested_cell`
and `pushed_cell` side by side — so if a host ever refuses while those two differ, the cell echo
was not the cause and this whole explanation needs revisiting.

**An empty list produces an instant in-game failure, and that is the CLIENT.** When `returned=0`
the client never sends `RequestBreakInTarget` at all; it prints "unable to find a world to invade"
by itself. We answer list requests in 1–2ms and have no way to make it wait. Any "searching…"
period would have to be the client's, and this one does not have it for an empty list.

Worth recording as a process note: the invadability gate and the location rule shipped together,
and the improvement was initially credited to the wrong one. Two filters in one deploy cannot be
told apart by a single test.

**OPEN: at least one invadability gate is still unmodelled, and it could not be reproduced.**
At 21:29:04 the host's client refused an invasion while every check we have passed
(`skipped_not_invadable=0`, not at a bonfire, no burnt effigy, activity area non-zero). Walking out
of a particular spot made it work. Field-level status logging was added specifically to catch this,
but by the time it was deployed the behaviour had stopped happening and repeated attempts could not
bring it back — so we have no capture of the failing state and **no identified cause**.

The logging stays in place and is silent until something moves, so a recurrence will be recorded
without anyone having to remember this. If it does recur, the field that flips as invasion becomes
possible is the answer.

Candidates never ruled out: an area transition or fog gate, a post-session cooldown on the host, or
a purely positional rule the client derives from geometry the server never sees. The last would
explain the failure to reproduce and would mean there is nothing for us to model.

**`PlayerLocation.cell_id` is NOT the `cell_id` in matchmaking requests.** It is a large opaque id
(`37749729` observed) in a different space from the 6-digit `online_activity_area_id` (`400330`)
that matching uses. Reading the wrong one would silently match nobody.

Soul memory uses **0 tiers below, 4 above** — invaders reach up, never down. DS2's anti-twink rule,
and the best-evidenced figure in the system, since the Cracked Red Eye Orb was the probe used to
derive the tier table.

**Still not applied to sign lists or quick match** — deliberately. Those work today, their ranges
are less certain, and a wrong filter there would break confirmed features.

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
