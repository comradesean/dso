# Live capture corpus — what FromSoftware's server actually sends

**Built 2026-08-07 from nine decrypted sessions** (seven local client, two VM), every message filed
by opcode under `corpus/` (git-ignored — it contains other players' Steam ids and character names,
so it must not be published).

```
python3 tools/pcap/udpdump.py <cap> --port <50000|50001> --tagged \
  | go run ./cmd/corpus -out corpus -session <label> -key <gamekey>
```

**~4,700 messages, 28 distinct opcodes, zero decryption failures.**

---

## The framing, recovered in full

This is the part that changes what we can read. `decodecap` looked at single datagrams and therefore
saw almost no server opcodes at all — a misleading picture that survived for hours.

**Fragment header, 12 bytes, inside the reliable-UDP body:**

```
[0:2]   message id, shared by every fragment of one message
[2:6]   flags; byte 2 non-zero => payload is zlib-compressed
[6:8]   TOTAL length across all fragments
[8]     0
[9]     fragment index
[10:12] this fragment's length
[12:]   payload; for a COMPRESSED fragment 0, a u32 inflated size comes first
```

Verified arithmetic: a message with `[6:8]=0x067c` (1660) arrived as fragments of 1008 and 652.

**The compression is zlib with a 4 KB window** (`CINFO=5`), so streams begin `58 c3` rather than the
familiar `78 9c`. That is a valid zlib header — `CM=8`, and the two bytes divide by 31 — but it does
not look like one at a glance, which is why it read as noise at first. `internal/frpg/rudp` already
handles this correctly.

**Message header is NOT fixed-size:**

| kind | header | opcode |
|---|---|---|
| request (c2s) | **12 bytes** | at `[4:8]` |
| response (s2c) | **28 bytes** | field is `0` |
| push (s2c) | 12 bytes | wrapper opcode `0x0320`, real id in protobuf field 1 |

`cmd/corpus` finds the boundary by trying candidate offsets and taking the first that parses cleanly
to the end of the buffer, then records which offset worked — so the layout was derived from the
corpus rather than assumed before reading it.

---

## What this confirms about our implementation

**Opcode numbering is identical.** All 28 observed opcodes land exactly where `docs/protocol-map.md`
says. DS2 SOTFS on PC shares the numbering we mapped from the PS3 binary.

**`PlayerStatus` matches field for field.** From a live `0x03B8`:

```
field 1  "Sean"        name
field 2  1             archetype
field 3  6             covenant        <- Bell Keepers, matching our constant
field 7  0             sitting_at_bonfire
field 11 0             human_effigy_burnt
field 12 50350000...   played_areas (repeated, four values)
field 15 108           soul_level
field 18 20200         play_time_seconds
```

Nested inside are position (`fixed32` floats), area `10160000`, cell `101630`, and sub-messages for
stats (`41,25,15,5,33,12,5,5,20` — the attribute spread) and equipment. **Our covenant constant
`6 = Bell Keepers` is confirmed against live data**, not just inferred from the PS3 client.

**`PushRequestVisit` matches field for field.** `0x3CC` (972), seen seven times, always at a belfry:
`push id, host player id, host Steam id (ASCII), player data blob, type=1, map id, cell id` — and it
confirms the belfry cell ids `101630` (Luna) and `101910` (Sol).

**`0x038C` is genuinely sent**, carrying a long element-id list — consistent with our reading of it
as upload scheduling, and a reason to leave it unimplemented rather than a reason to build it.

---

## What stands out

- **`RequestCreateGhostData` (`0x03B1`) is the second most common client message** — 548 across the
  corpus, behind only `RequestUpdatePlayerStatus` (486) and ahead of every list query. The client
  uploads replay data constantly. Worth knowing if we ever measure our own traffic against theirs.
- **The client polls hard.** Sign, bloodstain, blood-message and ghost lists each run 160-285 times
  across these sessions.
- **No `0x03EE` anywhere.** Neither client rang a bell during any captured session, so every bell
  observation here is of the RECEIVE side.
- **Responses carry opcode `0`** and are matched to requests by the message id in `[0:2]`, not by an
  echoed opcode. 1,925 of the corpus are these.
- **The game-service port varies per session** — `50000` or `50001`. Do not hardcode.

## Not established

- The 16 bytes between the opcode and the protobuf in a response header. `cmd/corpus` steps over
  them; nothing here says what they are.
- The meaning of most `PlayerStatus` sub-message fields beyond the ones named above.
- Whether `0x0320` is a push-specific wrapper or a general "server-originated message" opcode. Every
  push observed used it; no non-push used it.
