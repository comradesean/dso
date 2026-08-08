# Every invasion in the capture corpus, and which ones rang a bell

Built from `corpus/` (15,573 message files, 18 capture sessions, two batches). All times are UTC on
**2026-08-07** and were read out of each file's `time:` header — no timestamp in this document was
computed from an assumed epoch.

Scripts used were throwaway and live in the session scratchpad, not in the repo.

---

## 1. Method

### 1.1 How each message was decoded

The corpus writes a `hex:` blob and a `protobuf:` tree per file. **The corpus's own tree is wrong on
a minority of files** and cannot be trusted blindly: when the header says `proto-off: 8`, the hex
carries a **4-byte little-endian message-index prefix** before the protobuf, and the bundled decoder
sometimes parses straight through it. Example — `0x03ee_.../R2_LOCAL080826_s1p1_..._c2s_0100.txt`,
index 105, hex `6900000008808fec041200`, decoded by the corpus as `field 13 fixed64 3548162507…`
when the real message is `field 1 = 10160000`.

Everything below was re-decoded with an independent protobuf reader using this rule:

- `proto-off: 12`, `24`, `28` → protobuf starts at hex offset 0.
- `proto-off: 8` (including the 5,305 marked *NO CLEAN PARSE*) → strip 4 bytes.

> **FIXED 2026-08-07, after this document was written.** `cmd/corpus` no longer probes offset 8;
> the header is self-describing and every message now lands at 12 or 28, with `NO CLEAN PARSE` at
> zero and groups rendered. The corpus was rebuilt — same 15,573 files, same 43 buckets, identical
> filenames. **The workaround above is no longer needed**, and any future analysis should read the
> corpus trees directly. The finding itself was correct and is what prompted the fix.

**Verified:** for all 6,515 files with `proto-off: 8`, the first four bytes read as a
little-endian `uint32` equal the file's own `index:` header — 6,515 matches, 0 mismatches. Every
message this document relies on (`0x03E8/E9/EA/EB/ED/EE/F1`, `0x03EF`, `0x03CC/CD/CE`, `0x03D5/D6/D7`,
`0x03FB`, `0x03B9`, `0x0320`, `0x03B8`, the `0x0386` responses) re-parsed cleanly — 0 failures.

### 1.2 Our own player ids — derived per batch, and a correction

Taken from the six responses carrying `replies-to: 0x0386` in `corpus/0x0000_unknown`, field 2:

| session | machine | batch | field 1 (PSN id) | field 2 (player id) |
|---|---|---|---|---|
| `R1_LOCAL_s1p1_20260807000014` | LOCAL | R1 | `01100001000e538e` | **2910025** |
| `R1_LOCAL_s2p1_20260807004617` | LOCAL | R1 | `01100001000e538e` | **2910025** |
| `R1_run1_s1p1_20260807110512` | VM | R1 | `0110000129ca9838` | **3473926** |
| `R1_run2_s2p1_20260807114956` | VM | R1 | `0110000129ca9838` | **3473926** |
| `R2_LOCAL080826_s1p1_20260807141803` | LOCAL | R2 | `01100001000e538e` | **2910025** |
| `R2_VM080826_s2p1_20260807211536` | VM | R2 | `0110000129ca9838` | **3473926** |

> **Correction to the task premise.** The brief said R1 and R2 are different logins with different
> player ids and that ids must not be assumed to carry across. Derived per batch as instructed, they
> **do** carry across: LOCAL is 2910025 in both batches and VM is 3473926 in both, with identical PSN
> id strings. Six independent login responses agree. Nothing downstream assumes this — every
> classification below was computed per batch — but the premise as stated is not what the data says,
> and saying so is safer than silently working around it.

### 1.3 What was counted, and how each row was classified

- **An invasion EVENT** is one `0x03E8 RequestNotifyJoinGuestPlayer` (a guest entered our world → we
  are the HOST, we were invaded) or one `0x03EA RequestNotifyJoinSession` (we entered someone else's
  world → we are the PHANTOM, we invaded). 16 + 48 = **64 events**.
- **Field 1** of both carries the other party's player id. Verified on the self-invasions: at
  18:20:15.947 LOCAL logs `0x03E8 f1 = 3473926` and 1.97 s later VM logs `0x03EA f1 = 2910025` —
  each names the other machine, exactly as the roles require.
- **Session window** = from the join to the matching `0x03E9`/`0x03EB` leave carrying the same
  player id. Where no leave exists, a 600 s cap was used (this happened 0 times).
- **Outcome — killed:** a `0x03ED RequestNotifyKillPlayer` inside the window whose **field 2**
  equals the other party's id. (Field 2 = victim is corroborated twice: by the PS3 ground-truth
  capture recorded in `proto/DS2_Frpg2RequestMessage.proto`, and by every one of the 14 kills here
  naming a player we had joined. See §7 for a field-placement discrepancy that does *not* affect
  field 2.)
- **Outcome — died:** a `0x03F1 RequestNotifyDeath` inside the window. `0x03F1` field 3 separates
  the two roles cleanly and was checked against all 64 windows: **f3 = 14 ⟺ we died as a phantom
  inside someone's session; f3 = 0 ⟺ we died in our own world.** 26 of 26 deaths agree with the
  role independently derived from the join messages. Deaths carrying `f4 = 1, f5 = <six-digit id>`
  are PvE deaths (enemy id) and are flagged where they matter.
- **Self-invasion** = the other party's id is one of ours. Six such invasions exist, each observed
  from both ends (12 of the 64 events), all in R2, all LOCAL hosting VM. R1 has none.
- **A BELL EVENT** = one distinct `0x03EF PushRequestNotifyRingBell` broadcast, grouped by
  (field 2 = reporting host, field 3 = map) within a 3 s window. 18 deliveries → **13 events**.
- The **reporting host** is the player named in `0x03EF` field 2 — the client that sent `0x03EE`.
  The invader pulls the lever; the host's client reports it. No one here is called "the ringer".

### 1.4 Every count recomputed a second, different way

| claim | first method | second method | agree? |
|---|---|---|---|
| 16 host-side invasions | parsed `0x03E8` records | `ls corpus/0x03e8_*/ \| wc -l` = 16 | yes |
| 48 phantom-side invasions | parsed `0x03EA` | `ls corpus/0x03ea_*/ \| wc -l` = 48 | yes |
| 14 kills | parsed `0x03ED` | `ls corpus/0x03ed_*/ \| wc -l` = 14 | yes |
| 26 deaths | parsed `0x03F1` | `ls corpus/0x03f1_*/ \| wc -l` = 26 | yes |
| 3 rings | parsed `0x03EE` | `ls corpus/0x03ee_*/ \| wc -l` = 3 | yes |
| 18 toll deliveries / 13 events | my parser, grouped | `grep` on the corpus's own decoder text for `field 2`/`field 3` → 18 lines, 11 distinct (host, map) pairs, of which 3241000 and 2910025 each appear as two separated events → 13 | yes |
| 48 phantom joins all preceded by a Bell Keeper visit push | per-event lookup | **arithmetic on independent totals:** 52 `0x03CC` pushes − 3 `0x03D7` rejects − 1 ignored (3457842, 08:16:57, no join and no reject) = 48 | yes |
| 13 of the 16 guests were Bell Keepers we summoned | per-event lookup | 14 `0x03D6 RequestVisit` − 1 rejected by the target (`0x03CD` at 20:47:31) = 13 accepted, and `PUSH_0x03CE` remove-visitor count = **13** | yes |
| every visitor request is BellKeepers | parsed `0x03D5` field 5 | 430 requests, field 5 = 1 in **430/430**, 0 other values; and 0 files exist for any `0x03C9`–`0x03CB` (Blue Sentinel) or `0x03CF`–`0x03D1` (Rat) push | yes |
| 4 of 14 kills were followed by a bell | scanning each invasion window | independent scan: for each `0x03ED`, search all 18 tolls for one with `field 2 == victim` within 60 s → 4 kills hit (09:11:58, 09:47:14, 18:21:33, 22:54:08) | yes |

The 16 host-side events decompose exactly: **13** Bell Keeper visitors we summoned (`0x03D6`) +
**1** break-in (`0x03FB`) + **2** sign-family joins (`0x03B9`) = 16. Independent arithmetic, same total.

### 1.5 Location: how it was read and bracketed

`0x03B8 RequestUpdatePlayerStatus` nests `PlayerStatus` at field 1 and `PlayerLocation` at
field 1.1, with `f1 = online_area_id`, `f2 = opaque`, `f3 = online_activity_area_id`,
`f4 = {three fixed32 IEEE-754 floats}`.

