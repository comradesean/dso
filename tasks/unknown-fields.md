# The `unknown_N` / `field_N` fields, measured against FromSoftware's live traffic

Built 2026-08-08 from the whole 25,851-message corpus (`corpus/`, 49 buckets, 12,855 c2s /
12,996 s2c, LOCAL 15,351 / VM 10,500). Every field in `proto/*.proto` that carries a placeholder
name was pulled from every message that contains it.

Only two identities are assumed: **LOCAL = 2910025, VM = 3473926**, both read from their
`RequestWaitForUserLoginResponse`. Everything else is derived.

---

## 1. Method

**Decoding.** Every message was re-parsed from its `hex:` line with an independent protobuf wire
decoder rather than read out of the rendered tree, so nesting depth was under our control (the
`AllStatus` blob inside `0x03B8` and the `SignData` inside sign-list responses are only reachable
that way). All 25,851 messages decode cleanly, groups included — the corpus's own trees were used
only as a cross-check.

**Four instruments, in the order they were useful:**

1. **Value distribution.** Constant-vs-varying, with counts. A field that is one value across
   2,685 samples from two machines and 70 sessions is a different kind of fact from one that moves.
2. **Bracketed state.** Location, held souls and role were reconstructed as time series per machine
   and read by **bracketing**: last value strictly before, first value strictly after, with both
   ages in seconds. `PlayerLocation`'s area, cell, activity-cell and position each carry forward
   **separately**, because the client sends partial blocks — a fix that names only a position says
   nothing about the area.
3. **Cross-message identity.** The same id or the same float triple appearing in two unrelated
   messages ties them together.
4. **Both-ended events.** Where LOCAL and VM were the two sides of the same event, the two captures
   check each other. The corpus has six self-invasions, four self-rings (`0x03EE` on one machine and
   the matching `0x03EF` on the other ~95 ms later), one self-kill, and one self-visit-rejection.
   These are the strongest samples here and most of the IDENTIFIED verdicts rest on them.

**Buckets with enough samples to say anything:**

| bucket | n | usable for |
|---|---|---|
| `0x03B8` `RequestUpdatePlayerStatus` (c2s) | 3,430 | all `DS2_Frpg2PlayerData` sub-messages |
| `MatchingParameter` (ours, c2s) | 2,685 | `unknown_4/7/9/10` |
| `MatchingParameter` (other players, in `SignData`) | 95 | `unknown_4` across 61 distinct players |
| `BloodMessageData` (in `0x03AE`/`0x03FF` responses) | 40,057 | `unknown_8` |
| `0x0397` `RequestGetSignList` | 1,719 | `unknown_5/6/7` |
| `0x03D5` `RequestGetVisitorList` | 797 | `field_6` |
| `0x03EA`/`0x03EB` join/leave session | 66 each | `field_1..4` |
| `0x03F1` `RequestNotifyDeath` | 54 | `field_3..8` |
| `0x03E8`/`0x03E9` join/leave guest | 31 each | `field_1..9` / `field_1..4` |
| `0x03EF` `PushRequestNotifyRingBell` | 27 | `field_2/3/4` |
| `0x03ED` `RequestNotifyKillPlayer` | 18 | `field_1..5` |
| `0x0386` `RequestWaitForUserLogin` | 7 | `unknown_1..6` |
| `0x03EE` `RequestNotifyRingBell` | 6 | `field_1/2` |
| `0x03D7` `RequestRejectVisit` | 6 | `unknown_2` |

**Buckets too small, or empty:** `PushRequestRejectVisit` (1), `RequestSummonSign` (1),
`RequestNotifyBuyItem` (2), `AnnounceMessageData` (the announce lists came back **empty** in all 7
responses), and the ~20 message types that never appear at all.

**The sampling bias is severe and it limits several results below.** Two people farmed two belfries
for a day. Every visitor request in the corpus is `BellKeepers`; `clear_count` is 1 in every single
`MatchingParameter` seen, ours and other players'; almost every position is one of five cells; and
the two machines were often side by side. Where a field is constant, that may be a fact about the
field or a fact about the play, and each row below says which it could be.

---

## 2. Every unknown field examined

`n` is the number of message instances in which the field could have appeared. "absent" means the
field was **never serialised** in those instances.

### `DS2_Frpg2RequestMessage.proto`

