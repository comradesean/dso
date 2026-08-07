# Catalogue of unimplemented opcodes

Computed from `docs/protocol-map-ps3.md` (decompilation-derived, authoritative for BLUS41045)
against what `internal/server/game/` actually dispatches or sends. Not from the PC map, and not
from `ref/ds3os` — both describe messages this client does not contain.

**66 opcodes are dispatched, plus the push blocks we emit** (`0x0389`, `0x038B`, `0x03AA`, `0x03EF`,
and the BreakIn / Visitor / QuickMatch alias blocks based at `0x03B9`, `0x03C9`, `0x03E0`).

> **Updated 2026-08-07.** This file had drifted badly: it described quick match as the largest
> remaining mode when it was implemented and duelled live, called `0x03EF` a session-disconnect push
> when it is the belfry bell and is working, and listed six never-implement opcodes when one of them
> is deliberately implemented. Everything below is re-checked against the code.

A note on counting: three subsystems register many more push opcodes than they have message
types (BreakIn 16 for 4, Visitor 9 for 3, QuickMatch 8 for 4). Those extra values are *aliases of
message types we already handle*, not separate features, so they are catalogued separately from
real gaps.

---

## A. Real features not built — none

Every named multiplayer mode has an implementation. The three entries this section used to carry
are all done:

- ~~**A1. Quick match**~~ — **DONE.** `internal/server/game/quickmatch.go` handles all six of
  `0x03D9`–`0x03DE`, and **a full duel completed live at Undead Purgatory**. The push block is
  `quickMatchPushBase + 2*role + (1 - mode)` — mode-minor *and* inverted, which is the detail that
  makes it unlike the other two blocks.
- ~~**A2. Power-stone ranking**~~ — DONE (2026-08-05). `ranking.go`, persisted in
  `power_stone_rankings`. Ranks derived on read; `offset` is 1-based; submissions are increments and
  are bounded.
- ~~**A3. Player-character reads + persistence**~~ — DONE (2026-08-05). Player ids are stable per PSN
  account across restarts; characters are keyed `(player_id, character_id)` because `character_id`
  is the client's local slot number.

  Note the PC map is wrong here: it puts `RequestGetPlayerCharacterList` at `0x03A1`, which on PS3
  is `RequestGetMirrorKnightSignList`.

---

## B. Pushes the server never sends — 1 opcode

| Opcode | Message | Summary |
|---|---|---|
| `0x038C` | `PlayerInfoUploadConfigPushMessage` | **Deliberately not implemented as a feature** — upload scheduling only. Handler `0x15F4580` parses, applies lower bounds (`upload_period >= 5`, `char_data >= 60`, `enemy_kill >= 5`) and makes one virtual call; no field can carry an item, a file or a flag, which is what ruled it out as the event-chest trigger. FromSoftware genuinely sent it, carrying an element-id list. **But see §C — it is the best candidate for what switches the four unidentified opcodes on, and sending it as a probe is a different act from implementing it.** |

Both former entries here are done:

- ~~`0x038B` `RegulationFileUpdatePushMessage`~~ — **DONE, and it drives two features.** Replaces one
  whole resource file in the running client on the next frame, no restart. Confirmed live on both
  routes: params (the Majula event chest paid out) and FMG (the Majula obelisk displays what we
  send). `regulationpush.go`; full trace in `tasks/regulation-push-038b.md`.
- ~~`0x03EF` "session-disconnect push"~~ — **that name was wrong, and it is implemented.** It is
  `PushRequestNotifyRingBell`, the belfry bell broadcast, confirmed live: a player outside the
  belfry and not in the session heard the bell. `telemetry.go`. See `tasks/bell-broadcast.md`.

---

## C. Unidentified — 4 opcodes

`0x0387`, `0x0388`, `0x038A`, `0x0390`. Present in the binary, no message type recovered, and never
observed from a real client.

**They are client→server, not pushes** (established 2026-08-07). Each has a *send site* rather than
a push-handler registration, which is the discriminator — compare `0x0389`/`0x038B`/`0x038C`, which
have dispatcher→handler entries and are marked `P`:

| Opcode | Send site | Note |
|---|---|---|
| `0x0387` | `0x16638F4` | ┐ |
| `0x0388` | `0x1663994` | ├ all three from **one** function, `0x16633A8`, via `bl 0x1798AA0` |
| `0x038A` | `0x1663A34` | ┘ |
| `0x0390` | `0x166806C` | fn `0x1667770`, builds `NRLoggingMessage` |

So we would *answer* these, never send them.

**The PC capture corpus cannot help, and its silence is not evidence.** These four are **new on
PS3** — absent from the reference's DS2 *and* DS3 tables — and the corpus is from the PC/Steam
client. The platform split is visible in the corpus listing itself, which contains `0x03FF` and
`0x0400`, two opcodes that do not exist in the PS3 binary at all.

**The place they would appear is our own logs**, as `game: no handler, not replying`. That line has
never fired for them across many PS3 logins, so the client is genuinely not sending them — they are
conditional on something we do not do.

**Best candidate: `0x038C` schedules them.** It carries exactly **three** periods
(`upload_period`, `char_data_upload_period`, `enemy_kill_upload_period`), and `0x0387`/`0x0388`/
`0x038A` are exactly **three** opcodes emitted from a single dispatch function; `0x0390` is
separately the `NRLoggingMessage` uploader. That reads like scheduled telemetry that stays off until
a server schedules it. Against the hypothesis: FromSoftware *did* send `0x038C` in the captured PC
sessions and nothing followed — but on a platform where these opcodes do not exist, so it neither
confirms nor refutes.

