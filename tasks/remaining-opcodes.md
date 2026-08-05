# Catalogue of unimplemented opcodes

Computed from `docs/protocol-map-ps3.md` (decompilation-derived, authoritative for BLUS41045)
against what `internal/server/game/` actually dispatches or sends. Not from the PC map, and not
from `ref/ds3os` — both describe messages this client does not contain.

**59 opcodes are dispatched or emitted.** What follows is everything else, by category.

A note on counting: three subsystems register many more push opcodes than they have message
types (BreakIn 16 for 4, Visitor 9 for 3, QuickMatch 8 for 4). Those extra values are *aliases of
message types we may already handle*, not separate features, so they are catalogued separately
from real gaps.

---

## A. Real features not built — 6 opcodes

### A1. Quick match — 6 request/response + 8 push aliases

The Undead Match arena at the Brotherhood of Blood. The largest remaining mode, and the only one
needing a concept we do not have: a match session with a lifecycle, rather than a broker that
introduces two players and steps out.

| Opcode | Message | Summary |
|---|---|---|
| `0x03D9` | `RequestRegisterQuickMatch` | Player enters the queue for an arena bracket |
| `0x03DA` | `RequestUnregisterQuickMatch` | Player leaves the queue |
| `0x03DB` | `RequestUpdateQuickMatch` | Queue keepalive / state refresh |
| `0x03DC` | `RequestSearchQuickMatch` | Find joinable matches |
| `0x03DD` | `RequestJoinQuickMatch` | Join a found match |
| `0x03DE` | `RequestRejectQuickMatch` | Decline a join |
| `0x03E0`–`0x03E7` | QuickMatch push block | 4 message types × 2 aliases, **interleaved**: odds `0x3E1,0x3E3,0x3E5,0x3E7` registered first, then evens `0x3E0,0x3E2,0x3E4,0x3E6` |

Reference: ds3os `DS2_QuickMatchManager`, 12 handlers / 382 lines.

### ~~A2. Power-stone ranking~~ — DONE (2026-08-05)

All four implemented in `internal/server/game/ranking.go`, persisted in `power_stone_rankings`.
Ranks derived on read; `offset` is 1-based; submissions are increments and are bounded.

### ~~A3. Player-character reads + persistence~~ — DONE (2026-08-05)

Both implemented, and players/characters/status are now persisted. Player ids are stable per PSN
account across restarts; characters are keyed `(player_id, character_id)` because `character_id`
is the client's local slot number.

Note the PC map is wrong here: it puts `RequestGetPlayerCharacterList` at `0x03A1`, which on PS3
is `RequestGetMirrorKnightSignList`.

---

## B. Pushes the server never sends — 3 opcodes

| Opcode | Message | Summary |
|---|---|---|
| `0x038B` | `RegulationFileUpdatePushMessage` | **Parked.** Confirmed to parse (handler `0x158B150`, `ParseFromArray` at `0x158B1D0`) but never shown to *apply* — no param reload or file write was reached. Carries `start_at`/`end_at`, the shape of time-windowed content. See `tasks/calibration-reverse-engineering.md`. |
| `0x038C` | `PlayerInfoUploadConfigPushMessage` | ds3os sends this at login to configure what telemetry the client uploads. Never sent by us. A candidate for the event-chest trigger. |
| `0x03EF` | session-disconnect push | Would let the server evict a client cleanly rather than waiting out the 60s idle timeout. |

---

## C. Unidentified — 4 opcodes

Present in the binary, no message type recovered, and **none has ever been observed live** from a
real client. Nothing to build until one appears in a capture.

| Opcode | What is known |
|---|---|
| `0x0387` | Nothing beyond existing |
| `0x0388` | Nothing beyond existing |
| `0x038A` | Nothing beyond existing |
| `0x0390` | NRLogging-related |

---

## D. Unused push aliases — 28 values, but not 28 features

These belong to message types we already send. Each subsystem registers several opcodes per
message type, and static analysis could not say which alias maps to which: every registration
site loads the same callback vtable, and the distinguishing state passes through the callback
object at runtime.

| Block | Range | Registered | We use | Unused |
|---|---|---|---|---|
| BreakIn | `0x03B9`–`0x03C8` | 16 for 4 types | `0x03B9` (target, **confirmed live**), `0x03BA` (reject, unverified) | 14 |
| Visitor | `0x03C9`–`0x03D1` | 9 for 3 types | `0x03CF`/`0x03D0`/`0x03D1` (unverified) | 6 |
| QuickMatch | `0x03E0`–`0x03E7` | 8 for 4 types | none | 8 |

BreakIn registration order, in groups of four (call-site order, from the disassembly):

```
group 1: 0x3BD 0x3BE 0x3C0 0x3BF
group 2: 0x3C1 0x3C2 0x3C4 0x3C3
group 3: 0x3B9 0x3BA 0x3BC 0x3BB   <- BreakInTarget (0x3B9 confirmed live)
group 4: 0x3C5 0x3C6 0x3C8 0x3C7
```

Groups 1, 2 and 4 map to reject/allow/remove in an unknown order. `pushBreakInRejected` assumes
`breakInPushID + 1`, which is **not confirmed** — a declined invasion may not notify the invader,
and that is a cheap live test.

---

## E. Never implement — 6 opcodes

`0x03FA`, `0x03FB`, `0x03FC`, `0x03FD`, `0x03FF`, `0x0400`.

Listed as DS2 opcodes by the PC/SOTFS map, so anyone working from that map implements all six by
default. **This client contains no code for any of them**: no `li r4` with those values exists
anywhere in `.text`, and the maximum opcode across all 132 send/register call sites is `0x03F9`.
A server that sends `0x03FB` will simply never be dispatched.

`internal/server/game/opcodes_test.go` fails the build if any of these is ever dispatched, and
also asserts every dispatched opcode appears as present in the PS3 map.

---

## Suggested order

1. **Persistence for players/characters**, then the two character reads. This unblocks
   matchmaking filters too, since nothing currently consumes the status blob.
2. **Quick match** — needs the match-session concept, which would also let
   `PushRequestRemoveVisitor` be sent properly.
2. **Verify the unverified push ids** with live tests: a declined invasion, and a visit. Both are
   minutes of testing and would retire real ambiguity.
3. **Consume the status blob** for matchmaking filters — soul memory, area and covenant all
   arrive in it and are now persisted, but nothing reads them, so every listing still offers
   every online player.