| message | field | n | observed values (counts) | verdict |
|---|---|---|---|---|
| `PushRequestNotifyRingBell` | `field_2` | 27 | 20 distinct player ids | **IDENTIFIED** — reporting host's player id |
| `PushRequestNotifyRingBell` | `field_3` | 27 | 10160000 (10), 10190000 (17) | **IDENTIFIED** — map id of the belfry rung |
| `PushRequestNotifyRingBell` | `field_4` | 27 | empty (27) | **IDENTIFIED** — always empty |
| `RequestNotifyRingBell` | `field_1` | 6 | 10160000 (3), 10190000 (3) | **IDENTIFIED** — map id |
| `RequestNotifyRingBell` | `field_2` | 6 | empty (6) | **IDENTIFIED** — always empty |
| `RequestNotifyDeath` | `field_3` | 54 | 0 (34), 14 (20) | **IDENTIFIED** — 14 = died as a phantom, 0 = died in own world |
| `RequestNotifyDeath` | `field_4` | 54 | 0 (38), 1 (13), 5 (2), 3 (1) | SUGGESTIVE — cause-of-death category |
| `RequestNotifyDeath` | `field_5` | 54 | 0 (43), 836000 (3), 753004 (3), 319000 (2), 324000/836300/753003 (1 each) | SUGGESTIVE — enemy id, non-zero only when `field_4` = 1 |
| `RequestNotifyDeath` | `field_6` | 54 | 0 (49), 2515 (4), 188 (1) | SUGGESTIVE — souls at death; non-zero only in own-world deaths |
| `RequestNotifyDeath` | `field_7` | 54 | 0 (54) | NO SIGNAL |
| `RequestNotifyDeath` | `field_8` | 54 | 54 distinct 15-byte blobs | **IDENTIFIED** — position, three LE IEEE-754 `fixed32` |
| `RequestNotifyJoinGuestPlayer` | `field_1` | 31 | 16 distinct player ids | **IDENTIFIED** — the guest's player id |
| `RequestNotifyJoinGuestPlayer` | `field_2` | 31 | 14 (23), 5 (6), 6 (1), 10 (1) | **IDENTIFIED** — join kind; value map below |
| `RequestNotifyJoinGuestPlayer` | `field_3` | 31 | 1 (31) | NO SIGNAL |
| `RequestNotifyJoinGuestPlayer` | `field_4` | 31 | 0 (31) | NO SIGNAL |
| `RequestNotifyJoinGuestPlayer` | `field_5` | 31 | 0 (31) | NO SIGNAL |
| `RequestNotifyJoinGuestPlayer` | `field_6` | 31 | 0 (24), 1 (7) | **IDENTIFIED** — 1 iff the guest arrived by break-in |
| `RequestNotifyJoinGuestPlayer` | `field_7` | 31 | 10190000 (17), 10160000 (14) | **IDENTIFIED** — `online_area_id` |
| `RequestNotifyJoinGuestPlayer` | `field_8` | 31 | 8 distinct | **IDENTIFIED** — `cell_id` |
| `RequestNotifyJoinGuestPlayer` | `field_9` | 31 | 31 distinct 15-byte blobs | **IDENTIFIED** — the reporting host's own position |
| `RequestNotifyLeaveGuestPlayer` | `field_1` | 31 | same 16 ids | **IDENTIFIED** — the guest's player id |
| `RequestNotifyLeaveGuestPlayer` | `field_2` | 31 | 14 (23), 5 (6), 6 (1), 10 (1) | **IDENTIFIED** — join kind, echoing the join |
| `RequestNotifyLeaveGuestPlayer` | `field_3` | 31 | 0 (31) | NO SIGNAL |
| `RequestNotifyLeaveGuestPlayer` | `field_4` | 31 | 0 (24), 1 (7) | **IDENTIFIED** — same break-in flag as the join's `field_6` |
| `RequestNotifyJoinSession` | `field_1` | 66 | 40 distinct player ids | **IDENTIFIED** — the host's player id |
| `RequestNotifyJoinSession` | `field_2` | 66 | 14 (65), 2 (1) | **IDENTIFIED** — join kind |
| `RequestNotifyJoinSession` | `field_3` | 66 | 0 (66) | NO SIGNAL |
| `RequestNotifyJoinSession` | `field_4` | 66 | 0 (66) | NO SIGNAL |
| `RequestNotifyLeaveSession` | `field_1` | 66 | same 40 ids | **IDENTIFIED** — the host's player id |
| `RequestNotifyLeaveSession` | `field_2` | 66 | 14 (65), 2 (1) | **IDENTIFIED** — join kind |
| `RequestNotifyLeaveSession` | `field_3` | 66 | 0 (66) | NO SIGNAL |
| `RequestNotifyLeaveSession` | `field_4` | 66 | 0 (66) | NO SIGNAL |
| `RequestNotifyKillPlayer` | `field_1` | 18 | 14 (18) | NO SIGNAL — see note |
| `RequestNotifyKillPlayer` | `field_3` | 18 | 0 (18) | NO SIGNAL |
| `RequestNotifyKillPlayer` | `field_4` | 18 | 0 (18) | NO SIGNAL |
| `RequestNotifyKillPlayer` | `field_5` | 18 | 0 (18) | NO SIGNAL |
| `MatchingParameter` | `unknown_4` | 2,685 ours + 95 others | ours 77 (2685); others 77 (79), 78 (2), 79 (3), 80 (5), 81 (6) | SUGGESTIVE — sum of bonfire intensities |
| `MatchingParameter` | `unknown_7` | 2,685 + 95 | 2 (2,780) | NO SIGNAL |
| `MatchingParameter` | `unknown_9` | 2,685 + 95 | 1 (2,779), 0 (1) | NO SIGNAL |
| `MatchingParameter` | `unknown_10` | 2,685 + 95 | ours 0 (2533), 1 (152); others 0 (95) | **IDENTIFIED** — set while a session is live |
| `RequestGetSignList` | `unknown_5` | 1,719 | 1 (1,719) | NO SIGNAL |
| `RequestGetSignList` | `unknown_6` | 1,719 | 1 (1,719) | NO SIGNAL |
| `RequestGetSignList` | `unknown_7` | 1,719 | 0 (1,719) | NO SIGNAL |
| `RequestGetVisitorList` | `field_6` | 797 | 0 (797) | NO SIGNAL |
| `RequestRejectVisit` | `unknown_2` | 6 | 0 (2), 1 (3), 2 (1) | SUGGESTIVE — enumerated rejection reason |
| `PushRequestRejectVisit` | `unknown_3` | 1 | 1 | SUGGESTIVE — relay of the above |
| `BloodMessageData` | `unknown_8` | 40,057 | **absent (40,057)** | NO SIGNAL — never populated |
| `RequestWaitForUserLogin` | `unknown_1` | 7 | 1 (7) | NO SIGNAL |
| `RequestWaitForUserLogin` | `unknown_2` | 7 | 0 (7) | NO SIGNAL |
| `RequestWaitForUserLogin` | `unknown_3` | 7 | 1 (7) | NO SIGNAL |
| `RequestWaitForUserLogin` | `unknown_4` | 7 | 2 (7) | NO SIGNAL |
| `RequestWaitForUserLogin` | `unknown_5` | 7 | **absent (7)** | NO SIGNAL |
| `RequestWaitForUserLogin` | `unknown_6` | 7 | **absent (7)** | NO SIGNAL |
| `RequestGetAnnounceMessageList` | `unknown_1` | 7 | **absent (7)** | NO SIGNAL |
| `RequestGetAnnounceMessageList` | `unknown_2` | 7 | **absent (7)** | NO SIGNAL |
| `AnnounceMessageData` | `unknown_1` | 0 | — | TOO FEW SAMPLES |
| `AnnounceMessageData` | `unknown_2` | 0 | — | TOO FEW SAMPLES |

