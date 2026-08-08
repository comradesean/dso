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

**25,851 messages in 49 buckets** across three capture batches (2026-08-07/08), from 79 captures
totalling ~8 GB, with 3 failed datagrams in the whole set. Rebuild with `tools/pcap/ingest.sh <dir>` — it finds the key, the port and the
ring-buffer grouping itself, all three of which were re-derived by hand the first time.

**The message-offset bug is fixed (2026-08-07).** `protoStart` used to probe candidate offsets
beginning at **8**, which is the INDEX field — and four bytes of index parse as valid protobuf often
enough that the probe accepted them. **6,515 of 15,573 files, 42% of the corpus, were written with
the index prepended to their payload and a decoded tree built from it.** Verified after the fact:
for every one of those files the first four payload bytes read as a little-endian uint32 equal the
file's own `index:` header, 6,515/6,515.

The header is self-describing and never needed probing: `[0:4]` is its own size (12), `[4:8]` the
opcode, `[8:12]` the index, and a reply carries 16 more bytes and has a zero opcode. After the fix
every message lands at **12 or 28 and nothing else**, and `NO CLEAN PARSE` went from 5,305 to **0**.

`parsesCleanly` also rejected protobuf **groups** (wire types 3/4), which DS2 still uses. That left
136 messages unparseable; groups are now handled and rendered nested. One payoff:
`RequestGetRightMatchingArea` responses are readable for the first time and turn out to be repeated
`{area_id, player_count}` groups — live area populations from FromSoftware's own server, e.g.
`10190000: 11, 10170000: 4, 10140000: 2`.

**Every response is now identified.** The message Index at `full[8:12]` (little-endian, while the two
fields before it are big-endian) is echoed by the reply that answers a request, so responses — half
the corpus, and previously anonymous under opcode 0 — are paired to their requests and carry a
`replies-to:` line. 7,739 of 7,739 matched.

Superseded counts: 5,594 messages in 34 buckets (batch 1); 15,573 in 43 (batches 1-2).

**Batch 3 was gathered to answer one question and it did**: whether a bell rung in one belfry is
heard in the other. It is not — see `tasks/bell-broadcast.md`. 61 captures, 3 login groups, and the
`LOCAL` key is the same as batch 2's because the client never logged out between them; one
sub-directory is a genuine re-login with its own pair. `ingest.sh` sorts that out by verifying every
file rather than trusting the directory. Counted from `corpus/` on 2026-08-07;
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

## What the captures added to the protocol definitions (2026-08-08)

A sweep of every corpus bucket against what the server dispatches. Six push types now have
FromSoftware's own bytes behind them rather than inference:

| message | opcode | n | verdict |
|---|---|---|---|
| `PushRequestSummonSign` | `0x039B` | 1 | **confirmed field-for-field** |
| `PushRequestRemoveSign` | `0x039D` | 1 | **confirmed field-for-field** |
| `PushRequestBreakInTarget` | `0x03B9`, `0x03FB` | 7 | **confirmed field-for-field** |
| `PushRequestVisit` | `0x03CC` | 72 | **confirmed field-for-field** |
| `PushRequestRejectVisit` | `0x03CD` | 1 | **confirmed**, incl. the `unknown_3` slot (read 1) |
| `PushRequestRemoveVisitor` | `0x03CE` | 23 | **confirmed field-for-field** |

Nothing needed changing in any of them, which is the useful result: six definitions that were
schema guesses are now measurements.

**NEW — `BreakInType` has a value 4 that we did not have.** Break-in pushes arrived with `type = 0`
(six, on `0x03B9`) and `type = 4` (one, on `0x03FB`). The enum held only `RedEyeOrb = 0` and
`BlueEyeOrb = 2`, and 2 has never actually been observed. Added as `Unknown4` deliberately unnamed —
guessing it from the item list is how the existing unverified name got there.

**The opcode moves with the type**, which is the alias-block behaviour: type 0 on `0x03B9`, type 4 on
`0x03FB`.

**CORRECTION to `docs/protocol-map.md` — `0x03B9` is not `PushRequestRemoveVisitor`.** The PC map
lists it as that. Six live pushes carry six fields including an area and a cell;
`PushRequestRemoveVisitor` has four and neither. It is `PushRequestBreakInTarget`, and the matching
allow-relay is `0x03BB` = `0x03B9 + 2`, which fits the BreakIn block `base + 4*mode + role` with
role 2 = Allow. So the PC build uses the same `0x03B9` block PS3 does. This also retires
`tasks/invasion-timeline.md`'s "join kind 5 — mechanic not identified": it is a break-in of type 0.

**Sign ids run around 194 million** on FromSoftware's server (194224127, 194220913). Ours start at
100000. Nothing depends on the range, but it is worth knowing before assuming a client cares.

## Placeholder fields identified from the corpus

`tasks/unknown-fields.md` holds a row for every `unknown_N` / `field_N` in all three protos, swept
against all 25,851 messages: **39 identified, 21 suggestive, 49 no signal, 6 too few samples.** The
proto comments carry the evidence and sample counts; anything weaker than IDENTIFIED was left out of
the protos and listed as a proposal instead.

Worth knowing beyond the placeholders — a field that already had a confident NAME turned out wrong:
**`PlayerStatus.play_time_seconds` is not play time.** It reads 20200 in 169 of 169, which is the
`calibration_version`. The real play-time counter is `StatsInfo.unknown_18`, monotone across 2,170
samples at very nearly one per second. A named field is not a verified field.
