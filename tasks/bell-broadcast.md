# The belfry bell — what is settled, and the one thing that is not

**Status: working end to end, but one design decision rests on inference rather than
evidence.** Everything below marked CONFIRMED was either observed on the wire, read out of
the executable, or verified in game. The OPEN section is the part we should not pretend to
know.

---

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
  a PC client or some future consumer. Field 2 is kept as the ringer's map id on the same
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