### `DS2_Frpg2PlayerData.proto` — all from the 3,430 `0x03B8` blobs

The client sends **partial** `AllStatus` blocks, so `n` differs per sub-message. `ServerSideStatus`
(field 9 of `AllStatus`) was **never sent at all**, 0 of 3,430.

| message | field | n | observed values (counts) | verdict |
|---|---|---|---|---|
| `PlayerLocation` | `unknown_5` | 2,490 | 1,705 distinct floats, min −3.14159, max +3.14159 | **IDENTIFIED** — an angle in radians |
| `PlayerStatus` | `unknown_4` | 180 | 1 (83), 0 (97) | NO SIGNAL — complement of `unknown_5` |
| `PlayerStatus` | `unknown_5` | 181 | 0 (83), 1 (94), 2 (3) | NO SIGNAL |
| `PlayerStatus` | `unknown_6` | 169 | 0 (169) | NO SIGNAL |
| `PlayerStatus` | `unknown_13` | 169 blocks | **absent** | NO SIGNAL |
| `PlayerStatus` | `unknown_14` | 169 blocks | **absent** | NO SIGNAL |
| `PlayerStatus` | `unknown_16` | 182 | 17 distinct, 0–80952 | **IDENTIFIED** — souls currently held |
| `PlayerStatus` | `unknown_17` | 169 blocks | **absent** | NO SIGNAL |
| `PlayerStatus` | `unknown_20` | 169 blocks | **absent** | NO SIGNAL |
| `PlayerStatus` | `unknown_21` | 169 blocks | **absent** | NO SIGNAL |
| `ItemUsingInfo` | `unknown_3` | 204 blocks | **absent** | NO SIGNAL |
| `ItemUsingInfo` | `unknown_7` | 169 | 0 (169) | NO SIGNAL |
| `ItemUsingInfo` | `unknown_8` | 169 | 0 (169) | NO SIGNAL |
| `StatsInfo` | `unknown_1` | 2,187 blocks | **absent** | NO SIGNAL |
| `StatsInfo` | `unknown_2` | 192 | 0 (83), 1 (48), 2 (31), 3 (22), 12 (4), 4/5/8/10 (1 each) | SUGGESTIVE — own-world death counter |
| `StatsInfo` | `unknown_5` | 169 | 1 (169) | NO SIGNAL |
| `StatsInfo` | `unknown_10` | 169 | 77 (169) | SUGGESTIVE — the length of `bonfire_levels` (also its sum) |
| `StatsInfo` | `unknown_11` | 2,187 blocks | **absent** | NO SIGNAL |
| `StatsInfo` | `unknown_12`–`unknown_16` | 2,187 blocks | **absent** | NO SIGNAL |
| `StatsInfo` | `unknown_18` | 2,172 | 2,029 distinct, monotone non-decreasing | **IDENTIFIED** — play time in seconds |
| `StatsInfo` | `unknown_20` | 2,187 blocks | **absent** | NO SIGNAL |
| `StatsInfo` | `unknown_21` | 312 | the pair {500, 800} (140), {500} alone (32) | NO SIGNAL |
| `PhantomTypeCount` | `unknown_1` | 507 | 2 (169), 4 (169), 11 (169) | SUGGESTIVE — a phantom-type key |
| `PhantomTypeCount` | `unknown_2` | 507 | 0 (507) | NO SIGNAL |
| `PhysicalStatus` | `unknown_4`, `unknown_5` | 213 blocks | **absent** | NO SIGNAL |
| `PhysicalStatus` | `unknown_8`–`unknown_13` | 213 blocks | **absent** | NO SIGNAL |
| `PhysicalStatus` | `unknown_15`–`unknown_20` | 175 each | 4 distinct values each, moving in lockstep | SUGGESTIVE — the rest of the defence block |
| `ServerSideStatus` | `unknown_1` | 0 | block never sent | TOO FEW SAMPLES |

