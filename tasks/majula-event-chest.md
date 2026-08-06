# The Majula event chest — SOLVED

**Status: SOLVED AND REPRODUCED LIVE (2026-08-06).** The chest reset visibly and handed over a
Torch, the prize sitting in stock 0114's `ItemLotParam2_SvrEvent[10045500]`.

The full chain, every step now observed rather than inferred:

```
server pushes 0x038B carrying a replacement OnlineEventParam.param,
  with version_required = the client's BUILD version (11500 = 1.15)
    -> listener at PushMessageManager+76 fires
    -> RegulationDiffHolder::Append queues the 88-byte record
    -> the per-frame applier reloads OnlineEventParam.param in place
    -> row 0, u16 at +2, goes 0 -> 1
    -> chest arm method 0x58E360: stored(0) >= threshold(1) is now FALSE
    -> chest arms and visibly resets
    -> claim reads ItemLotParam2_SvrEvent[10045500] -> Torch x1
```

Two open items closed by that run: the applier's resource repository **is** the one the chest's
threshold reader uses (they are reached through different globals and might not have been), and
**GATE A was already satisfied** — no separate condition to solve.

To change the prize, edit `ItemLotParam2_SvrEvent` row `10045500` and push that file too; the size
must stay identical, so rows can be rewritten but not added. To re-arm the chest afterwards, push a
higher threshold — the claim writes the threshold into the per-object counter, so each bump reopens
it exactly once. That is the weekly rotation, and it is now ours to drive.

See `tasks/regulation-push-038b.md` for the transport.

---

## How it was found (kept: the method matters)

**Status: the chest is identified, the prize lives in one param row, and the gate that keeps it
shut is a threshold in a second param row that is zero in every file we hold.**

---

## THE GATE (found 2026-08-06, supersedes the "next step" section below)

The live test was run: a calibration was authored putting Agape Ring (`42000000`) into
`ItemLotParam2_SvrEvent[10045500]`, signed, served, downloaded and installed — the client verified
our signature, proving the authoring pipeline works end to end. **The chest gave a Soul Vessel**,
i.e. lot `10045600`, the *ordinary* object's lot. The event object never participated.

Diffing both rows of `MapObjectInstanceParam_m10_04_00_00.param` (128 rows, table at `0x34`, stride
40) shows they are identical **except** the selector byte `+0x18` (`1c` vs `24`) and the lot id
`+0x24`. No enable flag, event-flag id, time window or condition field differs. **The gate is not in
the object data.** It is in the component code behind selector `0x24`.

### `0x58E360` — the arm/refresh method

```
this->armed = 0
r3 = *(world + 3368)
if (r3 && *(r3 + 34) == 0)        return          // GATE A - secondary, UNIDENTIFIED
threshold = OnlineEventParam[0].u16[1]            // bytes +2..+3, read via 0x66F1D8
stored    = 0x5B6318(mgr, objKey)                 // per-object u16, 0 by default
if (stored >= threshold)          return          // GATE B - 0 >= 0 is TRUE
0x58FF10(this, 1)                                 // enable the item
this->armed = 1
```

`0x58E228` (tick) requires `armed != 0`, and on claim calls `0x5B63D0(mgr, objKey, threshold)` —
**storing the threshold into the per-object counter**, so `stored >= threshold` holds forever after.

**That is a claim latch.** Bump the threshold and the chest re-arms exactly once. It accounts for
the weekly rotation, "once per account", "re-closes if you opened it before that week", and the NG+
reset, with **no network message involved at all**.

`OnlineEventParam` row 0 is `00000000000000000000000000000000` — byte-identical in all ten
calibrations and both disc regulations. Threshold = 0, so the chest can never arm. This predicts the
live result exactly.

### How `OnlineEventParam` was identified (traced, not inferred)

- `0xA5F484` = `GetOnlineEventParamPath()` → string `param:/OnlineEventParam.param` @ `0x1881208`
  (its only pointer slot, `0x1D26600`).
- Called from `0x66F6A0`, which stores the resource via `0x56A118` into **holder+24**
  (siblings: NetworkParam→+8, NetworkAreaParam→+16, NetworkMessageParam→+32).
- `0x66F1D8` reads `*(r4 + 24)` where `r4 = *(0x1E1EAB4 + 32)` — **the same slot**. Same compilation
  unit as `0x66F6A0`.
- `0x66F1D8` is a PARAM row-0 reader: `+10` = rowCount, `+68` = row[0].dataOffset, memcpy 16 bytes;
  zero-fills and returns untouched if the resource is missing, state `!= 2`, or rowCount `== 0`.
  Those offsets match §8 of `docs/regulation-format.md`.
- Corroboration: of the 12 params in the v1.10 regulation with 16-byte rows, `ONLINE_EVENT_PARAM` is
  the **only one with exactly one row at id 0**, and the string sits in the network param group.

**Confidence: high.**

### Still unknown

- **GATE A**, `*(world+3368)+34` — a second boolean, unidentified. May be an online/login flag. The
  chest needs it non-zero (or that object null).
