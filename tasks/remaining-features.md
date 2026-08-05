# Remaining features

> A per-opcode catalogue of everything still unimplemented, grouped by category with short
> summaries, is in **`tasks/remaining-opcodes.md`**. This file is the prioritised plan; that one
> is the reference.

Derived from `docs/protocol-map-ps3.md` (decompilation-derived, authoritative for BLUS41045)
cross-referenced against `ref/ds3os`, and against what `internal/server/game/boot.go` actually
dispatches.

**53 of 95 live opcodes are implemented.** Everything below is present in the retail client and
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

**`RequestNotifyRingBell` (`0x03EE`) deserves attention beyond logging** — it is the best-named
candidate for the event-item chest trigger (see `tasks/calibration-reverse-engineering.md`).
Log its full payload before deciding what it does.

## 3. Player characters  — needed before anything reads other players

| Opcode | Message | Kind |
|---|---|---|
| `0x03A9` | `RequestGetPlayerCharacter` | R/R |
| `0x03B5` | `RequestGetPlayerCharacterList` | R/R |

We already accept `RequestUpdatePlayerCharacter` (`0x03A8`) and `RequestUpdatePlayerStatus`
(`0x03B8`) and discard both. ds3os's `DS2_PlayerDataManager` (10 handlers) is the reference.

**Blocked on persistence.** Players and characters are still per-run and in memory, and the
id-reuse hazard documented in `docs/STATUS.md` becomes real the moment another client caches a
character id. Persist these first.

## 4. ~~Visitors~~ — DONE (2026-08-05), push ids UNVERIFIED

| Opcode | Message |
|---|---|
| `0x03D5` | `RequestGetVisitorList` |
| `0x03D6` | `RequestVisit` |
| `0x03D7` | `RequestRejectVisit` |
| `0x03C9`–`0x03D1` | Visitor push block (9 aliases, unassigned) |

Implemented in `internal/server/game/visitor.go` — the covenant auto-summon systems (Bell
Keepers, Rat King, Blue Sentinels). Structurally the invasion flow, not the sign flow: nothing is
stored, the server brokers between two live sessions and steps out.

**The push ids remain the open risk.** Nine aliases across `0x03C9`-`0x03D1` for three message
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

## 5. Quick match  — the largest remaining mode

| Opcode | Message |
|---|---|
| `0x03D9`–`0x03DE` | Register / Unregister / Update / Search / Join / Reject |
| `0x03E0`–`0x03E7` | QuickMatch push block (8 aliases, unassigned) |

ds3os `DS2_QuickMatchManager`, 12 handlers / 382 lines. This is the arena (Undead Match). Needs a
match-session concept we do not have yet.

## 6. ~~Mirror Knight~~ — DONE (2026-08-05)

| Opcode | Message |
|---|---|
| `0x039E`–`0x03A4` | Create / Update / Remove / GetList / Summon / Reject |
| `0x03A5`–`0x03A7` | Summon / Reject / Remove pushes |
| `0x03D8` | `RequestNotifyMirrorKnight` (M) |

Implemented in `internal/server/game/mirrorknight.go`. Reuses `signStore` with a **disjoint id
range** (`firstMirrorKnightSignID = 500000`) — both stores seed independently, so sharing the
range would hand an arena sign and a summon sign the same id, and the client caches by sign id
without distinguishing the two systems.

The structural difference from ordinary signs: **no placement**. `RequestCreateMirrorKnightSign`
carries no area or cell, so listing cannot filter by position. `SignData` still requires
`online_area_id`, `cell_id` and `sign_type`, so those go out as zero — absent required fields
would be rejected outright.

Not yet confirmed in-game: needs two clients at Belfry Sol.

## 7. Power stone ranking

| Opcode | Message |
|---|---|
| `0x03F3` | `RequestRegisterPowerStoneData` |
| `0x03F4` | `RequestGetPowerStoneRanking` |
| `0x03F5` | `RequestGetPowerStoneMyRanking` |
| `0x03F8` | `RequestGetPowerStoneRankingRecordCount` |

ds3os `DS2_RankingManager`, 8 handlers. Needs a persisted ranking table; otherwise self-contained.

## 8. Pushes we never send

| Opcode | Message | Note |
|---|---|---|
| `0x038B` | `RegulationFileUpdatePushMessage` | **Parked** — see `tasks/calibration-reverse-engineering.md`. Confirmed to parse; never shown to apply. |
| `0x038C` | `PlayerInfoUploadConfigPushMessage` | Never sent. ds3os sends it at login to configure what the client uploads. A candidate for the chest trigger. |
| `0x03EF` | session-disconnect push | Would let the server evict a client cleanly instead of letting it time out. |

---

## Known-unknowns worth keeping in view

- **Four opcodes are unidentified**: `0x0387`, `0x0388`, `0x038A`, `0x0390` (the last is
  NRLogging-related). None have been observed live.
- **`pushBreakInRejected` is unverified** — it assumes the alias immediately after
  `breakInPushID`. A *declined* invasion may not notify the invader. Cheap to test.
- **Six opcodes must never be implemented**: `0x03FA`–`0x03FD`, `0x03FF`, `0x0400` are in the
  PC/SOTFS map but have no code at all in this binary. See the warning at the top of
  `docs/protocol-map-ps3.md`.