### `Shared_Frpg2RequestMessage.proto`

| message | field | n | verdict |
|---|---|---|---|
| `GetServiceStatus` | `unknown_1` | 0 | TOO FEW SAMPLES — auth server, not captured |
| `GetServiceStatusResponse` | `unknown_1` | 0 | TOO FEW SAMPLES — auth server, not captured |
| `RequestQueryLoginServerInfo` | `f2` | 0 | TOO FEW SAMPLES — login server, not captured |

---

## 3. Evidence for each IDENTIFIED and SUGGESTIVE field

### `PushRequestNotifyRingBell` `field_2` / `field_3` / `field_4` — IDENTIFIED

Already settled elsewhere; recorded here for completeness because the proto comment was stale and
has now been corrected. All 27 tolls in the corpus, plus the six `0x03EE` rings, plus four
both-ended ring/toll pairs:

```
18:21:39  LOCAL c2s 0x03EE  field_1 = 10160000
18:21:39  VM    s2c 0x03EF  field_2 = 2910025 (LOCAL)  field_3 = 10160000   +95 ms
04:39:xx  LOCAL c2s 0x03EE  field_1 = 10190000
          VM    s2c 0x03EF  field_2 = 2910025          field_3 = 10190000
(and two more, one of them VM -> LOCAL)
```

`field_2` is the id of the machine that emitted the `0x03EE` — the invaded **host** who died, not
the invader who pulled the lever. `field_3` is the belfry's map id, constant per belfry: 10 of 10
Luna tolls = 10160000, 17 of 17 Sol tolls = 10190000. `field_4` is empty in 27 of 27.

The `0x03EE` request is likewise fully pinned: all six are **one of two literal hex strings**,
`08808fec041200` (Luna) x3 and `08b0f9ed041200` (Sol) x3.

Note for anyone acting on this: the PS3 client never reads this body. Its handler sets a boolean
latch and the loaded map's script asks `IsBellRung()`. These meanings matter for fidelity to
FromSoftware, not for client behaviour. `internal/server/game/telemetry.go` already sends
`field_2` = host id and `field_3` = map id, which is correct — do not "fix" it toward the old
comment.

### `RequestNotifyDeath` `field_3` — IDENTIFIED, now 54 of 54

Role was reconstructed independently from the join messages: a machine is a phantom between its
`0x03EA NotifyJoinSession` and the matching `0x03EB NotifyLeaveSession`.

```
field_3 = 14 and in someone else's session : 20
field_3 =  0 and in our own world          : 34
mismatches                                 :  0
```

This supersedes the 26-of-26 figure in the proto with the same conclusion.

### `RequestNotifyDeath` `field_8` — IDENTIFIED: the death position

The 15 bytes are protobuf: `0d <f32> 15 <f32> 1d <f32>` — a `Vector`, three little-endian
IEEE-754 `fixed32`, the same encoding as `PlayerLocation.position`. Decoded and compared with the
sender's own bracketing `PlayerLocation` fixes:

- **54 of 54** lie within 15 world units of the nearer bracketing fix.
- 40 of 54 lie within 5 units; several match to 0.0 at one decimal place.
- The largest residuals are all cases where the bracketing fix is 10-30 s away and the player was
  moving.

Same encoding, same verdict, for `RequestNotifyJoinGuestPlayer.field_9` (below).

### `RequestNotifyDeath` `field_4` / `field_5` / `field_6` — SUGGESTIVE

- `field_5` is non-zero in exactly 11 of 54, and in **all 11** `field_4` = 1. The values —
  319000, 324000, 753003, 753004, 836000, 836300 — have the shape of DS2 enemy ids. This agrees
  with the note already in the proto. It is not clean: `field_4` = 1 twice more with `field_5` = 0.
- `field_4` also takes 5 (twice) and 3 (once), both with `field_5` = 0. Those three deaths all have
  a bracketed position whose `y` is well below the surrounding fixes, which is what a fall looks
  like — but three samples is not an identification and it is not offered as one.
- `field_6` is non-zero **only** in own-world deaths: 5 of 5 non-zero have `field_3` = 0, and
  0 of 20 phantom deaths are non-zero. In 4 of the 5 the value is exactly the sender's
  `PlayerStatus` `unknown_16` (souls held) as bracketed on both sides — 2515, four times. The fifth
  (VM, 188) does **not** match VM's 80952, and that failure is why this is SUGGESTIVE and not
  IDENTIFIED.

### `RequestNotifyJoinGuestPlayer` — IDENTIFIED: `field_1`, `field_2`, `field_6`, `field_7`, `field_8`, `field_9`

This message is sent by the **host** when a guest enters their world. Its tail is a location
triple.

- **`field_1` = the guest's player id.** 30 of 31 name a player who, within the preceding 120 s,
  had either been sent a `RequestVisit` by this machine or had appeared as the subject of a
  `PushRequestBreakInTarget` to this machine. The 31st (23:21:44, kind 10) names 2997752, whose
  `SignInfo` this machine had summoned 11 s earlier with `RequestSummonSign` — so 31 of 31 once that
  path is included. Six of the 31 name 3473926 (VM) at moments when VM simultaneously emitted
  `0x03EA NotifyJoinSession` naming 2910025 (LOCAL): the six self-invasions, both ends captured.
