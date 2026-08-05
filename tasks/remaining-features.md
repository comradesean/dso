# Remaining features

Derived from `docs/protocol-map-ps3.md` (decompilation-derived, authoritative for BLUS41045)
cross-referenced against `ref/ds3os`, and against what `internal/server/game/boot.go` actually
dispatches.

**33 of 95 live opcodes are implemented.** Everything below is present in the retail client and
currently unanswered. An unanswered request/response opcode is not harmless: the client retries
silently and **will not open other online UI while one is outstanding**, which is how several
"broken menu" symptoms were eventually explained.

Ordering below is roughly by value-for-effort.

---

## 1. Connection keepalive and bandwidth  — small, and possibly load-bearing

| Opcode | Message | Kind |
|---|---|---|
| `0x038D` | `ServerPing` | R/R |
| `0x038E` | `RequestMeasureUploadBandwidth` | R/R |
| `0x038F` | `RequestMeasureDownloadBandwidth` | R/R |
| `0x03B7` | `RequestBenchmarkThroughput` | R/R |

ds3os answers the bandwidth pair with a fixed-size payload and treats ping as a liveness echo.
Worth doing first: tiny, and an unanswered `ServerPing` is a plausible cause of the periodic
disconnects that have not been investigated.

## 2. Telemetry notifies  — trivial, and they unblock diagnosis

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

`RequestNotifyKillEnemy` and `RequestNotifyBuyItem` are the same shape as the death counter and
should reuse the `counters` table.

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

## 4. Visitors  — a whole multiplayer mode, and the machinery already exists

| Opcode | Message |
|---|---|
| `0x03D5` | `RequestGetVisitorList` |
| `0x03D6` | `RequestVisit` |
| `0x03D7` | `RequestRejectVisit` |
| `0x03C9`–`0x03D1` | Visitor push block (9 aliases, unassigned) |

Structurally identical to invasions, which work. Reuse `breakin.go` almost wholesale.

**The push aliases are the risk**: nine registrations for three message types, and static analysis
cannot separate them. `0x03B9` was guessed correctly for BreakIn on the first try by taking the
group in registration order — the same approach should be tried here, then confirmed live.

## 5. Quick match  — the largest remaining mode

| Opcode | Message |
|---|---|
| `0x03D9`–`0x03DE` | Register / Unregister / Update / Search / Join / Reject |
| `0x03E0`–`0x03E7` | QuickMatch push block (8 aliases, unassigned) |

ds3os `DS2_QuickMatchManager`, 12 handlers / 382 lines. This is the arena (Undead Match). Needs a
match-session concept we do not have yet.

## 6. Mirror Knight  — a clean sign-manager clone

| Opcode | Message |
|---|---|
| `0x039E`–`0x03A4` | Create / Update / Remove / GetList / Summon / Reject |
| `0x03A5`–`0x03A7` | Summon / Reject / Remove pushes |
| `0x03D8` | `RequestNotifyMirrorKnight` (M) |

Mirrors `sign.go` almost exactly — ds3os's `DS2_MirrorKnightManager` is largely a copy of its sign
manager. Unlike the Visitor and QuickMatch blocks, **these three push ids are individually
identified in the map**, so there is no alias guessing.

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
