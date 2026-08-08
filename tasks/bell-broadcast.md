# The belfry bell — what is settled, and the one thing that is not

> **This file is about the BELL TOLL — who hears a bell that has been rung.** It is not about the
> Bell Keeper covenant, which is a different feature with a confusingly similar name: that is the
> auto-summon that pulls a grey spirit into a trespasser's world, and it lives in
> `docs/features.md` §6. This file's filter works on **area** ids (`bellRegions`, `telemetry.go`);
> the covenant's works on **cell** ids (`bellKeeperCells`, `matchmaking.go`). Conflating the two
> has already caused one wrong change.

**Status: working end to end, but one design decision rests on inference rather than
evidence.** Everything below marked CONFIRMED was either observed on the wire, read out of
the executable, or verified in game. The OPEN section is the part we should not pretend to
know.

---

## THE RING AND THE TOLL, BOTH SIDES CAPTURED (2026-08-07, second capture batch)

The second batch has **279 minutes of two machines recording FromSoftware's live server at once**,
and — for the first time — `0x03EE`, the ring itself. Eight distinct bell events, three rung by a
machine we were capturing. Machine identities come from the login response, not assumption:
**LOCAL = 2910025, VM = 3473926**.

### CONFIRMED — `0x03EE` is sent by the HOST, after they die

Not by whoever pulls the lever. All three captured bells that follow a death come from the player
who was **invaded** and **killed**, seconds later. Who *heard* it varies, and the distinction
matters — the rule is not "the killer hears it", it is "everyone in range except the host who sent
it":

```
18:21:33  SELF-INVASION. Our VM was the phantom in our LOCAL's world and killed it.
          18:21:39  LOCAL (host) sends 0x03EE.  VM, the killer, hears it.  LOCAL does not.

22:15:29  VM killed by an UNOBSERVED THIRD PARTY in VM's own world.
          22:15:43  VM (host) sends 0x03EE.  LOCAL hears it as an unrelated
                    BYSTANDER — LOCAL was not in that session at all.

22:54:08  LOCAL, as phantom, killed a third-party host.
          22:54:20  that host (3161263) REPORTS it.  LOCAL, who pulled the lever, hears it.
```

**Only the first is a self-invasion**, and it is the only session in this batch where both machines
were in the same world at a kill — 1 of 10 kills, 6 of 16 times we were invaded. The other two
cases involve third parties whose side was never captured, so they are single-ended.

Roles are read from the traffic, not assumed: `0x03E8 NotifyJoinGuestPlayer` means a guest entered
*my* world, so I am the host and I was invaded; `0x03EA NotifyJoinSession` means I entered someone
else's, so I am the phantom and I invaded.

**Nobody in this exchange is "the ringer" in the sense the field name suggests.** The INVADER
pulls the lever. The HOST's client is authoritative for its own world, so it is the one that
*reports* the ring with `0x03EE`, and `0x03EF` field 2 carries that reporting host's id. The
exclusion rule is therefore: the **reporting host** does not get the toll back. The invader who
actually rang it does — they are in the area like everyone else. Our `telemetry.go` excludes `from.playerID`, which is the sender of `0x03EE` — the same
player — so the behaviour matches, but the reason is not the one the code comment implies.

**A bell follows a kill only sometimes: 2 of 10.** Separated by role, out of 30 outcomes:

| role | outcome | n | followed by a bell |
|---|---|---|---|
| phantom (I invaded) | killed the host | 10 | **2** |
| phantom | died | 10 | 0 |
| host (I was invaded) | died | 10 | **2** (the same two, from the other side) |

**The player's account resolves this: the target often runs.** A successful kill frequently happens
far from the belfry, because the trespasser flees and the fight follows them, and the session ends
before the invader can get back to the lever. A kill is not a bell, and there is no missing
mechanism — the ratio is behavioural.

