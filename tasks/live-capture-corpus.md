# Live capture corpus — what FromSoftware's server actually sends

**Built 2026-08-07 from nine decrypted sessions** (seven local client, two VM), every message filed
by opcode under `corpus/` (git-ignored — it contains other players' Steam ids and character names,
so it must not be published).

```
python3 tools/pcap/udpdump.py <cap> --port <50000|50001> --tagged \
  | go run ./cmd/corpus -out corpus -session <label> -key <gamekey>
```

**Which key goes with which capture was never written down, and had to be re-derived.** Do not
guess it: `cmd/verifykey -keys <all candidate keys> -datagram <one c2s datagram>` reports the one
that authenticates, and the answer is not always the last key dumped. The captures and their keys,
as rebuilt on 2026-08-07 (sources in `Desktop/PACKETS`, outside the repo):

| session | capture | port | key |
|---|---|---|---|
| `LOCALa` | `LOCAL/ds2_00001_20260807000014` | 50000 | `4fc654ea…7ba2` |
| `LOCAL1`–`LOCAL6` | `LOCAL/ds2_0000{1..6}_2026080700461740…` | 50000 | `1de559fd…b977` |
| `VMrun1` | `VM/caps/run1/ds2_00001_20260807110512` | 50000 | `b02f96df…7944` |
| `VMrun2` | `VM/caps/run2/ds2_00001_20260807114956` | **50001** | `d07e5999…408d` |

**`LOCAL1` through `LOCAL6` are one login**, not six — dumpcap was writing a ring buffer, so they
share a key and a session. Messages whose fragments straddle a file boundary are lost at each seam,
because each file is assembled independently. `VMrun2` is on port **50001**, not 50000.

**5,594 messages in 34 buckets, zero decryption failures.** Counted from `corpus/` on 2026-08-07;
an earlier "~4,700 messages, 28 distinct opcodes" was an estimate and is superseded.

33 buckets carry an identified opcode. The 34th, `0x0000_unknown`, holds **2,787 messages — half
the corpus — and that is expected rather than a failure**: it is the server RESPONSES, whose
28-byte header carries no opcode at all (see the framing table below). They are matched to their
requests by message id, not by anything in themselves.

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

**Every message carries its capture time** (added 2026-08-07). The header gained:

```
time:      2026-08-07T04:46:54.537019Z  (1786078014.537019)
assembled: 0.001832s across fragments        <- only when it spanned datagrams
```

`tsLast` — the moment the message was complete — is what the `time:` line reports; `assembled:`
is the spread from the first fragment, printed only when there was one. Both come from the pcap's
own microsecond/nanosecond clock, so intervals are measured, not inferred from message order.

This existed in the capture all along and `--tagged` was discarding it. **It was also wrong when it
was printed at all**: `udpdump.py` assumed the pcapng default of microsecond resolution, and these
captures are NANOSECOND, so every timestamp it had ever shown was 1000x too large. `if_tsresol` is
now read from the IDB.

First thing it settles, against FromSoftware's own server rather than ours:

| poll | median interval |
|---|---|
| `0x03D5` visitor list (auto-summon) | **20.4 s** — confirms the ~20.5s figure measured from our own logs |
| `0x0397` sign list | 60.5 s |
| `0x03B8` status heartbeat | 16 s median, but hugely variable (p10 4.9s, p90 110s) — it is event-driven, not a timer |

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

**`PushRequestVisit` matches field for field.** `0x3CC` (972), seen **11 times**, always at a belfry:
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