**The client sends partial location blocks.** Of 2,213 `0x03B8` requests, 1,745 carry a location
block at all; of those, only **161** restate `online_area_id` and **293** restate the cell, while
1,640 carry a position. So area and cell must be carried forward, and a "current" area can be many
minutes old.

Every location in this document is therefore given as a **bracket**: the last fix *before* and the
first fix *after*, each with its age in seconds, separately for position and for the explicit
area/cell report. Where the before-fix and after-fix disagree on map or cell, the location is
marked **UNRESOLVED** rather than picked.

**Our own status stream follows us into the host's world.** Proved directly, not assumed —
LOCAL at 20:19:09 reports Belfry Luna `(-185.9, 23.2, 511.4)`; it joins 3471570's session at
20:19:19; at 20:19:17 it reports area **10190000** cell **101950** at `(-773.0, 203.7, 638.6)`;
at 20:20:43, 6 s after leaving, it is back at Luna `(-185.5, 23.2, 512.4)`. VM shows the identical
pattern at 22:23:16 and 22:24:12. The coordinate `(-773.0, 203.7, 638.6)` recurs verbatim across
many summons — it is the Bell Keeper summon spawn in cell 101950. Consequence: during a phantom
session our own position is a position **inside the ringing host's world**, and is usable for
distance work.

### 1.6 A correction that changes how bell origins are derived

The brief and `tasks/bell-broadcast.md` treat the `PUSH_0x03CC` visit push's area/cell as the
belfry the invader was summoned to. **It is not. It is the RECIPIENT's own registered area/cell.**

Two independent tests:

- **Test A.** For all **52** `0x03CC` pushes, the push's `f6`/`f7` equals the *recipient's* own
  carried area/cell at that instant. **52 matches, 0 mismatches.**
- **Test B.** The seven `0x03D6 RequestVisit` messages LOCAL aimed at VM. In six of them LOCAL's own
  cell (101640) differs from the cell it asked for; in all six the requested cell is **VM's** cell,
  not LOCAL's. The seventh (20:47:31) is the one time both were in 101640.

  | time | requester | asked | requester was in | target (VM) was in |
  |---|---|---|---|---|
  | 18:20:02 | LOCAL | 101630 | 101640 (cell 0 in flight) | **101630** |
  | 18:58:16 | LOCAL | 101630 | 101640 | **101630** |
  | 19:03:10 | LOCAL | 101630 | 101640 | **101630** |
  | 19:07:12 | LOCAL | 101630 | 101640 | **101630** |
  | 19:11:15 | LOCAL | 101630 | 101640 | **101630** |
  | 20:41:24 | LOCAL | 101630 | 101640 | **101630** |
  | 20:47:31 | LOCAL | 101640 | 101640 | 101640 (ambiguous) |

  Confirmed once more by 22:11:36, where VM (standing in Luna 101640) requests
  `area 10170000 cell 101730` for target 735037 — a Sinners' Rise cell, nowhere near either player's
  belfry. 735037 then joins VM's world at map 10160000.

The client polls `0x03D5 RequestGetVisitorList` across many cells in rotation (101610…101670,
101910…101970, and areas as far away as 10100000 and 50350000), so a candidate returned by the
query for cell X is registered at cell X, and that cell is what `0x03D6` and `0x03CC` carry.

**Consequence for this document:** for an invasion into a third party's world, the belfry's
*cell* is not recoverable from the visit push. Where one of our machines was in that world, our own
in-world position is used instead and is labelled as such. Otherwise the origin is known only to
map granularity, and is marked so.

---

## 2. Session inventory

`_VM` / `_run` in the name = the VM machine; everything else = LOCAL. `sNpM` = capture run N, part M;
the login handshake appears only in part 1, so parts inherit their run's player id.

| session | batch | machine | UTC start | UTC end | msgs | our player id |
|---|---|---|---|---|---|---|
| `R1_LOCAL_s1p1_20260807000014` | R1 | LOCAL | 04:05:14 | 04:05:14 | 5 | 2910025 (login) |
| `R1_LOCAL_s2p1_20260807004617` | R1 | LOCAL | 04:46:35 | 05:45:00 | 721 | 2910025 (login) |
| `R1_LOCAL_s2p2_20260807014511` | R1 | LOCAL | 05:45:19 | 06:19:57 | 360 | 2910025 (inherited) |
| `R1_LOCAL_s2p3_20260807022014` | R1 | LOCAL | 06:20:27 | 07:55:29 | 968 | 2910025 (inherited) |
| `R1_LOCAL_s2p4_20260807035532` | R1 | LOCAL | 07:55:51 | 08:52:18 | 652 | 2910025 (inherited) |
| `R1_run1_s1p1_20260807110512` | R1 | VM | 08:05:20 | 08:48:55 | 939 | 3473926 (login) |
| `R1_run2_s2p1_20260807114956` | R1 | VM | 08:50:10 | 09:47:55 | 876 | 3473926 (login) |
| `R1_LOCAL_s2p5_20260807045222` | R1 | LOCAL | 08:52:28 | 08:58:11 | 170 | 2910025 (inherited) |
| `R1_LOCAL_s2p6_20260807045841` | R1 | LOCAL | 08:58:42 | 09:48:03 | 903 | 2910025 (inherited) |
| `R2_VM080826_s2p1_20260807211536` | R2 | VM | 18:15:54 | 22:57:33 | 4387 | 3473926 (login) |
| `R2_LOCAL080826_s1p1_20260807141803` | R2 | LOCAL | 18:18:29 | 18:57:55 | 904 | 2910025 (login) |
| `R2_LOCAL080826_s1p2_20260807145757` | R2 | LOCAL | 18:57:59 | 19:32:24 | 651 | 2910025 (inherited) |
| `R2_LOCAL080826_s1p3_20260807153229` | R2 | LOCAL | 19:32:31 | 19:58:54 | 482 | 2910025 (inherited) |
| `R2_LOCAL080826_s1p4_20260807155907` | R2 | LOCAL | 19:59:14 | 20:04:32 | 91 | 2910025 (inherited) |
| `R2_LOCAL080826_s1p5_20260807160433` | R2 | LOCAL | 20:04:37 | 20:22:24 | 416 | 2910025 (inherited) |
| `R2_LOCAL080826_s1p6_20260807162225` | R2 | LOCAL | 20:22:27 | 20:32:25 | 165 | 2910025 (inherited) |
| `R2_LOCAL080826_s1p7_20260807163231` | R2 | LOCAL | 20:32:43 | 22:05:10 | 1669 | 2910025 (inherited) |
| `R2_LOCAL080826_s1p8_20260807180513` | R2 | LOCAL | 22:05:17 | 22:57:14 | 1214 | 2910025 (inherited) |

The session filename's trailing timestamp is a **local wall clock on the capturing machine and does
not match the UTC contents** (e.g. `…_20260807211536` starts at 18:15:54 UTC). Only the `time:`
headers were used.

Simultaneous coverage: R1 has both machines online 08:05:20–08:52:18 and 08:58:42–09:47:55;
R2 has both online 18:18:29–22:57:14.

---

## 3. THE TIMELINE — every invasion, both batches

Legend. **Role**: `INVADED (host)` = a guest entered our world; `INVADED THEM (phantom)` = we
entered theirs. **Kind** = `0x03E8`/`0x03EA` field 2 — 14 is the Bell Keeper session kind
throughout; 5 and 6 are two other join kinds seen only on the host side (§3.3). Outcome timestamps
are the `0x03ED`/`0x03F1` times. "Bell" columns give the `0x03EE` ring we sent (if any) and the
`0x03EF` toll that names this invasion's host, with the offset from the join.

### 3.1 Batch R1 — 9 invasions, all as phantom, no self-invasions