That is consistent with everything measurable here: the four kills that did produce a bell are
followed by the toll within 9.7-18.1 s, i.e. the lever was right there, while several silent kills
land at the very end of a session. It is not falsifiable from the wire, since nothing is sent
between the kill and the `0x03EE`.

Two alternatives were tested against the corpus first and both are refuted, which is what left the
behavioural explanation standing — so two alternatives were tested against the corpus and **both are
refuted**:

- **"Those were a different covenant."** No. Every visitor push in the batch is mode 1: `0x03CC`
  visit x41, `0x03CD` reject x1, `0x03CE` remove x13, and not a single Blue Sentinel (`0x03C9`-`CB`)
  or Rat (`0x03CF`-`D1`). All 426 `RequestGetVisitorList` requests carry type `BellKeepers`. Every
  one of the ten kills was preceded by a Bell Keeper visit push 45-170s earlier.
- **"The bell was already rung in that world."** No. Seven of the ten were the FIRST invasion of
  that host, and six of those seven were silent. The one host invaded repeatedly (3428695, three
  times) produced no bell even on the first.

So the difference is not the covenant and not a per-world cooldown. What remains is whether the
lever was actually pulled, which the protocol cannot see: nothing is sent between the kill and the
`0x03EE`, and a lever that is never touched looks exactly like one that is.

Incidentally this confirms field 2 of the visit push is the **host** player id — the self-invasion
at 18:21:33 names host 2910025, which is LOCAL.

### It broadcasts to the AREA AROUND THE BELL

Established from play, and every measurement in the corpus agrees. Recorded because two rounds of
analysis wandered off into whether the hearer was in the reporting host's session or invasion — they are
not the predicate, and testing that was a detour. A player hears a bell because of **where they are
standing**, not because of who was fighting whom.

Confirmed by measurement:

- **Field 3 of `0x03EF` is the map of the belfry that was rung** — `10160000` Luna, `10190000` Iron
  Keep/Sol. Every push carries it.
- **Every hearer was in the bell's map.** All 13 distinct events.
- **The one player in the wrong map did not hear it**: 18:32:48, a Sol bell, with the VM sitting in
  Luna throughout. That is the positive case.
- **It reaches well beyond the belfry itself.** The player who heard three Sol tolls was at the
  Threshold Bridge bonfire in Iron Keep (activity cell 101950), ~200 units from the belfry cell
  101910. Area, not room.
- **Session membership is irrelevant** — 7 of 15 hearings were from inside the ringing host's
  world and 8 from outside. Noted once so nobody re-tests it.

### The evidence behind it, and what is still unmeasured

> **Two earlier claims in this file were wrong and are withdrawn.** The first said FromSoftware
> filtered, on four pushes the two machines shared none of — weak reasoning that happened to point
> the right way. The second said tolls **cross** belfries, "confirmed". That came from taking the
> *nearest* status fix to each bell instead of bracketing it, and it does not survive the check.

Bracketing each bell with the last fix before and the first after:

| time | bell map | LOCAL | VM | verdict |
|---|---|---|---|---|
| 18:21:51 | Luna | Luna, **missed** | Luna, got | LOCAL is the reporting host |
| 18:32:48 | **Sol** | Sol at −29s, Luna at +10s, **got** | Luna, **missed** | VM in the wrong map missed it |
| 21:02:17 | Luna | Luna, got | Luna, got | both in map |
| 21:24:00 | Luna | Luna, got | Luna, got | both in map |
| 22:15:43 | Luna | Luna at −581s, Sol at +75s, got | Luna, **missed** | VM is the reporting host |
| 22:23:57 | Sol | Sol, got | Sol, got | both in map |
| 22:39:58 | Sol | Sol, got | Sol, got | both in map |
| 22:54:20 | Sol | Sol, got | Sol, got | both in map |

**Every one of the eight is consistent with "delivered to players in the bell's map, except the
reporting host."** Nothing requires cross-map delivery.