- Whether the per-object counter (8 slots at `+4964..+4992`) persists in the save.
- The `+0x2C` `_SvrEvent` table reader. The earlier scan was **unsound**: the loader at `0x56A320` is
  reached via `0xA5EE4C` which does `addi r3,r3,12`, so relative to the repository pointer the slots
  are `+0x28/+0x30/+0x38`. Corrected and re-run, the scan found no `_Chr` or `_Other` reads either —
  and those must exist — so the seed does not survive to use sites. **Inconclusive, not negative.**

### The tension that still points at `0x038B`

`OnlineEventParam` row 0 **and** `ItemLotParam2_SvrEvent[10045500]` are static across all ten
published calibrations — including 0109 (2014-06-05) and 0110 (2014-07-08), which land on event
dates. Yet the event demonstrably ran. Both halves of the mechanism being frozen in the S3 channel
is **two** independent reasons to think the weekly data arrived by another route, and
`0x038B RegulationFileUpdatePushMessage` — inline `diff_data` — remains the only candidate in the
client's 214-class message set.

**That candidate is now traced end to end: see `tasks/regulation-push-038b.md`.** The client applies
pushed resources on the next frame, in memory, no restart — so FromSoftware could replace
`OnlineEventParam.param` at runtime and never touch the S3 calibration channel. The theory that
`start_at`/`end_at` scheduled the rotation is **unsupported but not refuted**: no reader was found,
and the search shape could not have seen the likely one. See that doc.

Constraint that matters here: the pushed payload must be **exactly the same size** as the loaded
resource. Flipping the `u16` at `OnlineEventParam` row 0 `+2` qualifies. Creating a missing row does
not.

### Key addresses (v1.10)

Component CU `0x58D960`–`0x58E4E0`; vtable `0x1CA0BC0`; class descriptor `0x1E1C5FC` (ptr at
`0x1E1C5F4`); register `0x58DD5C`; unregister `0x58DE3C`; tick/claim `0x58E228`; **arm/gate
`0x58E360`**; threshold reader `0x66F1D8`; counter read `0x5B6318` / write `0x5B63D0`; enable-item
`0x58FF10`; gate-A predicate `0xB5338C`.

---

## The chest

**Majula is `m10_04_00_00`.** (`10020000` is Things Betwixt — an earlier note had this wrong.)

The mansion holds **two chest objects at identical coordinates**, both model `o04_0230`, entries
#563/#564 of `map\m10_04_00_00\m10_04_00_00.msb`:

| object | MSB name | selector byte `+0x18[0]` | item lot `+0x24` | contents |
|---|---|---|---|---|
| `10045500` | `o04_0230_0000` | `0x1C` — shared with 167 objects | `10045600` | Soul Vessel ×1 |
| **`10045510`** | `o04_0230_0001` | **`0x24` — unique across all 3641 map objects** | **`10045500`** | see below |

Lot `10045500` exists in **two** tables:

| table | contents |
|---|---|
| `ItemLotParam2_Other` | Rubbish ×1 (`60510000`) |
| **`ItemLotParam2_SvrEvent`** | **Torch ×1 (`60420000`)** |

It is the **only** map-object lot id present in `_SvrEvent` — everything else there is `11000`–`11280`
or `9000000x`. So `10045510` is the event chest and `10045500` is the ordinary Soul Vessel one.

Supporting fact: `MapObjectInstanceParam.+0x24` is the item lot id, validated by 889 of 924
non-zero values across all maps resolving to real `ItemLotParam2_Other` rows.

## What this corrects

- **The chest is NOT `ResultEventParam`-gated.** All 82 rows read; none references either lot.
  That table binds the `11xxx` lots, which are post-session and covenant rewards — Awestone,
  Sunlight Medal, Token of Fidelity, Human Effigy, Bonfire Ascetic. The results screen, not a chest.
- **No scripting is involved.** The three ids appear in exactly two files across the whole v1.10
  archive set, in none of the 584 EzState scripts, and nowhere as immediates in the executable.
  Native component behaviour over param data.
- Earlier work concluded "the chest stays empty because the result event never fires". That was
  chasing the wrong table entirely.

## The prize never came through our calibration channel

`ItemLotParam2_SvrEvent[10045500]` is **Torch ×1 in all thirteen regulation files we hold** —
calibrations 0101 through 0114, disc v1.00, patch v1.00, and patch v1.10. The historical weekly
prizes (Petrified Something ×3, Twinkling Titanite ×2, Cracked Red Eye Orb ×5, Bonfire Ascetic ×4,
Smooth & Silky Stone ×5, Murakumo) are all trivially expressible as an `ITEM_LOT_PARAM2` row, so
the delivery vehicle was almost certainly **a regulation carrying a different row `10045500`** —
one we have never obtained.

## No message can grant an item