| # | UTC | machine | role | other party (id / PSN id) | self? | kind | dur | outcome | 0x03EE | 0x03EF |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | 08:08:22 | VM | INVADED THEM (phantom) | host **2487834** / `011000011212bff7` | 3rd party | 14 | 309 s | **killed the host** 08:13:15 | none | none |
| 2 | 08:14:07 | VM | INVADED THEM (phantom) | host **1194297** / `0110000103ef22d9` | 3rd party | 14 | 152 s | **killed the host** 08:16:22 | none | none |
| 3 | 08:23:24 | VM | INVADED THEM (phantom) | host **1194297** / `0110000103ef22d9` | 3rd party | 14 | 47 s | **died** 08:23:59 (f3=14) | none | none |
| 4 | 09:07:21 | VM | INVADED THEM (phantom) | host **3470456** / `011000015cb52790` | 3rd party | 14 | 60 s | **died** 09:08:09 (f3=14) | none | none |
| 5 | 09:10:24 | LOCAL | INVADED THEM (phantom) | host **3468141** / `011000013d05b244` | 3rd party | 14 | 111 s | **killed the host** 09:11:58 | none (host's, uncaptured) | **B3** 09:12:16.895, +18.1 s after the kill, heard by LOCAL |
| 6 | 09:12:39 | LOCAL | INVADED THEM (phantom) | host **3066937** / `011000011c65684c` | 3rd party | 14 | 61 s | **died** 09:13:27 (f3=14) | none | none |
| 7 | 09:15:26 | LOCAL | INVADED THEM (phantom) | host **2657983** / `01100001475057fe` | 3rd party | 14 | 88 s | **died** 09:16:42 (f3=14) | none | none |
| 8 | 09:25:46 | LOCAL | INVADED THEM (phantom) | host **3470456** / `011000015cb52790` | 3rd party | 14 | 72 s | **died** 09:26:46 (f3=14) | none | none |
| 9 | 09:45:41 | VM | INVADED THEM (phantom) | host **2350487** / `011000011599661a` | 3rd party | 14 | 110 s | **killed the host** 09:47:14 | none (host's, uncaptured) | **B4** 09:47:24.653, +9.7 s after the kill, heard by VM |

All nine were preceded 12.6–17.4 s earlier by a `PUSH_0x03CC` mode-1 (BellKeepers) visit push
naming that host. R1 also contains two bells (B1, B2) that belong to no invasion of ours — see §4.

### 3.2 Batch R2 — 55 invasions (49 distinct; 6 are the same invasion seen twice)

Rows sharing a grey pairing note are two ends of one real invasion.

| # | UTC | machine | role | other party (id / PSN id) | self? | kind | dur | outcome | 0x03EE | 0x03EF |
|---|---|---|---|---|---|---|---|---|---|---|
| 10 | 18:20:15 | LOCAL | **INVADED (host)** | guest **3473926** = VM / `0110000129ca9838` | SELF (pairs with 11) | 14 | 95 s | **we died** 18:21:33 (f3=0, f6=2515) | **LOCAL rang twice**: 18:21:39.446 (+5.6 s after the death) and 18:21:51.153 (+17.3 s) | — (reporting host gets no toll) |
| 11 | 18:20:17 | VM | INVADED THEM (phantom) | host **2910025** = LOCAL / `01100001000e538e` | SELF (pairs with 10) | 14 | 92 s | **killed the host** 18:21:33 | — | **B5** 18:21:39.541 (+5.6 s) and **B6** 18:21:51.247 (+17.3 s), both heard by VM |
| 12 | 18:26:28 | LOCAL | INVADED THEM (phantom) | host **1647744** / `011000011a99dff9` | 3rd party | 14 | 107 s | **killed the host** 18:27:59 | none | **none** |
| 13 | 18:32:21 | LOCAL | INVADED THEM (phantom) | host **3207541** / `0110000144520b73` | 3rd party | 14 | 31 s | neither observed | none | **B7** 18:32:48.080 (+26.6 s), heard by LOCAL — the host rang while LOCAL was in the world but had not killed them |
| 14 | 18:37:09 | LOCAL | INVADED THEM (phantom) | host **1665540** / `0110000119f6a1f1` | 3rd party | 14 | 47 s | **killed the host** 18:37:39 | none | **none** |
| 15 | 18:41:00 | LOCAL | INVADED THEM (phantom) | host **3112652** / `011000015876dfad` | 3rd party | 14 | 116 s | **killed the host** 18:42:39 | none | **none** |
| 16 | 18:55:37 | LOCAL | **INVADED (host)** | guest **2973239** / `011000016d3ae342` | 3rd party | **5** | 118 s | **we died** 18:57:22 (f3=0, f6=2515) | none | none |
| 17 | 18:58:30 | LOCAL | **INVADED (host)** | guest **3473926** = VM | SELF (pairs with 18) | 14 | 81 s | neither observed | none | none |
| 18 | 18:58:32 | VM | INVADED THEM (phantom) | host **2910025** = LOCAL | SELF (pairs with 17) | 14 | 79 s | neither observed | none | none |
| 19 | 19:03:24 | LOCAL | **INVADED (host)** | guest **3473926** = VM | SELF (pairs with 20) | 14 | 36 s | neither observed | none | none |
| 20 | 19:03:26 | VM | INVADED THEM (phantom) | host **2910025** = LOCAL | SELF (pairs with 19) | 14 | 34 s | neither observed | none | none |
| 21 | 19:07:27 | LOCAL | **INVADED (host)** | guest **3473926** = VM | SELF (pairs with 22) | 14 | 10 s | neither observed | none | none |
| 22 | 19:07:29 | VM | INVADED THEM (phantom) | host **2910025** = LOCAL | SELF (pairs with 21) | 14 | 8 s | neither observed | none | none |
| 23 | 19:11:30 | LOCAL | **INVADED (host)** | guest **3473926** = VM | SELF (pairs with 24) | 14 | 10 s | neither observed | none | none |
| 24 | 19:11:32 | VM | INVADED THEM (phantom) | host **2910025** = LOCAL | SELF (pairs with 23) | 14 | 7 s | neither observed | none | none |
| 25 | 19:13:30 | LOCAL | INVADED THEM (phantom) | host **2487254** / `011000014c88083b` | 3rd party | 14 | 24 s | **died** 19:13:42 (f3=14) | none | none |
| 26 | 19:14:24 | LOCAL | INVADED THEM (phantom) | host **1616207** / `0110000106502d1b` | 3rd party | 14 | 38 s | **died** 19:14:49 (f3=14) | none | none |
| 27 | 19:27:52 | LOCAL | INVADED THEM (phantom) | host **3472879** / `011000011929d470` | 3rd party | 14 | 113 s | **died** 19:29:32 (f3=14) | none | none |
| 28 | 20:02:59 | LOCAL | INVADED THEM (phantom) | host **3459848** / `011000011a5dc8e2` | 3rd party | 14 | 151 s | **died** 20:05:17 (f3=14) | none | none |
| 29 | 20:14:14 | LOCAL | **INVADED (host)** | guest **2660247** / `0110000153d717a7` | 3rd party | **6** (break-in) | 171 s | **we died** 20:16:52 (f3=0, f6=2515) | none | none |
| 30 | 20:19:19 | LOCAL | INVADED THEM (phantom) | host **3471570** / `011000010b6c85ea` | 3rd party | 14 | 79 s | **killed the host** 20:20:21 | none | **none** |
| 31 | 20:29:15 | LOCAL | INVADED THEM (phantom) | host **3469576** / `011000013e6c6d52` | 3rd party | 14 | 517 s | neither observed | none | none |
| 32 | 20:41:38 | LOCAL | **INVADED (host)** | guest **3473926** = VM | SELF (pairs with 33) | 14 | 18 s | neither observed | none | none |
| 33 | 20:41:40 | VM | INVADED THEM (phantom) | host **2910025** = LOCAL | SELF (pairs with 32) | 14 | 16 s | neither observed | none | none |
| 34 | 20:44:48 | LOCAL | INVADED THEM (phantom) | host **3469576** / `011000013e6c6d52` | 3rd party | 14 | 73 s | neither observed | none | none |
| 35 | 20:45:15 | VM | INVADED THEM (phantom) | host **3446575** / `011000014a5e02e0` | 3rd party | 14 | 131 s | neither observed | none | none |
| 36 | 20:48:00 | VM | INVADED THEM (phantom) | host **3428694** / `0110000104f02a47` | 3rd party | 14 | 196 s | neither observed | none | none |
| 37 | 20:50:39 | LOCAL | INVADED THEM (phantom) | host **3428695** / `01100001009dc945` | 3rd party | 14 | 112 s | neither observed | none | none |
| 38 | 20:52:29 | VM | INVADED THEM (phantom) | host **3428695** / `01100001009dc945` | 3rd party | 14 | 141 s | neither observed | none | none |
| 39 | 20:52:57 | LOCAL | INVADED THEM (phantom) | host **3469576** / `011000013e6c6d52` | 3rd party | 14 | 20 s | neither observed | none | none |
| 40 | 20:55:17 | VM | INVADED THEM (phantom) | host **1910304** / `011000011caecc59` | 3rd party | 14 | 301 s | **died** 21:00:05 (f3=14) | none | none |
| 41 | 20:58:59 | LOCAL | INVADED THEM (phantom) | host **3428694** / `0110000104f02a47` | 3rd party | 14 | 8 s | neither observed | none | none |
| 42 | 21:00:17 | LOCAL | INVADED THEM (phantom) | host **1321650** / `0110000119ece636` | 3rd party | 14 | 131 s | neither observed | none | **B8** 21:02:17.456 (+120.0 s), heard by LOCAL **and** VM |
| 43 | 21:01:30 | VM | INVADED THEM (phantom) | host **3428695** / `01100001009dc945` | 3rd party | 14 | 7 s | neither observed | none | none |
| 44 | 21:02:04 | VM | INVADED THEM (phantom) | host **3428694** / `0110000104f02a47` | 3rd party | 14 | 23 s | neither observed | none | none |
| 45 | 21:06:43 | VM | INVADED THEM (phantom) | host **3428695** / `01100001009dc945` | 3rd party | 14 | 110 s | **killed the host** 21:08:16 | none | **none** |
| 46 | 21:11:43 | VM | INVADED THEM (phantom) | host **3428695** / `01100001009dc945` | 3rd party | 14 | 83 s | **killed the host** 21:12:49 | none | **none** |
| 47 | 21:13:54 | VM | INVADED THEM (phantom) | host **3446575** / `011000014a5e02e0` | 3rd party | 14 | 144 s | **killed the host** 21:16:02 | none | **none** |
| 48 | 21:16:56 | VM | INVADED THEM (phantom) | host **3428695** / `01100001009dc945` | 3rd party | 14 | 30 s | **killed the host** 21:17:10 | none | **none** |
| 49 | 21:21:43 | VM | INVADED THEM (phantom) | host **3428695** / `01100001009dc945` | 3rd party | 14 | 19 s | neither observed | none | none |
| 50 | 21:23:13 | VM | INVADED THEM (phantom) | host **3428694** / `0110000104f02a47` | 3rd party | 14 | 49 s | neither observed | none | **B9** 21:24:00.761 (+46.9 s), heard by VM **and** LOCAL |
| 51 | 21:41:20 | VM | **INVADED (host)** | guest **2140716** / `011000014686c072` | 3rd party | 14 | 82 s | **we died** 21:42:25 (f3=0, f6=188) — **ambiguous, see note** | none | none |
| 52 | 21:41:47 | VM | **INVADED (host)** | guest **1931731** / `0110000142af9262` | 3rd party | 14 | 55 s | same death 21:42:25 — **ambiguous, see note** | none | none |
| 53 | 21:43:11 | LOCAL | **INVADED (host)** | guest **2973239** / `011000016d3ae342` | 3rd party | **5** | 55 s | **we died** 21:43:53 (f3=0, f4=5) | none | none |
| 54 | 21:51:58 | LOCAL | **INVADED (host)** | guest **649074** / `0110000103c39338` | 3rd party | 14 | 54 s | **we died** 21:52:36 (f3=0) | none | none |
| 55 | 21:58:21 | VM | **INVADED (host)** | guest **1931731** / `0110000142af9262` | 3rd party | 14 | 172 s | **died to a PvE enemy** 22:00:56 (f3=0, f4=1, f5=753003) — not the guest | none | none |
| 56 | 22:11:51 | VM | **INVADED (host)** | guest **735037** / `01100001085fdd8d` | 3rd party | 14 | 235 s | **we died** 22:15:29 (f3=0) | **VM rang** 22:15:43.005 (+13.6 s after the death) | — (reporting host gets no toll); **B10** delivered to LOCAL |
| 57 | 22:23:18 | VM | INVADED THEM (phantom) | host **3471010** / `0110000172cf4978` | 3rd party | 14 | 47 s | neither observed | none | **B11** 22:23:57.355 (+38.6 s), heard by VM **and** LOCAL |
| 58 | 22:24:47 | LOCAL | INVADED THEM (phantom) | host **2113737** / `0110000106b64256` | 3rd party | 14 | 50 s | **died** 22:25:25 (f3=14) | none | none |
| 59 | 22:34:37 | LOCAL | **INVADED (host)** | guest **2512541** / `0110000136ff6938` | 3rd party | 14 | 84 s | **we died** 22:35:44 (f3=0) | none | none |
| 60 | 22:39:46 | LOCAL | INVADED THEM (phantom) | host **2712542** / `011000010551afcf` | 3rd party | 14 | 17 s | neither observed | none | **B12** 22:39:58.746 (+12.1 s), heard by LOCAL **and** VM |
| 61 | 22:40:41 | LOCAL | INVADED THEM (phantom) | host **2512541** / `0110000136ff6938` | 3rd party | 14 | 53 s | **died** 22:41:21 (f3=14) | none | none |
| 62 | 22:43:54 | LOCAL | **INVADED (host)** | guest **2512541** / `0110000136ff6938` | 3rd party | 14 | 73 s | **we died** 22:44:50 (f3=0) | none | none |
| 63 | 22:48:25 | LOCAL | INVADED THEM (phantom) | host **2712542** / `011000010551afcf` | 3rd party | 14 | 85 s | **died** 22:49:38 (f3=14) | none | none |
| 64 | 22:53:33 | LOCAL | INVADED THEM (phantom) | host **3161263** / `01100001050b29e8` | 3rd party | 14 | 52 s | **killed the host** 22:54:08 | none (host's, uncaptured) | **B13** 22:54:20.272 (+11.8 s after the kill), heard by LOCAL **and** VM |

**Note on rows 51/52.** Two guests were in VM's world simultaneously (2140716 from 21:41:20, 1931731
from 21:41:47, both leaving at 21:42:42) and VM died once, at 21:42:25. **Which of the two killed VM
is not determinable from the capture** — `0x03F1` names no killer, and VM sent no `0x03ED`. Counted
as one death, attributed to neither.

### 3.3 Where each guest came from (host-side rows only)

Every one of the 16 guest joins has an identified summoning path — none is unexplained as to
*whether* it happened, though two are unexplained as to *what mechanic* it is:

| join kind | n | preceded by | notes |
|---|---|---|---|
| 14 | 13 | `0x03D6 RequestVisit` we sent 10.8–16.1 s earlier, mode 1 = BellKeepers, plus a `PUSH_0x03CE` remove-visitor confirming the id | the Bell Keeper covenant defender we summoned into our own world |
| 6 | 1 | `PUSH_0x03FB PushRequestBreakInTarget` (20:14:01, f4 = 4), answered 0.3 s later by relaying push **1021 = 0x03FD AllowBreakInTarget** through `0x0320` | a break-in invader, a different mechanic from the covenant |
| 5 | 2 | `PUSH_0x03B9` (push id 953) naming the player, answered ~0.3 s later by relaying push **955 = 0x03BB** through `0x0320` | **mechanic not identified.** 0x03B9/0x03BB are not in `proto/DS2_Frpg2RequestMessage.proto`; by analogy with the sign block (0x039B/9C/9D) and the aliased visitor block (0x03CC/CD/CE) they look like a sign-family summon/allow pair, but that is inference. Both were player 2973239; LOCAL died within 2 min each time. |

Both kind-5 joins are counted as invasions in the tables below because rule 1 defines the role by
`0x03E8`, but they are **not** Bell Keeper invasions and should not be read as such.

---

## 4. THE BELLS — 13 events, 18 deliveries

`0x03EF` field 2 = the reporting host's id; field 3 = the map (10160000 Belfry Luna,
10190000 Iron Keep incl. Belfry Sol). We captured the sending side (`0x03EE`) for exactly three of
these, all from one of our machines acting as host.

| bell | UTC | reporting host (id / PSN id) | map | tied to which invasion | tie strength | heard by | our `0x03EE` |
|---|---|---|---|---|---|---|---|
| **B1** | 05:41:17.386 | 3241000 / **not recoverable** | 10160000 Luna | none — no machine of ours was in that world | n/a | LOCAL | no |
| **B2** | 05:41:29.994 | 3241000 / **not recoverable** | 10160000 Luna | none | n/a | LOCAL | no |
| **B3** | 09:12:16.895 | 3468141 / `011000013d05b244` | 10190000 Iron Keep | row 5 — LOCAL killed 3468141 at 09:11:58 | **id + timing** (+18.1 s) | LOCAL | no (host's own, uncaptured) |
| **B4** | 09:47:24.653 | 2350487 / `011000011599661a` | 10160000 Luna | row 9 — VM killed 2350487 at 09:47:14 | **id + timing** (+9.7 s) | VM | no |
| **B5** | 18:21:39.541 | 2910025 = **LOCAL** | 10160000 Luna | rows 10/11 — VM killed LOCAL at 18:21:33 | **id + timing + both ends** (+5.6 s) | VM | **yes**, LOCAL 18:21:39.446 (0.095 s earlier) |
| **B6** | 18:21:51.247 | 2910025 = **LOCAL** | 10160000 Luna | same invasion, second ring 11.7 s later | **id + timing + both ends** | VM | **yes**, LOCAL 18:21:51.153 (0.094 s earlier) |
| **B7** | 18:32:48.080 | 3207541 / `0110000144520b73` | 10190000 Iron Keep | row 13 — LOCAL was the phantom in that world, no kill by LOCAL | **id only** (timing is inside the session but nothing links it to an event we sent) | LOCAL | no |
| **B8** | 21:02:17.45 | 1321650 / `0110000119ece636` | 10160000 Luna | row 42 — LOCAL was the phantom, no kill by LOCAL | **id only** | VM 21:02:17.454, LOCAL 21:02:17.456 | no |
| **B9** | 21:24:00.76 | 3428694 / `0110000104f02a47` | 10160000 Luna | row 50 — VM was the phantom, no kill by VM | **id only** | VM 21:24:00.759, LOCAL 21:24:00.761 | no |
| **B10** | 22:15:43.103 | 3473926 = **VM** | 10160000 Luna | row 56 — VM was killed at 22:15:29 by guest 735037 | **id + timing + both ends** (+13.6 s) | LOCAL | **yes**, VM 22:15:43.005 (0.098 s earlier) |
| **B11** | 22:23:57.35 | 3471010 / `0110000172cf4978` | 10190000 Iron Keep | row 57 — VM was the phantom, no kill by VM | **id only** | VM 22:23:57.355, LOCAL 22:23:57.357 | no |
| **B12** | 22:39:58.74 | 2712542 / `011000010551afcf` | 10190000 Iron Keep | row 60 — LOCAL was the phantom, no kill by LOCAL | **id only** | VM 22:39:58.744, LOCAL 22:39:58.746 | no |
| **B13** | 22:54:20.27 | 3161263 / `01100001050b29e8` | 10190000 Iron Keep | row 64 — LOCAL killed 3161263 at 22:54:08 | **id + timing** (+11.8 s) | VM 22:54:20.270, LOCAL 22:54:20.272 | no |

Notes that matter:

- **The reporting host never receives its own toll.** B5, B6 (LOCAL reporting) produced no toll to
  LOCAL; B10 (VM reporting) produced none to VM. Three for three. There is no missing hearer row
  for those machines — they are correctly absent.
- **The `0x03EE` → `0x03EF` relay is 94–98 ms**, three samples, all consistent.
- **A double ring exists.** B5 and B6 are two separate `0x03EE` from the same host 11.7 s apart
  after a single death. That is the host's client sending twice, not a duplicate capture — different
  message indices (99 and 105) and different framing.
- **Five bells (B7, B8, B9, B11, B12) rang in worlds we were invading, with no `0x03ED` from us.**
  The most economical reading is that a second covenant defender killed the host and pulled the
  lever, but **the capture cannot see other phantoms**, so this is not established.
- **B1/B2's reporting host 3241000 has no recoverable PSN id.** Checked: every `PUSH_0x03CC`,
  `PUSH_0x03CD`, `PUSH_0x03CE`, `PUSH_0x03FB`, `PUSH_0x03B9`, all six `0x0386` login responses, and
  all 430 `RequestGetVisitorListResponse` `VisitorData` entries. 3241000 appears in none of them.
  It is **NOT RECOVERABLE**.

---

## 5. Hearer location vs bell origin

One row per (bell, hearing machine) pair, 18 rows, sorted by time.

**Reading the origin column.** Where our machine was the reporting host, the origin is that
machine's own bracketed position — a true, captured coordinate. Where our machine was a phantom in
the ringing world, the origin is that machine's own in-world position, which §1.5 establishes is a
position inside the host's world; it is a proxy for the belfry, not the lever itself. Where neither
applies (B1, B2), the origin is known **only to map granularity** and the cell column says so.

**Reading the distance column.** Computed only when both endpoints are captured coordinates, and the
pair being measured is named. For B5/B6/B10 both endpoints are inside a single world — a true
in-world distance. For B8/B9/B11/B12/B13 the two endpoints are in *different world instances of the
same map*; the map geometry is shared, so the number answers "how far from the bell's location was
the hearer standing", but it is not two players in one world. Where only one endpoint is known the
distance is **not computable** and no cell id was substituted for a coordinate.

| bell | hearer | hearer area (age of explicit report) | hearer cell (age) | hearer position (age) | after-fix | bell map | origin cell + position | same map? | same cell? | distance |
|---|---|---|---|---|---|---|---|---|---|---|
| B1 05:41:17 | LOCAL | 10160000 (−426 s) | 101630 (−426 s) | (−184.4, 7.2, 534.3) (−418 s) | pos (−184.4, 7.2, 533.8) @ +354 s, area/cell not restated | 10160000 | **unknown — map granularity only** | **yes** | unknown | not computable |
| B2 05:41:29 | LOCAL | 10160000 (−438 s) | 101630 (−438 s) | (−184.4, 7.2, 534.3) (−431 s) | pos (−184.4, 7.2, 533.8) @ +341 s | 10160000 | **unknown — map granularity only** | **yes** | unknown | not computable |
| B3 09:12:16 | LOCAL | 10190000 (−468 s) | 101950 (−114 s) | (−812.3, 204.3, 645.6) (−8.0 s) | cell 101910, (−606.5, 172.7, 604.3) @ +3.8 s | 10190000 | LOCAL was itself the phantom in 3468141's world — same point | **yes** | yes (hearer is the in-world machine) | 0 by construction |
| B4 09:47:24 | VM | 10160000 (−151 s) | 101640 (−105 s) | (−180.0, 23.2, 508.2) (−4.2 s) | (−184.7, 23.2, 510.7) @ +0.6 s | 10160000 | VM was itself the phantom in 2350487's world | **yes** | yes (in-world machine) | 0 by construction |
| B5 18:21:39 | VM | 10160000 (−28 s) | 101640 (−28 s) | (−185.5, 23.2, 510.1) (−3.3 s) | (−185.1, 23.2, 510.7) @ +2.2 s | 10160000 | **101640, (−185.2, 23.2, 507.0)** — the reporting host LOCAL, 12.2 s stale | **yes** | **yes** | **3.1** (VM ↔ reporting host LOCAL, one world) |
| B6 18:21:51 | VM | 10160000 (−40 s) | 101640 (−40 s) | (−185.1, 23.2, 510.7) (−9.5 s) | cell 101630, (−186.3, 7.2, 535.1) @ +6.7 s | 10160000 | **101640, (−185.2, 23.2, 507.0)** — LOCAL, 24.0 s stale | **yes** | **yes** | **3.7** (VM ↔ reporting host LOCAL, one world) |
| B7 18:32:48 | LOCAL | 10190000 (−29 s) | 101950 (−29 s) | (−826.5, 203.7, 641.1) (−5.3 s) | **area 10160000 cell 101630**, (−186.7, 7.2, 535.2) @ +9.8 s | 10190000 | LOCAL was itself the phantom in 3207541's world | **yes** (the before-fix; the after-fix is a different map, but it is 9.8 s later and LOCAL left the session at 18:32:52 — the bell is inside the Iron Keep bracket) | yes (in-world machine) | 0 by construction |
| B8 21:02:17 | VM | 10160000 (−156 s) | 101640 (−15 s) | (−185.3, 14.9, 516.5) (−10.2 s) | (−184.1, 14.9, 515.9) @ +7.7 s | 10160000 | **101640, (−177.9, 23.2, 509.5)** — LOCAL, in-world phantom, 4.7 s stale | **yes** | **yes** | **13.2** (VM ↔ LOCAL, different world instances) |
| B8 21:02:17 | LOCAL | 10160000 (−92 s) | 101640 (−92 s) | (−177.9, 23.2, 509.5) (−4.7 s) | (−184.7, 23.2, 510.7) @ +0.6 s | 10160000 | LOCAL was itself the in-world phantom | **yes** | yes (in-world machine) | 0 by construction |
| B9 21:24:00 | VM | 10160000 (−29 s) | 101640 (−24 s) | (−185.1, 23.2, 510.7) (−2.1 s) | cell 101630, (−186.6, 7.2, 539.8) @ +9.8 s | 10160000 | VM was itself the in-world phantom in 3428694's world | **yes** | yes (in-world machine) | 0 by construction |
| B9 21:24:00 | **LOCAL** | 10160000 (−1395 s) | **101630 (−1287 s)** | (−191.7, 7.2, 534.3) (−1245.5 s) | (−192.1, 7.2, 535.0) @ **+28.2 s**, then cell 0 @ +38 s and cell 101640 @ +43 s | 10160000 | **101640, (−185.1, 23.2, 510.7)** — VM in-world | **yes** | **NO — 101630 vs 101640** | **29.3** (LOCAL ↔ VM, different world instances) |
| B10 22:15:43 | LOCAL | 10160000 (−581 s) | 101640 (−581 s) | (−186.5, 23.2, 511.6) (−190.0 s) | identical position @ **+1.6 s**; cell 101630 restated @ +13.4 s; area 10190000 only at +75 s | 10160000 | **101640, (−184.6, 23.2, 510.0)** — the reporting host VM, 19.3 s stale | **yes** | **yes** | **2.5** (LOCAL ↔ reporting host VM, different world instances) |
| B11 22:23:57 | VM | 10190000 (−426 s) | 101950 (−41 s) | (−820.1, 215.3, 632.5) (−9.0 s) | cell 101910, (−582.0, 165.6, 592.1) @ +15.3 s | 10190000 | VM was itself the in-world phantom in 3471010's world | **yes** | yes (in-world machine) | 0 by construction |
| B11 22:23:57 | **LOCAL** | 10190000 (−419 s) | **101910 (−419 s)** | (−590.4, 173.3, 603.7) (−38.9 s) | cell 101950, (−773.0, 203.7, 638.6) @ +48.1 s | 10190000 | **101950, (−820.1, 215.3, 632.5)** — VM in-world | **yes** | **NO — 101910 vs 101950** | **235.3** (LOCAL ↔ VM, different world instances) — **best-evidenced long-range hearing: both endpoints fresh (9 s and 39 s)** |
| B12 22:39:58 | **VM** | 10190000 (−1388 s) | **101910 (−946 s)** | (−587.3, 173.0, 603.3) (**−696.4 s**) | **none — VM reported no further location in the whole capture** | 10190000 | **101950, (−837.0, 203.7, 639.7)** — LOCAL in-world, 0.0 s stale | **yes** (last known) | **NO — 101910 vs 101950** | **254.1**, low confidence (one endpoint 11.6 min stale, unbracketed on the after side) |
| B12 22:39:58 | LOCAL | 10190000 (−18 s) | 101950 (−18 s) | (−837.0, 203.7, 639.7) (−0.0 s) | (−789.4, 208.1, 626.8) @ +9.9 s | 10190000 | LOCAL was itself the in-world phantom | **yes** | yes (in-world machine) | 0 by construction |
| B13 22:54:20 | **VM** | 10190000 (−2249 s) | **101910 (−1808 s)** | (−587.3, 173.0, 603.3) (**−1557.9 s**) | **none** | 10190000 | **101950, (−826.9, 203.7, 644.2)** — LOCAL in-world | **yes** (last known) | **NO — 101910 vs 101950** | **245.0**, low confidence (one endpoint 26 min stale, unbracketed) |
| B13 22:54:20 | LOCAL | 10190000 (−499 s) | 101950 (−499 s) | (−826.9, 203.7, 644.2) (−5.8 s) | (−831.1, 203.7, 636.0) @ +24.9 s | 10190000 | LOCAL was itself the in-world phantom | **yes** | yes (in-world machine) | 0 by construction |

### 5.1 What this set shows

- **18 of 18 hearings were in the bell's map.** No hearer was ever outside it. Nothing in this
  corpus requires cross-map delivery, and nothing rules it out either — the negative case for it
  is in §5.2, not here.
- **Same cell: 12 of 18. Different cell: 4 of 18. Unknown: 2 of 18** (B1, B2 — origin known only to
  map). The four different-cell rows are B9/LOCAL, B11/LOCAL, B12/VM, B13/VM.
- **Distances where computable:** 2.5, 3.1, 3.7, 13.2, 29.3, 235.3, 254.1, 245.0 (units are the
  game's world units). Eight of the eighteen rows; the other ten are either 0-by-construction (the
  hearer *is* the in-world machine) or not computable (B1, B2).
- **The largest distance at which a toll was still heard is 254.1** (B12, VM at Belfry Sol hearing
  a bell whose origin was the Threshold Bridge). That row's hearer endpoint is 11.6 minutes stale,
  so **the strongest well-bracketed long-range hearing is B11 at 235.3**, where the hearer's
  position was 39 s old and the origin 9 s old. Either way, tolls plainly cross from cell 101950 to
  cell 101910 within Iron Keep — a distance of roughly 240 units and, in the map, from the belfry
  approach out to the Threshold Bridge bonfire.
- **The three cases where our own machine was the reporting host are the tightest measurements in
  the corpus** (2.5, 3.1, 3.7) and are the only true single-world distances. They confirm the
  exclusion rule from both ends: the ringer's own client got nothing while a player three units
  away got the toll.
- **The Belfry Sol picture is consistently "Iron Keep, not the belfry room".** Every Iron Keep bell
  we can localise (B3, B7, B11, B12, B13) had its in-world origin in **cell 101950** around
  x ≈ −773…−837, not in the belfry cell 101910 at x ≈ −582. The recurring summon spawn
  `(−773.0, 203.7, 638.6)` is in 101950. So "Belfry Sol's bell" is being rung by trespassers who
  were fought at the Threshold Bridge, and it is heard ~240 units away at the bonfire.

### 5.2 Two bells that a machine standing in the right map did NOT receive

This is the most important negative in the corpus and it **contradicts the rule this document was
asked to encode** ("everyone else in the bell's AREA does" receive it). Both cases are in R1.

| bell | map | machine that did not hear it | its bracketed location | in the bell's map? |
|---|---|---|---|---|
| **B3** 09:12:16 | 10190000 | **VM** | last located fix 09:10:05 = area 10190000, cell 101910, `(−578.5, 171.5, 604.5)` — 131.8 s before. VM sent status updates at 09:11:05, 09:11:25, 09:11:45, 09:12:05, 09:12:25, 09:12:45, 09:13:05… **all with no location block**, i.e. it reported no movement. | **yes — Belfry Sol** |
| **B4** 09:47:24 | 10160000 | **LOCAL** | **position** fix 09:47:09, **15.6 s** before, `(−195.2, 7.2, 538.4)`; that update is position-only and carries NO cell — the last explicit **cell** report (101630) is 143.8 s before, and the area 10160000 at 09:45:00. The position is ~11 units from the confirmed Luna position, so the location holds; the ages differ by field. | **yes — Belfry Luna** |

B4 is the cleaner one: LOCAL was standing in Belfry Luna, with a 15-second-old fix, when a Belfry
Luna bell rang, and received nothing. Meanwhile in R2 there are **six** unambiguous bystander
deliveries — B8/VM, B9/LOCAL, B10/LOCAL, B11/LOCAL, B12/VM, B13/VM — where the hearer was in the
map but **not** in the ringing host's session, and did receive it. And B1/B2 in R1 are themselves
bystander deliveries to LOCAL, so R1 is not uniformly different.

**This is not resolved by anything in the corpus.** Candidate explanations, none tested:

1. The server's view of a player's area goes stale or unset when the client stops sending
   `PlayerLocation`, and delivery fails closed. Fits B3 (131 s with no location block); **does not
   fit B4** (15 s old, and the position was reported).
2. The predicate is finer than "map" — a region, an activity cell group, or a matchmaking band. But
   B11/B12/B13 deliver across 235–254 units and across cells 101910↔101950, so any such region is
   wide.
3. Loss in the R1 capture. The R1 sessions received `PUSH_0x03CC` normally throughout (11 pushes),
   so push capture was working, but a single dropped datagram cannot be excluded.
4. Server behaviour differed between 09:xx and 18:xx–22:xx.
5. **Delivery is scoped to the world, not the map.** Added after the fact — the task brief told me
   session membership was not the predicate and to frame nothing around it, so this was not listed.
   It fits B4 exactly: VM was the phantom **inside** 2350487's world and heard the toll; LOCAL was
   in its own Belfry Luna and did not. It does **not** fit B8/VM, B9/LOCAL, B11/LOCAL, B12/VM or
   B13/VM, each of which was in a *different* world from the ringing host and still received it.
   So world-scoping alone is not the answer either, but it should not have been excluded.

Stated plainly: **the corpus contains both a clean positive and a clean negative for
"everyone in the bell's map hears it", and it cannot say which condition separates them.**

---

## 6. Summary tables

### 6.1 Invasions by role and outcome

Per **event** (64). Self-invasions are counted once on each side because each side is a separate
observation of a separate client.

| role | killed the other party | died | neither observed | total |
|---|---|---|---|---|
| INVADED THEM (phantom, `0x03EA`) | **14** | **13** | 21 | **48** |
| INVADED (host, `0x03E8`) | **0** | **11** ¹ | 5 | **16** |
| **total** | 14 | 24 | 26 | **64** |

¹ 11 host-side events sit on **10 distinct deaths** — rows 51 and 52 share the 21:42:25 death. One
of those 11 (row 55) is a PvE death to enemy 753003 while a guest happened to be present, so
**at most 9 host-side deaths were caused by the guest, and one of those 9 is un-attributable
between two simultaneous guests.**

We sent **zero** `0x03ED` naming a guest in our own world: across 16 invasions of our worlds our
machines never killed the invader once.

Per **distinct invasion** (58): 52 third-party, 6 self.

| | events | distinct invasions |
|---|---|---|
| self-invasion (LOCAL hosting VM, all R2) | 12 (6 pairs) | 6 |
| third-party | 52 | 52 |
| **total** | **64** | **58** |

By batch: R1 = 9 events, all phantom, all third-party, 0 self.
R2 = 55 events (16 host + 39 phantom), 6 self-invasion pairs, 49 distinct invasions.

### 6.2 Kills followed by a bell, vs not

| | n | bell within 60 s naming the victim |
|---|---|---|
| our kills as phantom (`0x03ED`) | 14 | **4** (09:11:58→B3 +18.1 s; 09:47:14→B4 +9.7 s; 18:21:33→B5 +5.6 s and B6 +17.3 s; 22:54:08→B13 +11.8 s) |
| our kills as host | 0 | — |
| host-side deaths caused by a guest (≤ 9, see §6.1) | ≤ 9 | **2** (18:21:33→our own `0x03EE` ×2; 22:15:29→our own `0x03EE`) |

Both directions agree: roughly a quarter to a third of player kills in a Bell Keeper session were
followed by a bell. Five of the thirteen bells followed a kill we recorded; five more came from
worlds we were in but did not kill in; three came from worlds we can say nothing about.

**Whether the lever was pulled is invisible to the protocol.** Nothing is sent between the kill and
the `0x03EE` in any of the three double-ended cases, so "the lever was never touched" and "the
lever was touched" look identical on the wire except for the presence of the `0x03EE` itself.

**The player's own account explains the ratio, and it is consistent with everything here.** In many
successful invasions the target ran, the fight was carried far from the belfry, and there was no
time to get back to the lever before the session ended. A kill is not a bell. That is unfalsifiable
from the wire — which is exactly the point of the paragraph above — but it needs no mechanism this
document is missing, and the session durations support it: the kills that produced a bell sat
9.7–18.1 s before the toll, while several silent kills came at the very end of a session.

### 6.3 Bells by whether one of our machines was in the ringing world

| | n | which |
|---|---|---|
| our machine **was the reporting host** | **3** | B5, B6 (LOCAL), B10 (VM) |
| our machine was a **phantom in that world** | **8** | B3, B4, B7, B8, B9, B11, B12, B13 |
| **neither** — pure third-party world | **2** | B1, B2 (host 3241000) |
| total | **13** | |

### 6.4 Hearings vs the hearer's map

| | n |
|---|---|
| hearings where the hearer was in the bell's map | **18 of 18** |
| hearings where the hearer was in the bell's map **and** the same cell as the origin | 12 |
| hearings in the bell's map but a **different cell** from the origin | 4 |
| hearings where the origin cell is unknown (map granularity only) | 2 |
| hearings outside the bell's map | **0** |
| **machines that were in the bell's map and did NOT receive it** | **2** (B3/VM, B4/LOCAL — see §5.2) |
| machines in a **different** map that did not receive it | 1 (B7/VM, in Luna during an Iron Keep bell — consistent with a map filter) |

By map: **8 Belfry Luna bells** (B1, B2, B4, B5, B6, B8, B9, B10) carrying `10160000`, and
**5 Iron Keep / Belfry Sol bells** (B3, B7, B11, B12, B13) carrying `10190000`. 8 + 5 = 13.
Deliveries split 10 Luna / 8 Iron Keep, which also sums to 18.

---

## 7. ID index

Scoped deliberately to **invasion, visit and bell traffic** — `0x03E8/E9/EA/EB/ED/EF`,
`PUSH_0x03CC/CD/CE`, `0x03D5` responses, `0x03D6/D7`, `PUSH_0x03FB`, `PUSH_0x03B9`, and the
`0x0386` login responses. Player ids that appear **only** as blood-message, bloodstain, sign or
ghost authors are excluded: there are thousands of them, they carry no invasion role, and mixing
them in would make this table useless. PSN ids come from the id/PSN pairs those messages carry.

"we invaded" = they were the host in one of our `0x03EA`. "invaded us" = they were the guest in one
of our `0x03E8`. "bells" = `0x03EF` events naming them as reporting host. "map/cell" is what the
corpus attributes to them; a **†** means the cell comes from a `0x03CC`/`0x03D6` and is therefore
**their registered cell, not a belfry** (see §1.6).

| player id | ours? | we invaded them | they invaded us | we killed them | bells reported | visit pushes naming them | maps / cells attributed | PSN id |
|---|---|---|---|---|---|---|---|---|
| 613004 | | 0 | 0 | 0 | 0 | 1 `0x03CC`, we rejected (`0x03D7` 08:42:31) | — | `0110000104bb6862` |
| 649074 | | 0 | 1 | 0 | 0 | 1 `0x03D6`, 1 `0x03CE` | 10160000 / 101630† | `0110000103c39338` |
| 735037 | | 0 | 1 | 0 | 0 | 1 `0x03D6`, 1 `0x03CE` | **10170000 / 101730†** (registered in Sinners' Rise, joined a Luna world) | `01100001085fdd8d` |
| 1194297 | | 2 | 0 | 1 | 0 | 2 `0x03CC` | Luna (from our in-world position) | `0110000103ef22d9` |
| 1321650 | | 1 | 0 | 0 | **1** (B8) | 1 `0x03CC` | 10160000 | `0110000119ece636` |
| 1616207 | | 1 | 0 | 0 | 0 | 1 `0x03CC` | Luna | `0110000106502d1b` |
| 1647744 | | 1 | 0 | 1 | 0 | 1 `0x03CC` | Luna | `011000011a99dff9` |
| 1665540 | | 1 | 0 | 1 | 0 | 1 `0x03CC` | Luna | `0110000119f6a1f1` |
| 1910304 | | 1 | 0 | 0 | 0 | 1 `0x03CC` | Luna | `011000011caecc59` |
| 1931731 | | 0 | **2** | 0 | 0 | 2 `0x03D6`, 2 `0x03CE` | 10160000 / 101620†, 101640† | `0110000142af9262` |
| 2113737 | | 1 | 0 | 0 | 0 | 1 `0x03CC` | Iron Keep | `0110000106b64256` |
| 2140716 | | 0 | 1 | 0 | 0 | 1 `0x03D6`, 1 `0x03CE` | 10160000 / 101660† | `011000014686c072` |
| 2350487 | | 1 | 0 | 1 | **1** (B4) | 1 `0x03CC` | 10160000 | `011000011599661a` |
| 2487254 | | 1 | 0 | 0 | 0 | 1 `0x03CC` | Luna | `011000014c88083b` |
| 2487834 | | 1 | 0 | 1 | 0 | 1 `0x03CC` | Luna | `011000011212bff7` |
| **2512541** | | **1** | **2** | 0 | 0 | 2 `0x03CC`, 2 `0x03D6`, 1 `0x03D7`, 2 `0x03CE` | 10190000 / 101950† | `0110000136ff6938` |
| 2657983 | | 1 | 0 | 0 | 0 | 1 `0x03CC` | Iron Keep | `01100001475057fe` |
| 2660247 | | 0 | 1 (break-in) | 0 | 0 | none — arrived via `PUSH_0x03FB` | 10160000 / 101640 | `0110000153d717a7` |
| 2712542 | | 2 | 0 | 0 | **1** (B12) | 2 `0x03CC` | 10190000 | `011000010551afcf` |
| **2910025** | **ours — LOCAL, both batches** | 6 (as host, from VM's side) | 0 | — | **2** (B5, B6) | 7 `0x03CC` to VM, 1 `0x03D7` | 10160000, 10190000 | `01100001000e538e` |
| 2973239 | | 0 | **2** (join kind 5) | 0 | 0 | none — arrived via `PUSH_0x03B9` | 10160000 / 101630, 101640 | `011000016d3ae342` |
| 3066937 | | 1 | 0 | 0 | 0 | 1 `0x03CC` | Iron Keep | `011000011c65684c` |
| 3112652 | | 1 | 0 | 1 | 0 | 1 `0x03CC` | Luna | `011000015876dfad` |
| 3161263 | | 1 | 0 | 1 | **1** (B13) | 1 `0x03CC` | 10190000 | `01100001050b29e8` |
| 3207541 | | 1 | 0 | 0 | **1** (B7) | 1 `0x03CC` | 10190000 | `0110000144520b73` |
| 3241000 | | 0 | 0 | 0 | **2** (B1, B2) | 0 | 10160000 only | **NOT RECOVERABLE** |
| 3428694 | | 4 | 0 | 0 | **1** (B9) | 4 `0x03CC` | 10160000 | `0110000104f02a47` |
| 3428695 | | **7** | 0 | **3** | 0 | 7 `0x03CC` | Luna | `01100001009dc945` |
| 3446575 | | 2 | 0 | 1 | 0 | 2 `0x03CC` | Luna | `011000014a5e02e0` |
| 3457842 | | 0 | 0 | 0 | 0 | 1 `0x03CC` — never joined, never rejected | — | `01100001178768ea` |
| 3459848 | | 1 | 0 | 0 | 0 | 1 `0x03CC` | Luna | `011000011a5dc8e2` |
| 3468141 | | 1 | 0 | 1 | **1** (B3) | 1 `0x03CC` | 10190000 | `011000013d05b244` |
| 3469576 | | 3 | 0 | 0 | 0 | 3 `0x03CC` | Luna | `011000013e6c6d52` |
| 3470456 | | 2 | 0 | 0 | 0 | 2 `0x03CC` | Iron Keep | `011000015cb52790` |
| 3471010 | | 1 | 0 | 0 | **1** (B11) | 1 `0x03CC` | 10190000 | `0110000172cf4978` |
| 3471570 | | 1 | 0 | 1 | 0 | 1 `0x03CC` | Iron Keep | `011000010b6c85ea` |
| 3472879 | | 1 | 0 | 0 | 0 | 1 `0x03CC` | Luna | `011000011929d470` |
| **3473926** | **ours — VM, both batches** | 0 | 6 (as guest in LOCAL's world) | — | **1** (B10) | 7 `0x03D6`, 6 `0x03CE` | 10160000 / 101630, 101640 | `0110000129ca9838` |

38 distinct ids, 2 of them ours.

**Ids appearing in more than one role**

- **2512541** — the only stranger on both sides. LOCAL rejected their visit request at 22:28:38
  (`0x03D7`), summoned them into LOCAL's world at 22:34:26 and again at 22:43:43 (`0x03D6`, guest
  both times), and was itself summoned into **their** world at 22:40:41 (`0x03CC` → `0x03EA`,
  phantom). Guest and host inside nine minutes.
- **2910025** and **3473926** — ours, both roles, by construction.
- **1931731** was summoned into VM's world twice (21:41:47, 21:58:21) from two different registered
  cells (101640, then 101620).
- **735037** is the clearest illustration of §1.6: registered at `10170000 / 101730` (Sinners'
  Rise), summoned by VM, joined a world at map 10160000, and later killed VM — the death that
  produced B10.

**Ids that look related, and why that means nothing**

There is a dense cluster of ids in the 3.42M–3.47M band: 3428694, 3428695, 3446575, 3457842,
3459848, 3468141, 3469576, 3470456, 3471010, 3471570, 3472879 — and **our own VM, 3473926, sits
inside it**. Similarly 3428694 and 3428695 are literally consecutive and were both hosts our
machines invaded repeatedly from the same Luna 101630 pool.

**Consecutive or nearby player ids are NOT evidence of a relationship.** Our own two machines
(2910025 and 3473926) are 563,901 apart and are the same two people; VM's id lands in the middle of
a cluster of complete strangers. The ids are plainly issued in ascending order, so proximity means
only "registered around the same time". Nothing else in this corpus supports any link between
3428694 and 3428695 — no shared PSN prefix beyond the constant `01100001`, no correlated timing
beyond both being in the same belfry pool. Treat them as unrelated unless something independent
says otherwise.

---

## 8. What this cannot show

**Single-ended capture.** 52 of the 58 distinct invasions were against strangers whose clients were
never recorded. For those we see only our own side: our joins, our kills, our deaths, and whatever
pushes the server chose to send us. We cannot see a third party's `0x03EE`, another phantom in the
same world, the host's health, or whether anyone touched the lever. **The only invasions with both
ends are the six self-invasions**, and only three bells (B5, B6, B10) have a captured `0x03EE`.

**The lever is invisible.** No message is exchanged between the kill and the `0x03EE`. A lever that
was never pulled produces exactly the same wire trace as one that was pulled but whose bell we
missed. Every statement in §6.2 about "why only some kills produced a bell" is therefore a
correlation, not a mechanism.

**The R1/R2 bell-delivery contradiction (§5.2) is unresolved** and it contradicts the rule this
task was told to encode. Two R1 bells were not delivered to a machine standing in the bell's own
map — one of them with a 15-second-old position fix in the very belfry — while six R2 bells were
delivered to exactly that kind of bystander. Do not treat "everyone in the bell's map hears it" as
established by this corpus.

**Sampling bias.** Two people deliberately farming belfries for several hours. Consequences:
every one of the 430 `RequestGetVisitorList` requests is mode 1 (BellKeepers) — the corpus contains
no Blue Sentinel or Rat traffic at all, so nothing here says anything about those covenants. 61 of
64 invasion events are join kind 14 (2 are kind 5, 1 is kind 6). Every bell is Belfry Luna or
Iron Keep. Almost every location
fix is in one of five cells. The two R2 machines were often standing next to each other, which is
why five of the eight computable distances are under 30 units — that is our behaviour, not the
game's.

**The self-invasions are not independent observations.** Six invasions, twelve rows, but one pair of
players sitting in one room. They are the best evidence in the corpus for message semantics
(both ends visible) and the worst evidence for anything statistical.

**Locations are sampled, not continuous.** The client omits `PlayerLocation` entirely from many
status updates and omits area/cell from most of the rest. Four hearer rows rest on fixes more than
190 s old (B9/LOCAL at 1245 s, B10/LOCAL at 190 s, B12/VM at 696 s, B13/VM at 1558 s) and two of
those have **no after-fix at all** because VM reported no location for the last 29 minutes of its
session. Those rows are marked low confidence and their distances should not be quoted without the
caveat.

**Ambiguous events, listed rather than resolved:**

- **21:42:25** — VM died with two guests present (2140716 and 1931731). Which killed it is not
  determinable.
- **The two join-kind-5 events (18:55:37, 21:43:11, both guest 2973239).** The mechanic is not
  identified. `PUSH_0x03B9` (953) and the relayed push 955 are not in the project's proto file; the
  sign-family reading is inference from opcode adjacency, not evidence. Whether these were hostile
  at all is not established, though LOCAL died within two minutes of each.
- **B7, B8, B9, B11, B12** — bells from worlds we were in but did not kill in. Who killed the host,
  and whether anyone did, is unobserved.
- **Five host-side deaths with a guest present but no PvE marker** (18:57:22, 21:52:36, 22:35:44,
  22:44:50, and 20:16:52) are recorded as "we died" but `0x03F1` does not name a killer, so
  "killed by the guest" is an inference from co-presence, not a reading.
- **B1/B2's reporting host 3241000 has no recoverable identity** and no recoverable location; those
  two bells are anchored only by map.
- **`0x03E8` field 8 / `0x03F1` field 2** carry large values (4286579708, 4196345, 8390648, 3064…)
  that the project's proto labels `cell_id`. They are not six-digit cell ids and match the cell
  encoding used by the sign list instead. Left unread; nothing here depends on them.

**A protocol discrepancy worth recording.** `proto/DS2_Frpg2RequestMessage.proto` documents a PS3
ground-truth `RequestNotifyKillPlayer` as `f1 = 0, f2 = victim, f3 = 14 (session kind)`. **All 14
kills in this corpus are `f1 = 14, f2 = victim, f3 = 0`** — the session kind and the zero are
swapped relative to that note. Field 2 = victim is unaffected and is what every conclusion here
rests on, but the field-1/field-3 placement in the proto comment does not match this capture and
should not be relied on.

**Not covered.** Bell region width beyond Iron Keep's two cells; whether a Luna toll reaches Lost
Bastille (that observation lives in `tasks/bell-broadcast.md` and is not re-tested here); any
covenant other than Bell Keepers; anything about the PC build.