The case I had called proof of crossing was 22:15:43: LOCAL's nearest fix was Sol, 75s *after* the
bell — but the fix *before* it says Luna, and the reporting host moved to Sol in the same window, so both
players were plainly in Luna at the ring and travelled together afterwards. Using the nearest fix
rather than the bracket inverted the answer.

**18:32:48 is positive evidence for the filter**, and the only such case here: a Sol bell, with the
VM sitting in Luna throughout, and the VM did not receive it.

**What is still not established** is the *width* of the region. `bellRegions` sends a Luna toll to
Lost Bastille as well, on a first-batch observation of a player in m10_14 receiving one. Nothing in
this batch tests that, because every player in it was in a belfry.

## The chain, confirmed end to end

```
covenant defender is summoned into a belfry
  → defeats the HOST                      (the lever is not interactive before this)
  → pulls the lever
  → client sends 0x03EE  RequestNotifyRingBell
  → server relays 0x03EF  PushRequestNotifyRingBell
  → receiving client sets a latch; the loaded map's script plays ITS OWN bell
```

Each arrow was established separately:

- **The trigger** — the lever cannot be pulled until a covenant defender kills the host. The
  prompt does not exist before that. Confirmed by the player who did it, and it explains
  five earlier solo rings that produced nothing.
- **`0x03EE` is genuinely the bell** — `GetTypeName` returns
  `Frpg2RequestMessage.RequestNotifyRingBell` from this binary, not from the PC protos.
- **It is reachable in retail** — the send is gated on EzState command **130631**, which
  appears in exactly two of the game's 475 scripts: `event_m10_16_00_00.esd` (Belfry Luna)
  and `event_m10_19_00_00.esd` (Belfry Sol). Nowhere else.
- **The payload** — `08 <varint mapid> 12 00`, captured live as
  `08 80 8f ec 04 12 00` = map `10160000`, second field empty. This matched the shape
  recovered from the client's serialiser *before any frame had ever been seen*.

## `0x03EF` — the client reads none of it

The consumer was resolved by breakpoint at `0x15D0924` (read `CTR`), landing on a
pointer-to-member thunk at `0x68BCC8` and thence the real handler:

| | v1.10 | v1.00 |
|---|---|---|
| real consumer | `0x6E4F14` | `0x6AD174` |
| `SetPushSink` | `0x15CF28C` | `0x156514C` |
| `IsBellRung` (notifier `vt+0x90`) | `0x6E0228` | `0x6A83D8` |
| `ClearBellRung` (notifier `vt+0x94`) | `0x6E0248` | `0x6A83F8` |

That consumer takes the payload pointer and its size as arguments and **never references
either**, verified across the whole function in both builds. Its entire effect is:

```
this->0x10 = 1        // a boolean latch
```

A per-frame poll does TestAndClear and forwards it; the script in the loaded map asks
`IsBellRung()`. **So every field of the push is inert on PS3.** Field 1 is not read by the
handler, fields 2, 3 and 4 are not read by the consumer.

Tested rather than assumed: field 3 was set to Belfry Luna's cell (`101640`) instead of the
literal `0`, and nothing changed.

## Consequences worth remembering

- **The latch is a boolean, not a counter.** Several rings arriving between script polls
  collapse into a single toll.
- **The client cannot tell which bell rang.** Proven: with server-side filtering disabled, a
  player in Iron Keep heard **Belfry Sol's** bell while the toll carried Belfry Luna's map
  and cell. Whichever belfry map is loaded plays its own bell.
- **The client's own packet is sufficient to identify the bell.** The two belfries are in
  different maps (Luna `10160000`, Sol `10190000`), one belfry each, so the map id in
  `0x03EE` is unambiguous. No finer location is needed.

---

## ANSWERED: FromSoftware did NOT filter to the ringing bell's map