- **`field_2` = the join kind**, and four of its five values are pinned by an independent message
  that arrived seconds earlier:

  | kind | n | what preceded it |
  |---|---|---|
  | 14 | 23 (+65 on `0x03EA`) | a `RequestVisit` / `PushRequestVisit` with `VisitorType` 1 (BellKeepers) |
  | 5 | 6 | `PushRequestBreakInTarget` with `type = 0`, 12-13 s earlier, same player — 6 of 6 |
  | 6 | 1 | `PushRequestBreakInTarget` with `type = 4`, 13 s earlier, same player |
  | 10 | 1 | our own `RequestSummonSign` for that player's sign, 11 s earlier |
  | 2 | 1 (on `0x03EA`) | `PushRequestSummonSign` for **our** sign, 21 s earlier — we were the summoned phantom |

  So kind 5 and kind 6 are break-ins of `BreakInType` 0 and 4 respectively. This retires the
  "join kind 5 — mechanic not identified" line the same way `tasks/live-capture-corpus.md` did, and
  adds kind 6.
- **`field_6` = 1 iff the guest arrived by break-in.** 31 of 31: `field_6` = 1 for every kind-5 and
  kind-6 join (7 of 7) and 0 for every kind-10 and kind-14 join (24 of 24). It is not simply a copy
  of the kind, since two different kinds map to 1 and two to 0.
- **`field_7` = `online_area_id`.** Equal to the sender's own bracketed area in 31 of 31 on the
  before side and 29 of 31 on the after side (the two exceptions are area changes after the join).
- **`field_8` = `cell_id`.** It is in the same value family as the fields already named `cell_id`
  elsewhere (`RequestNotifyDeath.field_2`, `RequestCreateBloodstain`, `RequestCreateGhostData`) and
  in a completely different family from `PlayerLocation.field_2`. Checked against an independent
  witness — the cell id in the bracketing `RequestCreateGhostData` from the same machine — it
  matches on one side or the other in **27 of 31**, versus 36 of 54 for the field that is already
  established as `cell_id` in `RequestNotifyDeath`. One direct confirmation: at 23:21:33 VM sent
  `RequestSummonSign` with `cell_id` 4290774015, and the `0x03E8` 11 s later carries `field_8` =
  4290774015.
- **`field_9` = the reporting host's own position**, same `Vector` encoding as
  `RequestNotifyDeath.field_8`. All 31 lie within 20 units of a bracketing `PlayerLocation` fix;
  24 within 3.5; **15 of 31 are exactly equal at one decimal place** to a fix the same client sent.
  Exact repeated equality is what makes this the sender's own position rather than the guest's.

`field_3` = 1, `field_4` = 0 and `field_5` = 0 in all 31 and carry no signal.

### `RequestNotifyLeaveGuestPlayer` — IDENTIFIED: `field_1`, `field_2`, `field_4`

Every leave was matched to its own join by (machine, player id, nearest preceding join). In
**31 of 31** the leave's `(field_1, field_2, field_4)` equals its join's `(field_1, field_2,
field_6)` — same guest, same kind, same break-in flag. `field_3` is 0 in all 31.

### `RequestNotifyJoinSession` / `RequestNotifyLeaveSession` — IDENTIFIED: `field_1`, `field_2`

Sent by the **phantom** when it enters (leaves) someone else's world, so `field_1` is the **host**.
Six of the 66 name 2910025 (LOCAL) and were sent by VM at the same moments LOCAL emitted `0x03E8`
naming 3473926 — the same six self-invasions, seen from the other side. `field_2` is the same join
kind as `0x03E8`'s `field_2`: 14 in 65, and 2 in the one session we entered as a summoned white
phantom (see the table above). `field_3` and `field_4` are 0 in all 66.

### `MatchingParameter.unknown_10` — IDENTIFIED: a session is live

For each of the 2,685 `MatchingParameter` instances the machine's role was reconstructed
independently from the join/leave pairs.

```
unknown_10 = 0 : 2533 — hosting a guest:   0    a phantom:   0
unknown_10 = 1 :  152 — hosting a guest: 136    a phantom:   6    neither: 10
```

**Zero of 2,533 zeros occur during a session.** Of the 10 ones with no reconstructed session,
**nine fall 1.4-28 s before a hosting session began** — the client already knows a summon is in
flight before the `0x03E8` goes out — and one (20:48:10) has nothing near it. Both directions are
covered (hosting and being a phantom), which is what stops this being a statement about hosting
alone.

Caveat that survives: every session in the corpus is a Bell Keeper visit or a break-in. If the flag
is really "a session of *some* kind is live", co-op is untested from the host side.

### `MatchingParameter.unknown_4` — SUGGESTIVE: the sum of bonfire intensities

Our own value is 77 in **2,685 of 2,685**, and three independent things are also 77:

1. `StatsInfo.bonfire_levels` has exactly **77 entries** in every one of the 169 blocks that carry
   it, with `bonfire_level` = 1 in all 13,013 entries, so their **sum is 77**.
2. `StatsInfo.unknown_10` = 77 in 169 of 169.
3. The reference schema records `unknown_4` values 77, 154, 231, 693 — which are 77 x {1, 2, 3, 9},
   exactly what "sum of 77 bonfire intensities" gives at NG, NG+, NG++ and NG+8, and DS2 raises
   every bonfire's intensity by one per clear.