**The experiment** is cheap and PS3-native: send `0x038C` once with short periods and watch for
`no handler` lines in this range. Until then "wait for a capture" is circular — nothing can appear
in a PS3 capture that the client has no reason to send.

---

## ~~D. Unused push aliases~~ — RESOLVED (2026-08-05)

All 33 aliases are mapped. They are **not** several aliases per message type: each manager is
instantiated once per gameplay MODE, and every instance registers all of its message types at its
own slice of the block. A shared callback re-derives the role from `opcode - block_base` with a
bitmask. So an alias identifies a **(mode, role) pair**.

| Block | Formula | Roles |
|---|---|---|
| BreakIn `0x03B9`–`0x03C8` | `0x3B9 + 4*mode + role` | 0 target, 1 reject, 2 allow, 3 **dead — no handler** |
| Visitor `0x03C9`–`0x03D1` | `0x3C9 + 3*mode + role` | 0 visit, 1 reject, 2 remove |
| QuickMatch `0x03E0`–`0x03E7` | `0x3E0 + 2*role + (1 - mode)` (mode-MINOR and INVERTED) | 0 join, 1 reject, 2 allow, 3 remove |

mode is the request's own type enum: `BreakInType`, `VisitorType`, `QuickMatchGameMode`.

Verified identical in v1.00 and v1.10 — same masks, same jump table, only relocated. Corroborated
from the send side: the invader's `0x0320` Allow-relay picks `0x3BB`/`0x3BF`/`0x3C3`/`0x3C7` by
mode, exactly the role-2 set, derived by a different route.

**There is no working Remove push for BreakIn.** `PushRequestRemoveBreakInTarget` is linked in but
the manager never loads its vtable and the callback has no role-3 branch, so it is silently
discarded on all sixteen ids.

**What this cost:** `0x3BD`, `0x3C1` and `0x3C5` were each tested live as rejections and ignored,
because they are TARGET pushes for modes 1/2/3. The original `breakInPushID + 1` = `0x3BA` was
right all along for mode 0 — and is **still** untested. See §F.

---

## E. Never implement — 5 opcodes

`0x03FB`, `0x03FC`, `0x03FD`, `0x03FF`, `0x0400`.

Listed as DS2 opcodes by the PC/SOTFS map, so anyone working from that map implements them by
default. **This client contains no code for any of them**, and a server that sends `0x03FB` will
simply never be dispatched.

**`0x03FA` was on this list and came off it.** It exists only in **v1.10** — `li r4,0x03fa` occurs
zero times in the v1.00 binary and twice in the title update — and two v1.10 machines were seen
sending it at boot with a 29-byte payload that decoded cleanly. It is now deliberately implemented
as `RequestGetRightMatchingArea` in `matchingarea.go`. The lesson is in `versions.go`: an
"absent from the binary" result is only ever true *of the build it was measured on*.

`internal/server/game/opcodes_test.go` fails the build if any of the five is ever dispatched, and
also asserts every dispatched opcode appears as present in the PS3 map.

---

## F. What is actually left

Not opcodes to build — live tests and open questions.

1. **A declined invasion.** `pushBreakInRejected` assumes the alias immediately after
   `breakInPushID`, i.e. `0x3BA` for mode 0. Never tested. Minutes of play.
2. **The chest and the obelisk landing in one login.** Both confirmed individually; the three
   login pushes are now spaced apart because the applier accepts one entry per pass, but the client
   side of that has not been watched.
3. **`0x038C` as a probe for §C.**
4. **`PushRequestRemoveVisitor` (`0x03D1`) is never sent.** Telling a host their visitor left needs
   to know which host a departing player was in, and no visit session is tracked. The phantom clears
   on the clients' own timeout instead.
5. **Physical distance in auto-summon range**, and **the places the covenant icon does not glow** —
   both believed real from play, both unmeasured. See `tasks/remaining-features.md`.
6. **The bell toll predicate** — who FromSoftware sent `0x03EF` to. Ours is inference.

---

## Confirmed from live PC captures (2026-08-07)

`tasks/live-capture-corpus.md` has the full corpus: nine decrypted sessions, 5,594 messages across
34 buckets — 33 identified opcodes plus `0x0000_unknown`, which is the server responses, whose
header carries no opcode by design. Relevant here:

| opcode | finding |
|---|---|
| `0x0320` | **Push wrapper.** Server→client with `msg_index = 0xFFFFFFFF`; the real push id is protobuf field 1. Client→server it really is `RequestSendMessageToPlayers`. |
| `0x038C` | Genuinely sent by FromSoftware, carrying an element-id list. Reading as upload scheduling stands. |
| `0x03AA` | `PushRequestEvaluateBloodMessage` seen live. |
| `0x03CC` | **Bell Keeper visitor push**, 11 instances. Layout matches our `PushRequestVisit` field for field, and carries belfry cells `101630` (Luna) / `101910` (Sol) — neither of which was in our cell map until 2026-08-07. |
| `0x03EF` | `PushRequestNotifyRingBell` seen live, 4 instances. See `tasks/bell-broadcast.md`. |
| `0x03EE` | Absent from all nine sessions — it is the HOST's message and neither captured client ever held that role. Not evidence of anything. |
| `0x03FA`, `0x03FF`, `0x0400` | Present in the corpus, which is the PC/PS3 split made visible: `0x03FA` exists on PS3 only from v1.10, and `0x03FF`/`0x0400` not at all. |