**Settled 2026-08-07 by reading a live push off FromSoftware's own server.** See
`tasks/pc-capture-decryption.md` for how the traffic was decrypted.

Two bells were rung in Belfry Luna by a second account. The listening client — standing in **Lost
Bastille (m10_14)**, not in either belfry — received both. Byte for byte, twice, identically:

```
s2c  ...0000 0320                 push wrapper opcode
     ffffffff
     08 ef07        field 1 = 1007 = 0x03EF   push id
     10 a8e8c501    field 2 = 3241000         the RINGER'S player id
     18 808fec04    field 3 = 10160000        Belfry Luna, the ringing map
     22 00          field 4 = empty
```

### What this establishes

- **`0x03EF` is real and FromSoftware sent it.** Not dead code, not a guess from the PS3 binary.
- **The field layout, which we had backwards.** Field 3 carries the map; field 2 is a
  server-assigned **player id**, not a map. Corrected in `broadcastBellToll`.
- **Field 2 is the HOST of the world the bell rang in** — the client that REPORTED the ring, not
  the player who pulled the lever, and not the recipient. This file
  said first. A second pair of captures settled it inside one session: a Bell Keeper visitor push
  (`0x3CC`) invited that client into host **2350487**'s world at map **10160000**, and the bell toll
  that followed carried `f2=2350487 f3=10160000`. Same host, same map. The receiving client's own id
  was 3473926, so it is not the recipient's either.
- **Pushes ride a wrapper opcode `0x0320`**, with the real push id in protobuf field 1. That is why
  an opcode table keyed on the message header mislabels every push.
- **The map filter was wrong.** A listener in `m10_14` received a toll for `m10_16`. Our filter
  dropped exactly that case, silencing a bell the real server delivered. Removed.

### The reasoning that was sound and still wrong

The filter existed because the client never reads the message body — it sets a boolean latch, and
whichever belfry map is loaded plays its own bell. So the server is the only place the decision
*can* be made, therefore FromSoftware must have made it there. Every step of that is true except the
conclusion. Worth keeping as a reminder that "it must have been done this way" survives right up
until someone reads the wire.

### The rule is REGIONAL, not per-map

The toll reaches the belfry's surrounding region, not just the belfry map:

- **Belfry Luna -> Lost Bastille.** Confirmed on the wire: the listening client was in Lost Bastille
  (`m10_14`) and received a toll carrying `10160000`.
- **Belfry Sol -> Iron Keep.** Reported from play by the person running these tests. A fourth toll,
  in a later session, carried `f3=10190000` (Sol) to a client reporting Sol-side areas — consistent
  with the regional model, though it does not discriminate further.

It is also not scoped to the fight. The listening client received a toll for a world it had no part
in — its whole session contained three pushes, one `0x038C` and the two bells, with **no `0x3CC`**,
so it was never summoned into that host's world. It got a bell for a stranger's fight.

**Exact boundaries are UNKNOWN.** How far each region extends, and how finely the game partitions
areas for this purpose, is not established by anything here. Do not guess at it in this file.

**Whether the covenant matters is UNTESTED.** No observation here bears on it either way.

### Both previous implementations were wrong, in opposite directions

1. **Filter to the ringing bell's map exactly.** Too narrow. The capture shows a Lost Bastille
   listener receiving a Luna toll; this dropped it.
2. **No filter at all.** Too wide, and worse — it reintroduces the **wrong bell**. The client cannot
   tell which bell rang, so a Luna toll delivered to someone standing in Belfry Sol makes them hear
   SOL. Observed on PS3 with filtering off: a player in Iron Keep heard Belfry Sol's bell from a
   toll carrying Belfry Luna's map. A missing packet is invisible; a bell ringing in the wrong
   belfry is not.

Now: a **regional** filter, `bellRegions` in `internal/server/game/telemetry.go`, holding only what
is established.