Across 95 `MatchingParameter`s from 61 other players it takes only 77 (79), 78 (2), 79 (3), 80 (5),
81 (6) — never below 77 and never more than 4 above, with `clear_count` = 1 in all 95. A Bonfire
Ascetic raises **one** bonfire's intensity, which is what a handful of +1s looks like. It does not
correlate with soul level (the same value spans levels 67-115) and cannot be `77 * clear_count`
alone, because `clear_count` is 1 in every sample here.

**Why this is not IDENTIFIED:** our own value never moves, `clear_count` never leaves 1 anywhere in
the corpus, and the 154/231/693 evidence comes from the reference rather than from these captures.
The 78-81 spread is consistent with the story but is not diagnostic on its own.

### `RequestRejectVisit.unknown_2` / `PushRequestRejectVisit.unknown_3` — SUGGESTIVE

The Bell Keeper flow runs host-first: the trespassing **host** picks a defender out of
`RequestGetVisitorList` and sends `RequestVisit` naming them; the server pushes
`PushRequestVisit` to that defender; the defender may answer `RequestRejectVisit`, and the server
relays `PushRequestRejectVisit` back to the host.

All six rejections, each sent within the same second as the push it answers:

| when | who declined | host | `unknown_2` | state of the decliner, bracketed |
|---|---|---|---|---|
| 08:42:31 | VM | 613004 | 0 | area changes to 10040000 **9 s later**; no session for 18 min |
| 22:28:38 | LOCAL | 2512541 | 0 | area changes to 10040000 **7 s later**; position jumps |
| 20:47:31 | VM | 2910025 | 1 | left a session **4 s** earlier |
| 23:29:02 | LOCAL | 3471982 | 1 | left a session **4 s** earlier |
| 00:29:30 | LOCAL | 3468252 | 1 | left a session **2 s** earlier |
| 03:47:43 | LOCAL | 3064304 | 2 | that same player 3064304 had broken into LOCAL's world and left **7 s** earlier |

What is solid: it is a small enumerated reason, not a boolean and not an id — three values in six
samples. What is not: two samples per value is not an identification, so no name is proposed for
0, 1 or 2.

The relay is the cleanest part. At 20:47:31 VM sent `RequestRejectVisit{player 2910025,
unknown_2 = 1, ...}` and in the same second LOCAL received `PushRequestRejectVisit{player 3473926,
unknown_3 = 1, ...}` — both ends captured, so `unknown_3` carries the decliner's id in field 2 and
their reason in field 3. **n = 1**, which is why it stays SUGGESTIVE.

### `PlayerLocation.unknown_5` — IDENTIFIED: an angle in radians

2,490 samples, 1,705 distinct values, **min exactly −3.14159, max exactly +3.14159, nothing
outside**, and the values are spread over the whole circle (round-to-nearest-integer histogram:
−3:188, −2:480, −1:441, 0:452, 1:284, 2:411, 3:234). A float that is hard-bounded at ±π and covers
the full range is an angle wrapped to (−π, π].

What the corpus proves is "an angle in radians". Sitting immediately after `position` inside
`PlayerLocation`, the player's facing/yaw is the only sensible referent, but that last step is
placement, not measurement, and the proto comment says so. The existing comment on this field
("1074321607, 3225573558, 1078414675") is the raw bit patterns, not values.

### `PlayerStatus.unknown_16` — IDENTIFIED: souls currently held

Three independent lines, all from the same 182 samples:

1. **Lockstep with `soul_memory`.** In the 08:52:36-08:56:13 run, twelve consecutive LOCAL uploads
   have Δ`unknown_16` exactly equal to Δ`soul_memory` — 161, 322, 323, 323, 323, 323, 323, 646,
   646, 646, 323. Every soul gained raises both by the same amount, which is precisely the
   relationship between held souls and soul memory.
2. **The one decrease is a purchase.** `unknown_16` falls 7515 -> 2515, exactly −5000, with
   `soul_memory` unchanged at 752008. The only `RequestNotifyBuyItem` that machine ever sent is at
   08:56:33 with `souls_spent = 5000`. Spending souls lowers held souls and leaves soul memory
   alone.
3. `unknown_16` < `soul_memory` in all 182.

Note the field is a **cached snapshot**: it is refreshed only when the character record is
refreshed, so it can lag the true value by tens of minutes (2515 persisted through nine deaths).
That staleness is why `RequestNotifyDeath.field_6` above could only be checked loosely against it.

### `StatsInfo.unknown_18` — IDENTIFIED: play time in seconds

2,172 samples, 2,029 distinct, **monotone non-decreasing in 2,170 of 2,170 consecutive pairs**.
Compared with the pcap's own clock over the 2,167 consecutive same-machine pairs less than 600 s
apart:

```
median  d(unknown_18) / d(wall clock)  = 0.997
|d(unknown_18) - d(wall clock)| <= 2 s : 1652 / 2167   (76%)
```

The residual is one-sided in the expected direction — the counter runs *slower* than the clock
across pauses and menus — plus a handful of pairs that straddle a ring-buffer seam. A worked run:
05:17:39 -> 05:18:00 -> 05:18:20 -> 05:18:40 -> 05:19:00 gives Δ = 20, 20, 20, 20 against elapsed
21, 20, 20, 20.