An exhaustive inventory of the client's protobuf classes — 214 of them, from the contiguous
`GetTypeName` string run at `0x190BF5F`–`0x190FBD1` — finds **no message anywhere carrying an item
id, a lot id, an event flag or a grant**. The only item-bearing message is `RequestNotifyBuyItem`,
which is client→server telemetry.

**So the only server→client path that can change the chest is a regulation swap.**

## The two pushes

### `0x038C PlayerInfoUploadConfigPushMessage` — ruled out, do not implement

```proto
1 push_message_id
2 config -> PlayerStatusUploadConfig { 1 element_id_list (repeated uint32), 2 upload_period }
3 char_data_upload_period
4 enemy_kill_upload_period          // all four required
```

Handler `0x15F4580`. It parses, copies the id list to a local vector, applies **lower bounds only**
(`upload_period >= 5`, `char_data >= 60`, `enemy_kill >= 5`), and makes one virtual call. No file
I/O, no crypto, no save data, no globals. It has no field capable of carrying an item, a file or a
flag. **It is upload scheduling and nothing else.**

Our proto's field names were the reference's guesses; the real ones are `element_id_list` and
`upload_period`, now fixed. There is no upper clamp.

### `0x038B RegulationFileUpdatePushMessage` — the standing candidate

```proto
1 push_message_id
2 update_msg -> RegulationFileUpdateMessage { 1 diff_data_list (repeated RegulationFileDiffData) }

RegulationFileDiffData:
  1 importance          2 target_regulation_version
  3 path (string)       4 diff_data (bytes)
  5 start_at (DateTime) 6 end_at (DateTime)      7 (varint, never read)
```

Handler `0x15F74B8`. **The diff bytes are carried inline** — no separate download is implied. The
handler defaults `start_at` to 2000-01-01 and `end_at` to 2100-01-01 via `cellRtc`, so the client
models **per-diff activation windows**, which is exactly the shape a weekly rotation would need.

It marshals into an 88-byte-per-entry vector and makes one virtual call to a listener at
manager+76. **What that listener does is unresolved** — its installer was not found, and that is a
filtered negative rather than an absence proof. Note the `0x0389` banner listener was also never
found and that push is confirmed working, so a missing installer means little.

**Also new: the client never sends `RequestGetRegulationFile`.** Its ctor `0x163A840` has zero
direct callers anywhere in `.text`, against a control test where `RegulationFileUpdateMessage`'s
ctor is correctly found being called from the `0x038B` handler. Consistent with the diff being
inline. Do not implement it.

## The per-account limit

`PlayerStatus` carries a **`regulation_version`** field, reported to us in `RequestUpdatePlayerStatus`
(`0x03B8`) — exactly what a server needs to decide whether to push a diff, and matching
`target_regulation_version` in the message.

Beyond that, **no message carries "already claimed"**. Two possibilities, neither provable from the
binary:

- **Server-side only** — the server stops serving the diff to that account. Needs no client support
  and is consistent with everything observed.
- **Local save flag** — "once per account" may be community shorthand for the save being
  account-locked.

## ~~NEXT STEP — the test that settles it~~ (DONE — result: Soul Vessel, see THE GATE above)

This test was run and came back "Soul Vessel", which the section below calls the "runtime gate
exists" branch. That gate has since been found: it is `OnlineEventParam` row 0. Kept for the record.

**Author a calibration changing only `ItemLotParam2_SvrEvent` row `10045500`** to something
unmistakable — Bonfire Ascetic (`60527000`) ×4 is one of the real historical prizes — leaving every
other byte identical. We can already author calibrations end to end; see
`tasks/calibration-reverse-engineering.md`.

Then open the chest.

- **Chest gives it** → the mechanism is entirely local regulation data, no server message is needed,
  and the question is closed. `0x038B` becomes only an optimisation (updating without a reboot).
- **Chest gives Rubbish or a Soul Vessel** → a runtime gate exists, and `0x038B` is next: its payload
  is now fully known, so set `start_at` in the past, `end_at` in the future, and
  `target_regulation_version` to whatever the client reports in `PlayerStatus`.

Two practical notes for the live test:

- Target object **`10045510`**, the far-side chest of the pair. `10045500` is the ordinary Soul
  Vessel one.
- Chest state resets on NG+ but **not** via Bonfire Ascetics, so use a character that has not
  opened it.

## Not established

- **The condition inside `MapObjSvrEventTreasureBoxComponent` that picks `_SvrEvent` over the
  default lot.** Traced as far as: class name string `0x1869818`, descriptor slot `0x1D08E70`,
  manager global `0x1E1C5F4`, accessor `0x58E048`, creator `0x58DA84`, and the item-lot repository
  loader `0x56A320` which stores the tables at `this+0x1C` (Chr), `+0x24` (Other), **`+0x2C`
  (SvrEvent)**. The code reading `+0x2C` was not found — a scan produced **885 candidates of which
  fewer than ten were triaged**, so that is a filtered negative, not an absence proof.
- Whether the `0x038B` listener actually applies a regulation.
- Whether selector byte `0x24` is literally a component-type enum. Its uniqueness is fact; its
  meaning is inference.