```
10160000 Belfry Luna -> {10160000, 10140000}   Lost Bastille CONFIRMED on the wire
10190000 Belfry Sol  -> {10190000}             Iron Keep reported from play, map id NOT established
```

**Iron Keep's map id is the missing piece.** It is deliberately absent rather than guessed. Fill it
without a rebuild via `DSO_BELL_REGION_10190000=10190000,<ironkeep>`.

An unknown listener area (profile not yet received) is **not** sent to. That is a reversal of the old
fail-open behaviour, and deliberate: a false positive rings the wrong bell where players can hear it,
a false negative costs one person one toll.

Until then we send to everyone, because an extra small packet is a cheaper mistake than a bell that
should have rung and did not.

### `0x03EE` is absent from every capture — because we never played the role that sends it

**Across nine decrypted sessions, ~4,700 messages, there is not one `0x03EE`.** Not from the client
that rang, not from either machine. Meanwhile four `0x03EF` tolls were received.

The sequence that explains it, from the VM's own session (`corpus/`, VMrun2 messages 849-858):

```
849  c2s  RequestNotifyKillPlayer (0x03ED)   the Bell Keeper kills the host
855  s2c  PUSH 0x03EF                        toll, f2 = that host's player id
858  c2s  RequestNotifyLeaveSession
```

No ring message between them. The server produced the toll from the kill.

On **PS3** the lever demonstrably sends `0x03EE` — confirmed live, `08 <varint mapid> 12 00` on the
wire. Nothing here contradicts that; these captures simply never include the sending side.

**The likely explanation: `0x03EE` is sent by the HOST, and neither captured client was ever a host
in a belfry.** The VM was always the summoned Bell Keeper; the local client was never in that role
either. So neither would ever send one, and its absence says nothing about whether the message is
used.

That also explains field 2 without any extra machinery: the toll carries the **host's** player id
because the server is relaying the id of whoever sent the ring. `f2=2350487 f3=10160000` is the host
of the world, which is exactly what a relayed sender id looks like.

I first read the absence as "the server derives the toll from the kill". That was an inference built
on a gap in the evidence, and the simpler reading — we never occupied the role that sends it — fits
better and needs nothing invented.

**To settle it:** capture from a client that is the HOST in a belfry when the bell rings. That side
has never been recorded.

Consequence for us: none yet. `broadcastBellToll` is driven by `handleNotifyRingBell`, which matches
the PS3 behaviour we observed directly.

### `0x3CC` (972) is the Bell Keeper visitor push, and OUR LAYOUT IS RIGHT

Seen seven times across the two VM sessions, always at a belfry:

```
f1 push id (972)      f2 host player id      f3 host Steam id, ASCII hex
f4 player data blob   f5 = 1                 f6 map id      f7 cell id
```

That is `PushMessageId, PlayerId, PlayerPsnId, PlayerStruct, Type, OnlineAreaId, CellId` — the exact
shape of our `PushRequestVisit`, confirmed against live traffic rather than inferred. All seven
carried `f6=10160000 f7=101630` (Belfry Luna) or `f6=10190000 f7=101910` (Belfry Sol), which also
confirms our belfry cell ids.

### Also seen in the same capture

`0x038C PlayerInfoUploadConfigPushMessage` is genuinely sent by FromSoftware, carrying a long
element-id list. We had concluded it was upload scheduling and deliberately left it unimplemented;
that reading stands, and it is now known to be live rather than vestigial.

---

## Superseded: the original open question

## OPEN: did FromSoftware's server filter, and how?

**This is the part we do not know, and the current implementation guesses.**

We currently send `0x03EF` only to players whose area matches the ringing bell's map,
failing open when a player's location is unknown. The reasoning was: the client cannot
distinguish bells, so the only place the decision *can* be made is the server, therefore
FromSoft must have made it there.

That is plausible and it is not evidence. It is exactly the kind of "must have been"
reasoning that has been wrong repeatedly on this project.