**This also shows the field currently named `PlayerStatus.play_time_seconds` (field 18) is not play
time** — see §5.

### `StatsInfo.unknown_2` — SUGGESTIVE: a counter of own-world deaths

Tested against the 54 `RequestNotifyDeath` messages, split by `field_3`:

- Across every consecutive pair of observations, Δ`unknown_2` equals the number of **own-world**
  deaths (`field_3` = 0) in between in **155 of 190** pairs; a further 26 are the same relationship
  lagging by exactly one upload (the death is one snapshot late), 24 are the mirror of that lag,
  and 9 are decreases.
- Phantom deaths (`field_3` = 14) never move it. Worked example: between the 02:15:06 and 02:39:35
  uploads LOCAL died three times, all in its own world, and the counter went 5 -> 8; 02:39:35 to
  02:47:43, two own-world deaths, 8 -> 10; 02:47:43 to 02:59:17, two more, 10 -> 12; and between
  01:08:42 and 02:15:06 there were two deaths of which only one was in its own world, and it moved
  4 -> 5.

**Why not IDENTIFIED:** it resets to 0 nine times and nothing in the corpus explains what resets it.
A counter whose reset condition is invisible is only half understood.

### `StatsInfo.unknown_10` — SUGGESTIVE, and degenerate

77 in 169 of 169, which is simultaneously the **number of** `bonfire_levels` entries (77 in 169 of
169) and their **sum** (77, because every level is 1). The corpus cannot separate the two readings,
and our own value never moves.

### `PhantomTypeCount.unknown_1` / `unknown_2` — SUGGESTIVE

`StatsInfo` field 6 carries exactly three `PhantomTypeCount` entries in each of the 169 blocks:
`{unknown_1 = 2, unknown_2 = 0}`, `{4, 0}`, `{11, 0}`. Fields 7, 8 and 9 — the other three repeated
`PhantomTypeCount` slots — never appear at all. So `unknown_1` is a small key that distinguishes the
entries within a list and `unknown_2` is a payload that was zero every time. With all counts zero
there is nothing to test "count" against.

### `PhysicalStatus.unknown_15`-`unknown_20` — SUGGESTIVE

All six are present in exactly 175 of 213 blocks and take exactly four distinct value tuples, with
identical counts (89 / 61 / 19 / 6) — they change together and only when the character or their
equipment changes. Immediately before them is `physical_defence` (14) and immediately after them are
`petrify_resist` (21), `curse_resist` (22), `agility` (23) and `poise` (24), so a defence/resistance
block of six is the natural reading. One tuple is `30, 30, 30, 30, 124, 108`, i.e. the first four
equal at a floor and the last two much larger, which is how DS2 presents elemental defences versus
poison/bleed resistance. **No assignment of individual names is proposed** — the corpus has four
distinct equipment states and no ground truth from the game.

---

## 4. Fields the corpus can say nothing about, and why

**Never present in any captured message.** These message types were never exchanged in any of the
70 sessions, so their placeholder fields have zero samples:

- `RequestNotifyDisconnectSession.field_1`, `RequestNotifyMirrorKnight.field_1`
- `RequestRejectSign.unknown_4`, `RequestRejectMirrorKnightSign.unknown_4`
- `PushRequestAllowBreakInTarget.unknown_4`
- `PushRequestRejectBreakInTarget.unknown_3`, `.unknown_5`
- `RequestRejectBreakInTarget.unknown_2`, `.unknown_5`
- `PushRequestRejectQuickMatch.unknown_7`, `RequestRejectQuickMatch.unknown_5` — no Undead Match
  traffic at all
- `RegulationFileDiffData.unknown_7` — no regulation push was ever received
- `ManagementTextMessage.unknown_4`, `.unknown_5` — no `0x0389` in the corpus
- `GetServiceStatus.unknown_1`, `GetServiceStatusResponse.unknown_1`,
  `RequestQueryLoginServerInfo.f2` — these live on the **login and auth servers**, and the corpus is
  game-server traffic on ports 50000/50001 only

**Present as a container but empty.** `AnnounceMessageData.unknown_1` and `.unknown_2`: all seven
`RequestGetAnnounceMessageListResponse` messages carry both lists (`changes`, `notices`) and both
are empty, so the item message was never instantiated.

**Optional and never serialised by the client.** These have plenty of carrier messages and the
field is simply absent every time, which is itself the finding — nothing needs to be sent:

| field | absent in |
|---|---|
| `BloodMessageData.unknown_8` | 40,057 message instances |
| `RequestWaitForUserLogin.unknown_5`, `.unknown_6` | 7 of 7 |
| `RequestGetAnnounceMessageList.unknown_1`, `.unknown_2` | 7 of 7 |
| `ItemUsingInfo.unknown_3` | 204 blocks |
| `PlayerStatus.unknown_13/14/17/20/21` | 169 blocks |
| `StatsInfo.unknown_1/11/12/13/14/15/16/20` | 2,187 blocks |
| `PhysicalStatus.unknown_4/5/8/9/10/11/12/13` | 213 blocks |
| `ServerSideStatus.unknown_1` | the whole sub-message is absent from 3,430 of 3,430 |

**Constant only because the play was constant.** These have thousands of samples and never move,
and the reason may be the field or may be us. Do not read "constant" as "meaningless":

