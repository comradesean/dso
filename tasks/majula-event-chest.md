# The Majula event chest — found, and one test away from settled

**Status: the chest is identified, the prize lives in one param row, and there is a cheap live
test that decides whether anything server-side is needed at all.**

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

## NEXT STEP — the test that settles it

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