### A PC capture was tried, and CANNOT settle it (2026-08-06)

680 MB captured against FromSoftware's live DS2 SOTFS servers. **Result: inconclusive, and no further
analysis of those files will change that.** Two independent blockers:

- **The login/auth phase was not captured.** A 369-second gap between files swallowed the whole
  handshake. Zero Frpg2 packet-header signatures across 441,902 TCP segments with payload, and the
  first captured game datagram is mid-session (`packet_type = 0x00`, no connection prefix).
- **The key is unrecoverable even with it.** `RequestHandshake` carries the 16-byte CWC key
  **client->server under RSA-OAEP** to FromSoftware's public key (2048-bit, e=3, at file offset
  `0x10D3940` in `DarkSoulsII.exe`). Only their private key reverses it; e=3 buys nothing because
  OAEP is randomized and fills the modulus. Both directions switch to CWC *before* the game key is
  derived, so it is never in the clear. The one readable direction, server->client X9.31, carries
  only the auth server's address.

The envelope decode is CONFIRMED working — all 4,773 client->server datagrams carry the same
cleartext 8-byte auth token and the framing matches `dev/proto/pc/**/frpg2_game_*.ksy` — so the
transport read is right and only the payload is closed. 2.4 MB of ciphertext at 7.9999 bits/byte,
zero `F5 02` magic, zero occurrences of the bell payload or either belfry map id.

**The player's own bell ring is in there and could not be located either.** That is the positive
control failing, which makes this inconclusive rather than a negative — exactly the distinction this
file exists to keep straight.

Corrections from the capture: the PC game service is on UDP **`:50000`**, not `:50010` or `:50031`.

Settling it this way needs client-side key extraction — hooking the CWC cipher construction or
reading the 16-byte key from process memory at session start — paired with a capture started
*before* the game launches. Tooling is ready for that:
`scratchpad/pccap/pcapng.py` (dependency-free dissector) and `decode_game.py` (envelope decoder,
takes `--key`).

### What would settle it

**The discriminating question: did anyone in retail DS2 ever hear a belfry bell while
nowhere near a belfry?** That is only possible with no server filter.

- If yes → FromSoft broadcast more widely than we do, and our filter is wrong.
- If no → filtering is right, though the exact predicate (same map? same area? some
  radius?) is still unknown.

Community reports of hearing bells "mysteriously, without being invaded" do **not**
discriminate: a Luna ring reaching a Lost Bastille player is consistent with both.

### Other unknowns in the same area

- **Whether the polling script uses map or bell identity from its own local state.** That
  lives in the event-script data, not the executable, so the decompilation cannot reach it.
  It is the only remaining place a Luna/Sol distinction could hide.
- **A Sol ring has never been captured.** That it carries `10190000` is inference from the
  generic map-id helper the decomp traced — well-founded, unobserved.
- **Fields 3 and 4 have no known meaning.** They are inert on PS3, so this only matters for
  a PC client or some future consumer. Field 2 is kept as that map id on the same
  grounds.

### Current settings

```
DSO_BELL_BROADCAST=1        relay on
DSO_BELL_TEST_SECONDS=45    synthetic toll, TURN OFF when not testing
```

The synthetic toll broadcasts to **everyone** regardless of location, deliberately — that is
what made the client's behaviour observable, and adding the filter there would destroy the
experiment. The real relay is filtered.

---

## How this was found, worth keeping

Four theories about the trigger were proposed and falsified by live testing before the right
one: ghost replay, a first-ring one-shot, an active-session guard, and host death alone. The
decompilation was right about everything it could see and wrong twice about things it could
not — it declared the receive path dead because a store to `manager+0x80` was encoded as
`stw rS,0(rA)` after an `lwzu` advanced the base, so every search keyed on the displacement
missed it.

The pattern that worked: static analysis to find **where** to look, a breakpoint to read what
static analysis structurally cannot, and live play to decide which explanation survives.