- `MatchingParameter.unknown_7` = 2 in 2,780 — but this includes 95 samples from 61 **other**
  players, so it is at least not a per-player attribute of ours.
- `MatchingParameter.unknown_9` = 1 in 2,779 of 2,780. The single 0 is one `RequestGetSignList` from
  LOCAL at 04:38:24 on 08-08, with nothing distinguishable around it.
- `RequestGetSignList.unknown_5` = 1, `.unknown_6` = 1, `.unknown_7` = 0 across 1,719 requests from
  both machines, two areas and 70 sessions. Every one of those requests was a Bell Keeper build
  polling for signs, so a field that varies with sign type or covenant would never have moved.
- `RequestGetVisitorList.field_6` = 0 in 797 — but every one of the 797 is `VisitorType` 1.
- `RequestWaitForUserLogin.unknown_1..4` = 1, 0, 1, 2 in 7 of 7 logins. Both machines, both
  accounts, three capture batches. Nothing here varied, including the things a login might key on.
- `RequestNotifyKillPlayer.field_1` = 14 and `field_3` = 0 in **18 of 18** (up from the 14 of 14
  recorded in the proto). Every kill in the corpus happened inside a kind-14 session, so
  "`field_1` is the session kind" and "`field_1` is the constant 14" cannot be told apart here. This
  does not resolve the PS3-versus-PC placement disagreement already noted in the proto. What the
  corpus does add is a both-ended confirmation of `field_2`: at 18:21:33 VM sent
  `RequestNotifyKillPlayer{field_2 = 2910025}` and LOCAL sent `RequestNotifyDeath` in the same
  second with `field_3` = 0.
- `PlayerStatus.unknown_4` and `unknown_5` are two halves of one indicator — `unknown_4` = 1 exactly
  when `unknown_5` = 0, in 180 of 180 — and `unknown_5` also takes 2 three times. They flip several
  times per session, but every one of those flips is observed at a status upload triggered by a
  session event, so the sampling cannot localise the change and no correlation with death, covenant,
  bonfire, host or phantom role holds up. NO SIGNAL, and worth revisiting with a capture where
  someone deliberately toggles human/hollow form.

---

## 5. Things the corpus implies that are outside this task

Recorded, not acted on. None of these are `unknown_N` fields, and two of them would need renames,
which this task forbids.

1. **`PlayerStatus.play_time_seconds` (field 18) is not play time.** It is 20200 in 169 of 169,
   across two different characters and 15 hours of wall clock — and 20200 is exactly
   `MatchingParameter.calibration_version`. The real play-time counter is `StatsInfo` field 18
   (§3), which advances one per second.
2. **`PlayerLocation.cell_id` (field 2) is not the same quantity as `cell_id` everywhere else.** Its
   values (71304185, 88091620, 67109881, …) share no range with the cell ids in
   `RequestNotifyDeath`, `RequestCreateGhostData`, `RequestCreateBloodstain`,
   `RequestGetBloodMessageList` or `RequestNotifyJoinGuestPlayer.field_8` (4286579708, 4196343,
   4290774012, …). Checked directly: `0x03E8.field_8` equals the bracketed `PlayerLocation.field_2`
   in **0 of 31**, while equalling a bracketing `RequestCreateGhostData` cell in 27 of 31.
3. **`MatchingParameter` carries an undeclared field 13** when it is embedded in
   `RequestGetVisitorList` — present in 797 of 797 with value 0, and absent from all 1,888
   `MatchingParameter`s in the other three carriers. Our proto has no field 13 at all.
4. **`0x03A8` `character_data` is not protobuf** — 0 of 138 parse; it is a 404-byte opaque blob.
5. **Every request in the corpus receives a response**, including the eleven our proto marks
   "Never received" (`0x03B8`, `0x03B1`, `0x03E8`, `0x03E9`, `0x03EA`, `0x03EB`, `0x03ED`, `0x03F1`,
   `0x03F7`, `0x0394`, `0x0398`). They are presumably empty, but the reply does come back.

---

## 6. What was changed in `proto/*.proto`

Comment-only. No name, number or type was touched.

| file | message | fields commented |
|---|---|---|
| `DS2_Frpg2RequestMessage.proto` | `PushRequestNotifyRingBell` | `field_2`, `field_3`, `field_4` (replaced a stale, wrong comment) |
| | `RequestNotifyRingBell` | `field_1`, `field_2` |
| | `RequestNotifyDeath` | `field_3` (count 26 -> 54), `field_8` |
| | `RequestNotifyJoinGuestPlayer` | `field_1`, `field_2`, `field_6`, `field_7`, `field_8`, `field_9` |
| | `RequestNotifyLeaveGuestPlayer` | `field_1`, `field_2`, `field_4` |
| | `RequestNotifyJoinSession`, `RequestNotifyLeaveSession` | `field_1`, `field_2` |
| | `RequestNotifyKillPlayer` | corpus count 14 -> 18, plus the both-ended confirmation |
| | `MatchingParameter` | `unknown_10` |
| `DS2_Frpg2PlayerData.proto` | `PlayerLocation` | `unknown_5` |
| | `PlayerStatus` | `unknown_16` |
| | `StatsInfo` | `unknown_18` |

Nothing rated SUGGESTIVE was applied. `go build ./...` and `go test ./internal/...` were run
afterwards.
