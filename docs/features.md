# What the player sees, and which opcodes make it happen

> **Purpose.** `docs/protocol-map-ps3.md` says what the wire carries. This document says what a
> person sitting in front of a PS3 is *doing* when that traffic happens, and whether `dso` answers.
> It is organised by feature, not by opcode, and it is deliberately blunt about where the mapping is
> inference rather than fact.
>
> **Target: original Dark Souls II on PS3, BLUS41045.** Not Scholar of the First Sin, not DS1, not
> DS3. Where an edition or patch changed a mechanic and it matters, it is called out.
>
> **One thing to get straight before reading:** the last old-gen patch, **1.10 (5 February 2015)**,
> shipped to PS3/360/DX9-PC and backported a pile of things people call "SotFS changes" — most
> importantly the **Agape Ring** and relaxed NG/NG+ matchmaking. `tools/rpcs3/dso.yml` carries a
> patch block for v1.10, so **our target can be either side of that line**. "SotFS-only" and
> "1.10-only" are different claims and this document keeps them apart.

## Rules this document follows

1. **The decompilation wins.** Player behaviour comes from the wikis; opcode facts come from
   `docs/protocol-map-ps3.md`. A wiki claim never promotes an opcode guess to a fact.
2. **`docs/protocol-map.md` (PC/SOTFS, DS3OS-derived) is not a source here.** It is wrong for PS3 in
   fourteen places, and six opcodes it lists — `0x03FA`–`0x03FD`, `0x03FF`, `0x0400` — have no code
   at all in this binary.
3. **Inference is labelled.** Anything marked **INFERRED** is a bridge *this document* built between
   a wiki fact and an opcode fact. Neither source states it outright.
4. **Enum member names are PC-derived.** Message *class* names (`RequestCreateSign`) were recovered
   from the PS3 binary's own vtables. Enum *members* (`SignType_SmallWhiteSoapstone`,
   `VisitorType_BellKeepers`) were not — they come from the DS3OS protos in
   `proto/DS2_Frpg2RequestMessage.proto`, several carrying "guessing, needs validation" comments from
   their original author. The wire *values* are real, because the client sends them. The *labels* are
   borrowed.

## Status vocabulary

| Term | Means |
|---|---|
| **Working** | Exercised end to end by two real BLUS41045 clients against this server. Evidence in `docs/STATUS.md`. |
| **Implemented, unconfirmed** | Code exists and unit tests pass, but no console has driven it. Usually because an unverified push alias sits in the path. |
| **Not built** | No handler. The client's request goes unanswered. |

An unanswered request/response opcode is **not** harmless: the client silently retries and **will not
open other online UI while one is outstanding**. That is how several "broken menu" symptoms were
eventually explained.

---

## 1. Blood messages

**What the player does.** There is **no soapstone item for this in DS2** — writing a message is a
menu function available from the moment you leave Things Betwixt. You open the pause menu, pick the
message icon, and assemble a line from a fixed template plus a word list. **Each character may have
ten messages placed at once**, and you can revisit your own to edit or delete and free a slot. Other
players walking the same ground see the sigil, read it, and rate it. When a message of yours is rated
positively you get a **partial health restore, wherever you are**, shown as a small point of light —
and the amount scales with how many ratings that message has accumulated. That, and nothing else, is
why every ledge in Drangleic is fenced with "try jumping".

**Opcodes**

| Opcode | Message | Kind |
|---|---|---|
| `0x03AB` | `RequestCreateBloodMessage` | R/R |
| `0x03AC` | `RequestRemoveBloodMessage` | R/R |
| `0x03AD` | `RequestReentryBloodMessage` | R/R |
| `0x03AE` | `RequestGetBloodMessageList` | R/R |
| `0x03AF` | `RequestEvaluateBloodMessage` | R/R |
| `0x03B0` | `RequestGetBloodMessageEvaluation` | R/R |
| `0x03AA` | `PushRequestEvaluateBloodMessage` (name inferred in the map) | P |

**Status in dso: Working.** `internal/server/game/bloodmessage.go`. Placed, listed, rated
cross-client and persisted in SQLite. The praise notification was the **first push ever delivered on
PS3** and is what proved the push transport model.

**Notes**

- The message body is a word-ID blob the server never interprets. Across the whole DS2 schema the
  only genuine free text server→client is `AnnounceMessageData`'s header/message and
  `ManagementTextMessage` — everything else is a PSN id or a template id.
- The health restore is the *client's* reaction to the `0x03AA` push. The server only forwards.
- **The ten-message cap is enforced client-side.** Our store has no per-character limit. A modified
  client could flood the world; the reference has the same hole.
- **Open question — negative ratings.** Wiki sources describe DS2 rating as positive *or* negative,
  while `BloodMessageData` exposes only a `good` counter and our `EvaluateBloodMessage` handler
  unconditionally increments. Either the client never sends a negative evaluation on this platform,
  or we are silently converting downvotes into upvotes. `RequestEvaluateBloodMessage`'s payload has
  never been examined for a sign or type field. Cheap to check in a log.
- `0x03AA`'s message *name* is inferred from the enclosing manager; the *opcode* is certain — a
  directly registered push handler at `0x158ECEC`, not an alias.
- **`RequestGetAreaBloodMessageList` (`0x03FF`) does not exist here.** Area-wide message queries are
  a SOTFS addition. On PS3 there is one listing call and it is cell-scoped.
- Self-rating is declined rather than punished; the reference disconnects for it.
- Message ids start at 100000 and never repeat, because the client caches rating state by id. A
  reused id once made a brand-new message show as already rated.

---

## 2. Bloodstains and death replays

**What the player does.** Dark bloodstains mark where other players died. Touching one plays a few
seconds of their final moments — **only their phantom is shown, not the enemy that killed them**, so
reading a stain is inference, not evidence. No item, no covenant, no toggle: it happens while online.
Placement is deliberately approximate — for a fall death the stain sits roughly five seconds before
the end, and for a death during an invasion it is placed where the invasion *started*.

**Opcodes**

| Opcode | Message | Kind |
|---|---|---|
| `0x0391` | `RequestCreateBloodstain` | **M** (no reply) |
| `0x0392` | `RequestGetBloodstainList` | R/R |
| `0x0393` | `RequestGetDeadingGhost` | R/R |

**Status in dso: Working.** `internal/server/game/bloodstain.go`. Memory-only, matching the
reference's default.

**Notes**

- `0x0391` is the one opcode here that registers no response callback — the single case where the PS3
  binary and the PC map agree on "no reply".
- The stain carries **two** opaque blobs: `data` (the stain itself) and `ghost_data` (the replay).
  `0x0393` serves the second on touch, and an unknown id must still get a reply with empty data, or
  the client retries forever over a stain that has simply expired from memory.
- **SotFS pools multiple deaths at one spot into a single stain that plays every replay.** The
  original build shows one death per stain, which is what our one-blob-per-stain model assumes. If
  anyone ever targets SotFS, this is a schema-shaped difference, not a cosmetic one.
- **`RequestGetAreaBloodstainList` (`0x0400`) does not exist on PS3.** SOTFS addition.

---

## 3. Ghosts — the drifting apparitions of other players

**What the player does.** Nothing. Translucent phantoms of players who are online *right now* drift
through your world, replaying their movement. They render in full detail — you can see the other
player's actual armour and weapons, and near bonfires they resolve clearly enough to read their
equipment and whether they are human. Non-interactive, on by default. In practice they are the
game's live population indicator, which makes them a useful smoke test on a private server.

**Opcodes**

| Opcode | Message | Kind |
|---|---|---|
| `0x03B1` | `RequestCreateGhostData` | R/R |
| `0x03B2` | `RequestGetGhostDataList` | R/R |

**Status in dso: Working.** `internal/server/game/ghost.go`. Memory-only.

**Notes**

- Both are request/response and the client **retries until answered** — confirmed live by a client
  resending an identical 1367-byte `RequestCreateGhostData` twice. An unanswered
  `RequestCreateGhostData` was what made the Message menu appear broken. This is the canonical
  example of why "harmless" unimplemented opcodes are not harmless.
- Response field numbering is a trap: `ghosts` is field **3** and field 2 is skipped. Hand-rolling it
  as field 2 produces a message the client silently ignores.
- **The Name-engraved Ring biases which ghosts you see** toward players who chose the same god. That
  is a matchmaking rule applied to `RequestGetGhostDataList`, and we ignore it — see §13.

---

## 4. Summon signs — ordinary co-op

**What the player does.** You place a sign and wait to be pulled into a stranger's world.

- **White Sign Soapstone** (Mild-Mannered Pate, Forest of Fallen Giants). The full co-op item.
  **66m 40s**, minus time for every enemy the host kills, paused inside boss rooms. Pays a **Token of
  Fidelity** per boss cleared. **Cannot be used to enter an area whose boss is already dead** — this
  restriction is the entire reason the Small version exists.
- **Small White Sign Soapstone** (locked chest above Cardinal Tower). A DS2 novelty. You arrive as a
  **shade**, not a white phantom; the session is **8m 20s** with the screen darkening as a warning at
  two minutes and one minute; **it works in areas whose boss is already dead**, and in areas with no
  boss; and **you are sent home the instant the host walks through a fog gate**. Pays a Smooth &
  Silky Stone regardless of outcome, and **does not** advance Heirs of the Sun or earn Tokens of
  Fidelity.
- **Heirs of the Sun** membership turns your white sign and your phantom **gold** and swaps the boss
  reward to a Sunlight Medal.
- **Red Sign Soapstone** (bought from Titchy Gren for 5,000 souls after joining the Brotherhood of
  Blood; reusable, and it keeps working after you leave). Places a **red** sign — the host summons
  you in as a hostile dark spirit for a consensual duel, which **incurs no sin**.
- **Dragon Eye** (Dragon Remnants: give Magerold the Petrified Egg). Places a dragon sign for a duel
  worth a Dragon Scale.
- Red and white signs are **mutually exclusive** — placing one removes the other.
- The host must be **human**; the person placing the sign need not be, and is restored to human on
  being summoned. Cap: **2 friendly phantoms** in the original release, **3 in SotFS**.

**Opcodes**

| Opcode | Message | Kind |
|---|---|---|
| `0x0394` | `RequestCreateSign` | R/R |
| `0x0395` | `RequestUpdateSign` | R/R (keepalive) |
| `0x0396` | `RequestRemoveSign` | R/R |
| `0x0397` | `RequestGetSignList` | R/R |
| `0x0398` | `RequestSummonSign` | R/R |
| `0x039A` | `RequestRejectSign` | R/R |
| `0x039B` | `PushRequestSummonSign` | P |
| `0x039C` | `PushRequestRejectSign` | P |
| `0x039D` | `PushRequestRemoveSign` | P |

The item used arrives as `SignType` on `RequestCreateSign` (schema-derived names, wire values real):

| Value | Schema name | Item — **INFERRED** from the name |
|---|---|---|
| 0 | `WhiteSoapstoneSunlight` | White Sign Soapstone, Heirs of the Sun member (gold sign) |
| 1 | `WhiteSoapstone` | White Sign Soapstone |
| 2 | `SmallWhiteSoapstoneSunlight` | Small White Sign Soapstone, Heirs of the Sun member |
| 3 | `SmallWhiteSoapstone` | Small White Sign Soapstone |
| 4 | `RedSoapstone` | Red Sign Soapstone (Brotherhood of Blood duel) |
| 6 | `Dragon` | Dragon Eye (Dragon Remnants duel) |

That six-value split maps exactly onto the five items above plus the gold variants, which is strong
independent support for the schema's naming — the enum has precisely the shape DS2's item set
requires, including the otherwise-odd "Sunlight" duplication.

**Status in dso: Working.** `internal/server/game/sign.go`. Host↔summoner brokering confirmed between
two consoles.

**Notes**

- The server is a **broker, not a relay**: it hands the summoner's blob to the host with `0x039B` and
  steps out. Everything after that is peer-to-peer.
- `0x0399` is unused within the block — the sequence is `0x0394`–`0x0398`, then `0x039A`.
- **Sign types are not filtered by the server.** A red duelling sign and a white co-op sign go into
  the same store and are listed to whoever asks for that cell; the client filters. Fine for two
  players, wrong for a busy server — and note that the *game's* Soul Memory range differs per sign
  type (§13), so type-blind listing is doubly wrong once matchmaking exists.
- **The `SmallWhiteSoapstone` semantics are not modelled and probably should be.** Its whole point is
  that it works where the boss is dead and the full soapstone does not. Neither our store nor the
  client's request carries boss state, so this is presumably enforced entirely client-side — but it
  is worth confirming, because if the server is expected to filter, small-soapstone co-op will look
  subtly broken rather than obviously broken.
- **"Dragon Talon" is not the dragon-sign item.** It exists in DS2, but as a *key item* that opens
  the door to Shulva for the Crown of the Sunken King DLC — nothing multiplayer about it. The dragon
  sign comes from the **Dragon Eye**. (One of our research passes claimed DS2 has no Dragon Eye; that
  is wrong — the Dragon Remnants covenant hands one out, and it has its own documented Soul Memory
  range and 25-minute timer.)
- `awareOf` tracking exists so a removed or disconnected host's sign vanishes from other worlds
  immediately rather than at the next poll.
- **No `RequestGetRightMatchingArea` (`0x03FA`) on PS3.** Do not build the SOTFS area-matching path.

---

## 5. Invasions

**What the player does.** You use a **Cracked Red Eye Orb** — consumable, and DS2 has **no unlimited
Red Eye Orb at all**, in any edition, which was one of the loudest complaints about its PvP. You need
to be human, in a PvP-enabled area. The invasion lasts fifteen minutes; killing the host pays a Token
of Spite and about 10% of their level-up cost in souls, and **accrues sin**. In NG+ Titchy Gren sells
Cracked Red Eye Orbs without limit, which is as close to unlimited invading as the game gets.

The **Cracked Blue Eye Orb** is the mirror image: Blue Sentinels only, and it hunts a player carrying
**sin** rather than a random target. It pays no souls.

On the receiving end: **you can be invaded while hollow in DS2**, unlike DS1 — hollow hosts are just
lowest priority in the queue. You cannot be invaded at a bonfire. Burning a Human Effigy blocks
invasions in that area for about thirty minutes, and killing the area boss does it permanently and
for free — **except in the belfries, where neither works** (§6).

**Opcodes**

| Opcode | Message | Kind |
|---|---|---|
| `0x03D2` | `RequestGetBreakInTargetList` | R/R |
| `0x03D3` | `RequestBreakInTarget` | R/R |
| `0x03D4` | `RequestRejectBreakInTarget` | R/R |
| `0x03B9` | `PushRequestBreakInTarget` — **confirmed live** | P |
| `0x03BA` | assumed `PushRequestRejectBreakInTarget` — **unverified** | P |
| `0x0320` | `RequestSendMessageToPlayers` — carries the host's "allow" back to the invader | M / relay |
| `0x03ED` | `RequestNotifyKillPlayer` | M |

`BreakInType` on the request (schema-derived names): `0 = RedEyeOrb`, `2 = BlueEyeOrb`.
**INFERRED:** `0` is the Cracked Red Eye Orb and `2` the Cracked Blue Eye Orb / sinner hunt. The gap
at `1` is unexplained — possibly the Dark Chasm of Old's Abyss Phantom invasion, which behaves
differently (the invader is hostile to the NPC phantoms too), but that is a guess with nothing behind
it.

**Status in dso: Working.** `internal/server/game/breakin.go` plus `relay.go`. A real invasion
completed between two consoles.

**Notes**

- **This is the single biggest PS3-vs-PC divergence.** The PC map puts the BreakIn pushes at
  `0x03FB`–`0x03FD`. Those values have **no code at all** in BLUS41045. The PS3 client registers a
  **sixteen-opcode block, `0x03B9`–`0x03C8`** — four message types with four aliases each — and a
  server sending `0x03FB` is simply never dispatched. `opcodes_test.go` fails the build if any of the
  six forbidden opcodes is ever routed.
- Registration order from the disassembly, in groups of four:
  `(0x3BD 0x3BE 0x3C0 0x3BF)`, `(0x3C1 0x3C2 0x3C4 0x3C3)`, `(0x3B9 0x3BA 0x3BC 0x3BB)`,
  `(0x3C5 0x3C6 0x3C8 0x3C7)`. Group 3 is the BreakIn-target group, and `0x03B9` leading it was
  confirmed by a live invasion. **The other three groups — reject/allow/remove — are still
  unassigned.**
- **`PushRequestAllowBreakInTarget` is never sent by the server.** The host's client serialises it
  itself and tunnels it through `0x0320`. Invasions timed out for a while even though the break-in
  push was landing, purely because that relay went unhandled. Any invasion-adjacent feature should
  assume the same shape.
- `pushBreakInRejected` assumes `breakInPushID + 1`. **A declined invasion may silently fail to
  notify the invader.** This is a minutes-long live test and the cheapest open item in the project.
- `0x03ED` is the *killer's* report of a death the victim already reported with `0x03F1`. Counting
  both doubles every PvP death — our capture shows the pair arriving 55 ms apart from two consoles.
  We deliberately do not count it.
- **The Crushed Eye Orb is not an invasion in the protocol sense.** It is a scripted, single-use NPC
  invasion against Licia of Lindeldt in the Majula rotunda. No second player is involved, so it
  should produce no BreakIn traffic at all. Worth knowing so nobody hunts for an opcode for it.
- **Sin has no representation anywhere in the protocol.** See the gap list.

---

## 6. Covenant auto-summons — pulled into a world without a sign

This is what the protocol calls **Visitor**: three distinct covenant mechanics sharing one message
family. None involves placing a sign. You equip a ring, go about your business, and the game moves
somebody.

**What the player does.**

- **Bell Keepers** — join the marionette in **Belfry Luna** (Lost Bastille, Servants' Quarters) or
  **Belfry Sol** (Iron Keep, Ironhearth Hall); both entrances need a Pharros' Lockstone. Wearing the
  **Bell Keeper's Seal**, you are auto-summoned as a **grey spirit** into the world of anyone
  trespassing in either belfry. You can be almost anywhere when it fires; standing in or near a
  belfry only biases which one. **No healing items**, and you **cannot unequip the ring mid-invasion**.
  Ten-minute timer. Winning pays a Titanite Chunk (~83%) or better; ranks at 10 / 30 / 100
  trespassers. From the victim's side, entering a belfry online *will* get you invaded — human status
  is irrelevant and burning an effigy does not help. **Two grey phantoms at once in the original
  release, three in SotFS.**
- **Rat King** — join via the Rat King after the Royal Rat Vanguard (Grave of Saints) or Royal Rat
  Authority (Doors of Pharros). **This one is inverted.** Wearing the **Crest of the Rat** inside
  either rat territory, a non-member who walks into that area *in their own world* is dragged into
  **yours** as a hostile grey phantom. You never leave home; every enemy in the area is friendly to
  you and hostile to them; you can pre-arm Pharros' Contraptions with Lockstones. Ten minutes. The
  trespasser's objective is to reach the boss fog, not to kill you, and they lose no souls if they
  die. Kill reward: a Rat Tail and a Lockstone.
- **Blue Sentinels + Way of Blue** — Way of Blue is joined from Saulden in Majula immediately, with
  no prerequisites. Blue Sentinels needs a Token of Fidelity and Targray in the Cathedral of Blue.
  Wearing the **Guardian's Seal** (and being **human**, in a PvP-enabled area, without having burned
  an effigy), you are auto-summoned as a blue **Arbiter Spirit** into the world of a Way of Blue
  member who is being invaded. A pulsing covenant icon by your HP bar means you are eligible; on
  arrival a **red eye floats above the invader's head**. You cannot attack the host's regular
  enemies. **You have no timer of your own** — you last as long as the invader's remaining clock.
  Crucially, on the *victim's* side, being rescued needs **only Way of Blue membership**: not
  humanity, not the Blue Seal equipped. The one blocker is the host already being at the phantom cap.

**Opcodes**

| Opcode | Message | Kind |
|---|---|---|
| `0x03D5` | `RequestGetVisitorList` | R/R |
| `0x03D6` | `RequestVisit` | R/R |
| `0x03D7` | `RequestRejectVisit` | R/R |
| `0x03C9`–`0x03D1` | Visitor push block — **9 aliases for 3 message types** | P ×9 |

`VisitorType` selects which covenant (schema-derived names; the original author marked
`BlueSentinels` "guessing, needs validation"):

| Value | Schema name | Covenant |
|---|---|---|
| -1 | `None` | — |
| 0 | `BlueSentinels` | Guardian's Seal defence of a Way of Blue host — **INFERRED, flagged unverified in the schema itself** |
| 1 | `BellKeepers` | Bell Keeper's Seal |
| 2 | `Rat` | Crest of the Rat |
| 3 | `3` | unknown |

**Status in dso: Implemented, unconfirmed.** `internal/server/game/visitor.go`. Structurally the
invasion flow, not the sign flow — nothing is stored, the server brokers between two live sessions
and steps out.

**Notes**

- **The push ids are the open risk.** We send `0x03CF` (visit) and `0x03D0` (reject). That has
  positive evidence behind it — the PC protos assign exactly those three values to exactly those
  three types, and unlike the BreakIn case the decompilation confirms all three *exist* on PS3, as
  the last group of nine. It is still unverified. **If a visit silently does nothing in-game, try
  `0x03C9` / `0x03CC` / `0x03CF`** — the first alias of each contiguous group, the shape that proved
  right for BreakIn.
- **`PushRequestRemoveVisitor` (`0x03D1`) is deliberately never sent.** Telling a host their visitor
  left requires knowing which host a departing player was in, and no visit session is tracked. The
  phantom clears on the clients' own timeout instead.
- **The Rat King direction inversion is not modelled.** Our handler is symmetric: whoever sends
  `RequestVisit` is treated as asking to enter the target's world. That matches Bell Keepers and Blue
  Sentinels. For Rat King the *trespasser* ends up in the *member's* world, so either the client
  sends from the opposite side or the semantics differ. **Nothing in the decompilation settles this
  and we have not tested it.** A live Rat King summon would, in minutes.
- **The Blue Sentinel path has an asymmetry our model cannot express.** The Sentinel is the one who
  polls, but the *trigger* is an invasion happening to a third party. See gap 1 — this is the single
  most interesting hole in the whole map.
- **The PC map claims `0x03C9` is `PushRequestNotifyRingBell`, "not registered".** On PS3 `0x03C9`
  *is* registered — as the first entry of the Visitor block. There is no separate ring-bell push id
  on this platform (§15).
- Matchmaking is not applied: every other online player is offered as a visit target. The real ranges
  differ sharply per covenant — Guardian's Seal reaches 7 tiers down and 6 up, Bell Keeper's Seal
  only 1 down and 3 up. See §13.

---

## 7. Mirror Knight — the Looking Glass Knight's summoned phantom

**⚠️ Correction to our own notes.** `internal/server/game/mirrorknight.go` (file header) and
`tasks/remaining-features.md` §6 both describe this as "the Belfry Sol arena". **That is wrong, and
so is the framing in the task that produced this document.** *Mirror Knight* is the pre-release name
of the **Looking Glass Knight**, the boss in **King's Passage, Drangleic Castle**. Belfry Sol is a
Bell Keeper belfry with no boss in it at all. The two are unrelated.

**What the player does.** Partway through the Looking Glass Knight fight, the boss plants its mirror
shield into the ground and summons an ally — usually an NPC, but it can pull in a **real player, as a
hostile**. To be that player you leave a **Red Sign Soapstone** sign anywhere in Drangleic Castle,
by convention around the last two bonfires in King's Passage, re-placing it every five or ten seconds
to cycle through worlds faster and to avoid being summoned by an ordinary host first. You get the
prompt *"The Looking Glass Knight summons you! Become a mirror squire, and vanquish the world
master."* As a Mirror Squire you can even **heal the boss** with Warmth or Great Heal. The boss holds
up to two phantoms and replaces them as they die. No covenant is required on either side, and it
still works after you have beaten the boss yourself. Real-player summons are **rare** in practice.

**Opcodes**

| Opcode | Message | Kind |
|---|---|---|
| `0x039E` | `RequestCreateMirrorKnightSign` | R/R |
| `0x039F` | `RequestUpdateMirrorKnightSign` | R/R (keepalive) |
| `0x03A0` | `RequestRemoveMirrorKnightSign` | R/R |
| `0x03A1` | `RequestGetMirrorKnightSignList` | R/R |
| `0x03A2` | `RequestSummonMirrorKnightSign` | R/R |
| `0x03A4` | `RequestRejectMirrorKnightSign` | R/R |
| `0x03A5` | `PushRequestSummonMirrorKnightSign` | P |
| `0x03A6` | `PushRequestRejectMirrorKnightSign` | P |
| `0x03A7` | `PushRequestRemoveMirrorKnightSign` | P |
| `0x03D8` | `RequestNotifyMirrorKnight` | M |

**Status in dso: Implemented, unconfirmed.** `internal/server/game/mirrorknight.go`. Needs two clients
at the Looking Glass Knight — and given how rare the summon is even on retail servers, testing it
will need the sign spammed deliberately.

**Notes**

- Unlike Visitor and QuickMatch there is **no alias guesswork**: all six requests and all three pushes
  are individually confirmed, with `cmpwi` on `0x3A6`/`0x3A7`/`0x3A5` visible in the MirrorKnight
  manager's own dispatcher.
- **The structural oddity — no placement — now makes sense.** `RequestCreateMirrorKnightSign` carries
  no `online_area_id` and no `cell_id`. That fits the mechanic exactly: the sign is placed *anywhere
  in the castle* and the boss, not a host walking over it, picks who to pull in. It is one global
  pool, so listing cannot filter by position. `SignData` still declares those fields `required`, so
  they go out as zero — an absent required field is rejected outright by the client's proto2 parser.
- Mirror Knight signs use a **disjoint id range** (`firstMirrorKnightSignID = 500000`) because both
  stores seed independently and the client caches by sign id without distinguishing the systems.
- `0x03A3` is unused within the block.
- `RequestGetMirrorKnightSignListResponse` has no `SignInfo` field, so unlike ordinary signs there is
  no "you already have this one" optimisation; every match is sent in full each poll.
- `0x03D8 RequestNotifyMirrorKnight` is fire-and-forget. **INFERRED:** it is the host's client
  announcing that the shield-plant summon has fired, or that the fight has begun. We accept and log
  it; nothing is known about its payload beyond the name. **Because the boss decides when to summon,
  this is the natural candidate for "the boss is now looking for a phantom"** — and if so, the server
  is expected to *react* to it rather than merely record it. That would make it the one place where
  Mirror Knight is not just a sign system with the placement removed. Worth capturing.
- Community lore claims you can predict from the animation whether an NPC or a player is coming
  (shield-plant vs sword-plant). Unsourced; ignore it.

---

## 8. The duelling arenas

**⚠️ Terminology correction.** *"Undead Match"* is **Dark Souls III** terminology, and DS3's version
is the one with 2v2, 3v3 and brawl modes. **DS2's arenas are 1v1 duels only.** There is no team mode
and no battle royale in this game.

**What the player does.** Two covenants run arenas, and they are separate venues:

- **Brotherhood of Blood** — Titchy Gren, in **Undead Purgatory** past the Executioner's Chariot,
  off Huntsman's Copse. Needs a **Token of Spite** present in inventory (never consumed).
- **Blue Sentinels** — Targray, at the **Cathedral of Blue** bonfire in Heide's Tower of Flame. Needs
  a **Token of Fidelity** present *each time you queue*, which is why this arena is close to dead
  compared to the Brotherhood's.

Either way you pray at one of **three statues**, each a vote for a different map — a bridge over a
lethal drop, a two-level labyrinth, a circular scaffolded stage on the Brotherhood side; a labyrinth,
a cross-shaped bridge over a pit, and an open stepped arena on the Blue side. If nobody is queued at
your map you get paired with someone from another, and **the higher covenant rank decides which map
is used**. Matches are capped at ten minutes. Healing items are barred; healing spells are not.

Brotherhood rank is a **points** system: +1 per arena win *or* per blue phantom killed while
invading, **−1 per loss**, with ranks at 50 / 150 / 500. **The game shows you no win/loss record and
there is no arena leaderboard** — players track it themselves.

**Opcodes**

| Opcode | Message | Kind |
|---|---|---|
| `0x03D9` | `RequestRegisterQuickMatch` | R/R |
| `0x03DA` | `RequestUnregisterQuickMatch` | R/R |
| `0x03DB` | `RequestUpdateQuickMatch` | R/R |
| `0x03DC` | `RequestSearchQuickMatch` | R/R |
| `0x03DD` | `RequestJoinQuickMatch` | R/R |
| `0x03DE` | `RequestRejectQuickMatch` | R/R |
| `0x03E0`–`0x03E7` | QuickMatch push block — **8 aliases for 4 message types** | P ×8 |

`QuickMatchGameMode`: `0 = Blue`, `1 = Brotherhood`.
**INFERRED, and it fits well:** these are the two *venues* — the Cathedral of Blue statues and Titchy
Gren's statues — not two match formats. That reading is what makes sense of a mode field in a game
with only one match format, and the schema's own sample `online_area_id` values (`10230000` and
`10310000`, with cells `102350` and `103140`) are two distinct places, exactly as expected.

**Status in dso: Not built.** The largest remaining mode, and the only one needing a concept we do
not have: a **match session with a lifecycle**, rather than a broker that introduces two players and
steps out. Reference shape: ds3os `DS2_QuickMatchManager`, 12 handlers / 382 lines.

**Notes**

- **Soul Memory is ignored entirely in the arenas.** A tier-1 character can be matched against a
  tier-45 one; this is the only place in DS2 with no range filter at all. That is a *simplification*
  for us — arena matchmaking needs the venue and nothing else — but it must be a deliberate
  exemption, not an accident of not having filters yet.
- The push block is **interleaved**, not grouped: odds `0x3E1, 0x3E3, 0x3E5, 0x3E7` are registered
  first, then evens `0x3E0, 0x3E2, 0x3E4, 0x3E6`. That interleaving is the documented precedent for
  suspecting the Visitor aliases might be interleaved too.
- `PushRequestAllowQuickMatch` is **client-relayed through `0x0320`**, exactly like
  `PushRequestAllowBreakInTarget` — both `0x0320` send sites in the binary sit inside functions that
  construct precisely those two classes. Whoever builds this must handle the relay path, not just the
  pushes.
- `RequestRegisterQuickMatchResponse` and `RequestRejectQuickMatchResponse` are annotated "never
  received" in the schema, so the reply may be empty or absent. `0x03DF` is unused.
- **Map selection has no home in the protocol.** Three statues per venue, and the higher-ranked
  player's choice wins — but `RequestRegisterQuickMatch` carries only area, cell, matching parameter
  and mode. Either the cell id encodes the statue, or map selection is negotiated peer-to-peer after
  the match is brokered. Unresolved; see the gap list.
- **Nothing in the protocol carries arena rank points**, which is consistent with the game not
  showing you a record.

---

## 9. The leaderboard — Champion's Tablet and Awestones

**What the player does.** Joining the **Company of Champions** at the **Victor's Stone** in Majula
hands you the **Champion's Tablet**. Using it opens the game's one real online leaderboard: the **top
100 members**, each with their **name and the number of Awestones they have offered**, scrollable
across several pages. Awestones drop from defeated red-phantom invaders (players and NPCs) and from a
few PvE enemies; you offer them at the Victor's Stone for rank at 10 / 25 / 50. **Awestones offered
while offline count toward your rank but never appear on the tablet** — the ranking is explicitly a
server-side aggregate.

**Opcodes**

| Opcode | Message | Kind |
|---|---|---|
| `0x03F3` | `RequestRegisterPowerStoneData` | R/R |
| `0x03F4` | `RequestGetPowerStoneRanking` | R/R |
| `0x03F5` | `RequestGetPowerStoneMyRanking` | R/R |
| `0x03F8` | `RequestGetPowerStoneRankingRecordCount` | R/R |

**"Power Stone" is the Victor's Stone, and this board is the Champion's Tablet — INFERRED, but about
as well-supported as an inference gets.** There is **no item called a Power Stone in DS2**; both
research passes searched for one and found nothing. Meanwhile: DS2 has exactly one player-visible
global leaderboard and it is the Champion's Tablet; the covenant's currency is a *stone* offered to a
*stone*; `RequestRegisterPowerStoneData` carries an **`increment`, not a total**, which is precisely
"I just offered N Awestones"; and `RequestGetPowerStoneRanking` takes `offset` + `count` with a
separate `RecordCount` call, which is precisely the paging UI the tablet presents. Neither the
decompilation nor any wiki says "power stone" — this is our bridge.

**Status in dso: Implemented, unconfirmed in-game.** `internal/server/game/ranking.go`, persisted in
`power_stone_rankings` with unit tests. No console has opened a Champion's Tablet against it.

**Notes**

- Ranks are **derived on read**, so they cannot go stale against the scores they describe.
  `serial_rank` is a unique 1-based position; `rank` is a competition rank where ties share a value.
  The client's `offset` is 1-based.
- The submission increment is **bounded** (`maxScoreIncrement`). The board is persistent, so an
  unvalidated increment would let one modified client pin the top of it permanently.
- **Cap the board at 100.** The real tablet shows a top 100. `maxRankingPage` currently bounds a
  single page at 100 but nothing bounds the board's depth; matching the game is both cheaper and more
  faithful.
- **Keyed by `character_id`, which the protocol dictates** — `RequestGetPowerStoneMyRanking` looks up
  by character alone. Character ids are still per-run and in memory, so a reused id would inherit a
  previous character's score. Persisting characters is the fix.
- **The name problem is now concrete.** The tablet shows player *names*, and
  `PowerStoneRankingData` carries `player_id`, `character_id`, ranks, score, and an opaque `data`
  blob — **no name field**. Either the name is inside `data` (in which case our server must echo a
  blob the client itself supplied on `RequestRegisterPowerStoneData`, which we do), or the client
  resolves it through `RequestGetPlayerCharacter` (`0x03A9`). **We do echo the blob**, so the first
  case should already work. If the board renders nameless, the second case is true and the two
  features are coupled.

---

## 10. The world death counter

**What the player does.** Reads the plaque on the **stone monument in Majula** — the tall memorial by
the sea where Crestfallen Saulden sits. **Online it reports deaths worldwide**, across the whole
player base, a number that ran into the tens of millions on retail. **Offline the same plaque shows
your own character's death count instead.** It is not on a loading screen and not on the main menu;
this one interactable is the entire feature.

**Opcodes**

| Opcode | Message | Kind |
|---|---|---|
| `0x03F0` | `RequestGetTotalDeathCount` | R/R |
| `0x03F1` | `RequestNotifyDeath` | M |
| `0x03F2` | `RequestNotifyOfflineDeathCount` | M |
| `0x03ED` | `RequestNotifyKillPlayer` | M (see §5) |

**Status in dso: Working.** `internal/server/game/deathcount.go`. Counts live and survives restarts.

**Notes**

- `RequestGetTotalDeathCount` has a **zero-byte payload** — no area, character or scope — which is
  why a single world-wide total was the only available reading. Later confirmed by the in-game label
  "deaths worldwide". It is the *only* one of the four that needs a reply, and it *is* retried (twice,
  5.3 s apart); that unanswered retry is what originally made the counter time out.
- `total_death_count` is `required`, so it must be set even at zero.
- `RequestNotifyOfflineDeathCount` exists precisely because the plaque counts your offline deaths
  locally and uploads them on reconnect. The batch is client-supplied and clamped at 10000; the
  counter is persistent, so damage from a modified client would outlive the session.
- **This monument is the same object as "the Majula obelisk".** Its reverse face carries the line
  *"The letters are worn beyond recognition."* — which `docs/STATUS.md` identifies as **string id 100
  in `regulation<Language>.fmg`**. So the obelisk investigation and the death-counter investigation
  were always looking at two faces of one monument. Recorded because it took a long time to
  disentangle them.
- **The obelisk face is writable again** (2026-08-07). Not through an opcode of its own: the FMG is
  replaced wholesale over `0x038B`, keyed as the bare resource name `regulation.fmg`. That is the
  channel FromSoftware used to announce all three Lost Crowns DLCs — those messages, and how to
  send new ones, are in `docs/worn-writing.md`.

---

## 11. The post-login banner

**What the player does.** On entering online mode a line of server text appears in the **upper left
of the screen**.

**Opcodes**

| Opcode | Message | Kind |
|---|---|---|
| `0x0389` | `ManagementTextMessage` | P (special-cased in the dispatcher) |

**Status in dso: Working.** `internal/server/game/managementtext.go`, driven by
`Config.ManagementText`.

**Notes**

- `0x0389` is one of only **three** push ids the client special-cases *by value* before its
  red-black-tree lookup (`0x0389` → `0x1587F60`, `0x038B` → `0x158B150`, `0x038C` → `0x1588218`), and
  handler `0x1587F60` builds an object whose `GetTypeName` returns
  `Frpg2RequestMessage.ManagementTextMessage`. No ambiguity, unlike the alias blocks.
- **It is not the Majula obelisk.** That was assumed for a long time. The obelisk's text is game data
  in the regulation archive (§10). Only a live console settled it.
- All five fields are genuinely required: `IsInitialized` at `0x162A6F8` masks the has-bits with
  `0x1F` and demands all five, recursing into the timestamp. An omitted field means the push is
  dropped in silence.
- It must be queued **behind** the login reply — the client will not proceed while a request/response
  is outstanding.
- **This is an abusable channel and it is worth knowing why.** In DS2 an ordinary player who could
  impersonate the matchmaking server was able to broadcast arbitrary text to other clients, used for
  harassment and — with malformed format strings — for crashes. The DS2 anti-cheat mod Blue Acolyte
  simply blocks incoming server announcement text for this reason, and the same class of bug in DS3
  is CVE-2022-24125, which our relay recipient cap already guards against (§18). Anything we ever add
  that lets one client influence another's banner text needs that history in mind.

---

## 12. Announcements

**What the player does.** DS2's online boot has a "Retrieving information" step. The protocol carries
two lists — **changes** and **notices** — each entry with a header, a body and a timestamp, and the
client displays them without the player asking. FromSoftware used this for operational notices:
pending updates, scheduled downtime, server status. The final use was the shutdown notice on
**31 March 2024**, when the PS3 and 360 servers were retired.

**Opcodes**

| Opcode | Message | Kind |
|---|---|---|
| `0x03EC` | `RequestGetAnnounceMessageList` | R/R — **confirmed live** |

**Status in dso: Working, but empty.** `handleGetAnnounceMessageList` in `boot.go` returns both lists
present and empty.

**Notes**

- Sent immediately after `0x0386` with a 2-byte payload, and **it blocks boot** until answered.
- `changes` and `notices` are both `required`. Omitting either produces a message the client's proto2
  parser rejects — an empty list is not the same as an absent one.
- The reference uses this channel to deliver bans and warnings: return an announcement, then
  disconnect.
- **We have never seen a non-empty response rendered.** Where the two lists appear in the DS2 UI,
  whether both are shown, and what `unknown_1` (observed as 20 / 104 / 24) selects is all untested.
  Cheap and interesting: it is one of only two free-text server→client channels in the schema, and
  the obvious place to put a "welcome to this private server" notice.

---

## 13. Matchmaking — Soul Memory, the Name-engraved Ring, the Agape Ring

**What the player does.** DS2 does not match on Soul Level. *"There is no level range matching in
Dark Souls 2."* It matches on **Soul Memory**: every soul the character has ever obtained, spent or
not, which only ever rises. Weapon upgrade level is not a parameter either. Soul Memory is banded
into **45 tiers**, and each multiplayer item reaches a different distance through them:

| Item / seal | Tier range | Session |
|---|---|---|
| White Sign Soapstone | −3 / +1 | 66m 40s |
| Small White Sign Soapstone | −4 / +2 | 8m 20s |
| Red Sign Soapstone | −5 / +2 | 15m |
| Dragon Eye | −5 / +5 | 25m |
| **Cracked Red Eye Orb** | **−0 / +4 — invaders can only punch up** | 15m |
| Cracked Blue Eye Orb | −3 / +3 | 15m |
| Bell Keeper's Seal | −1 / +3 (Name-engraved does **not** widen this) | 10m |
| Crest of the Rat | −1 / +3 | 10m |
| Guardian's Seal | −7 / +6 | invader's remaining clock |
| **Arena duels** | **no restriction at all** | 10m |

Tier 44 spans 45,000,000 to 359,999,999 souls, which is why the community calls anything past ~45M
"top tier": above it, Soul Memory effectively stops separating anyone.

Two rings modify this:

- The **Name-engraved Ring** (Sweet Shalquoir, Majula, 5,500 souls) is engraved with one of **ten
  gods** — Nehma, Caitha, Galib, Kremmel, Evlana, Hanleth, Nahr Alma, Zinder, Quella, Caffrey — and
  can be re-chosen freely by re-equipping. Between two players who **both wear it and picked the same
  god** it roughly doubles the co-op range (a −3/+1 White Sign becomes about −6/+4) and extends every
  session timer by **50%**. It also makes same-god players appear more often as ghosts. And it cuts
  both ways: **your sign becomes invisible to a ring-wearer who chose a different god**, while
  players not wearing the ring are unaffected. It does not touch invasions, red signs or the Dragon
  Eye.
- The **Agape Ring** (Straid, 5,000 souls, available once Soul Memory ≥ 30,000) absorbs every soul
  you gain so Soul Memory freezes — the enabler for stable low-tier duel builds, and the thing that
  undid Soul Memory's anti-twinking rationale. **It is not SotFS-exclusive: it arrived in Patch 1.10
  on PS3/360/DX9.** Whether our target has it depends on which title update the console is running.

**Opcodes.** There is **no matchmaking opcode.** Matchmaking is a filter applied inside the listing
calls, using a `MatchingParameter` the client attaches to nearly every multiplayer request:

| Field | Meaning |
|---|---|
| `calibration_version` | must match, or the two players are on different regulations |
| `soul_level` | present, but DS2 does not match on it |
| `clear_count` | NG+ cycle — **Patch 1.10 relaxed NG↔NG+ matching**, so this field's meaning is version-dependent |
| `covenant` | observed values 0, 2, 4, 6, 7 |
| `disable_cross_region_play` | region lock toggle |
| `name_engraved_ring` | **INFERRED:** the chosen god, 0 meaning none. Ten gods fits a small integer exactly |
| `soul_memory` | the real matchmaking key |

Carried on `0x0394` (create sign), `0x0397` (list signs), `0x03D2` (break-in targets), `0x03D5`
(visitors), `0x03D9` / `0x03DC` (quick match). Live per-character state — area, covenant, soul memory
— also arrives continuously as the opaque `AllStatus` blob on `0x03B8` `RequestUpdatePlayerStatus`.

**Status in dso: Not built.** `MatchingParameter` is stored and echoed verbatim on signs; **nothing
reads it**. `handleGetBreakInTargetList` and `handleGetVisitorList` offer *every* other online player.
The `0x03B8` status blob is saved and never parsed.

**Notes**

- Right for a two-console test server, wrong for a busy one — and it is also *why* the invasion and
  visit paths work today: no filter can exclude the only other player.
- **The ranges are per-item, and the item is knowable.** `SignType` tells us which soapstone;
  `BreakInType` and `VisitorType` tell us which orb or seal. So a faithful implementation is a table
  lookup on data we already receive, not a research project. The one exception is the arena, which is
  exempt entirely.
- The Name-engraved Ring **must** be honoured symmetrically if implemented — a sign hidden from
  mismatched ring-wearers is a rule about *listing*, not about summoning — and it is a **mutual**
  condition, applying only when both parties wear it.
- **The Agape Ring is invisible to the server.** It suppresses soul *gain*, so the server only ever
  sees a frozen `soul_memory`. Nothing to implement.
- Consuming the status blob is the prerequisite for all of this, and is a listed next step in
  `docs/STATUS.md`.

---

## 14. Being a player at all — identity, characters, and the character slot

**What the player does.** Chooses "play online", picks a save slot, and expects other players to see
their PSN name and their character over a summon sign, in a rating notification, or on the tablet.

**Opcodes**

| Opcode | Message | Kind |
|---|---|---|
| `0x0386` | `RequestWaitForUserLogin` | R/R — **confirmed live** |
| `0x03B6` | `RequestUpdateLoginPlayerCharacter` | R/R — **confirmed live** |
| `0x03A8` | `RequestUpdatePlayerCharacter` | M |
| `0x03B8` | `RequestUpdatePlayerStatus` | M |
| `0x03B3` | `RequestGetLoginPlayerCharacter` | R/R |
| `0x03A9` | `RequestGetPlayerCharacter` | R/R |
| `0x03B5` | `RequestGetPlayerCharacterList` | R/R |

**Status in dso: Working for the write side, implemented for the read side.**
`internal/server/game/playerdata.go`. Player ids are persisted and stable per PSN account.

**Notes**

- **`tasks/remaining-opcodes.md` §A3 is now stale.** It lists `0x03A9` and `0x03B5` as unbuilt, but
  both are dispatched in `boot.go` and handled in `playerdata.go`. **59** request opcodes are
  dispatched, not the 57 the catalogue states.
- `RequestUpdateLoginPlayerCharacter` is the "Initializing online mode…" step. `character_id = 0`
  means "allocate me one"; the store is consulted as well as the client's own list, because a client
  only volunteers slots it knows about.
- `RequestGetPlayerCharacterList`'s **response has no fields at all**, so an empty reply is complete.
  Its request looks more like an update than a query, and the schema even misspells the field
  `charatcer_id`. Logged in full pending a capture.
- **The id-reuse hazard is live here.** Character ids are per-run and in memory. Anything that caches
  one — the leaderboard does, by protocol necessity — will attribute it to the wrong character after
  a restart. Same class of bug as the blood message that showed up already rated.
- The PC map puts `RequestGetPlayerCharacterList` at `0x03A1`, which on PS3 is
  `RequestGetMirrorKnightSignList`. Do not follow it.

---

## 15. Ringing the bell

The task that produced this document flagged `RequestNotifyRingBell` as genuinely unknown, and asked
whether the wikis clarify what "ringing a bell" means as an online action in DS2. **They do, and the
answer is the opposite of what we hoped.**

**What the player does.** There are exactly two bells in DS2, in **Belfry Luna** (from the Servants'
Quarters bonfire in the Lost Bastille) and **Belfry Sol** (from Ironhearth Hall in Iron Keep). Each
entrance needs a Pharros' Lockstone and an illusory wall. Neither bell is struck: you **pull a lever**
in a side tower. Ringing is a **gate opener and nothing else** — in Belfry Luna it raises the
portcullis blocking the fog to the Belfry Gargoyles; in Belfry Sol it opens the fog gate onward.

**Ringing does not trigger, end, or affect invasions.** Bell Keeper invasions are triggered by the
host's *presence* in the belfry — you are flagged a trespasser by being there, and you will be
invaded on the way up, long before the bell. There is **no bell that summons the Bell Keepers**, no
third bell, and no bell in the Majula mansion. DS2 deliberately dropped DS1's Bells of Awakening
model, where ringing set a persistent world-state flag.

**Opcodes**

| Opcode | Message | Kind |
|---|---|---|
| `0x03EE` | `RequestNotifyRingBell` | M (no reply) |

**Status in dso: Accepted and logged in full, never interpreted.** `handleNotifyRingBell` in
`telemetry.go` deliberately does not parse the payload — our schema entry is an empty `TODO` — and
hexdumps the raw bytes instead.

**Notes — this closes an open question, in the negative**

- **Both candidate hypotheses in `tasks/remaining-features.md` are now weak.**
  - *Bell Keeper mechanics:* the bell has no online consequence, so there is nothing for a server to
    do with the fact. It cannot end an invasion, because it does not.
  - *The Majula Mansion event chest:* the mansion has no bell, and the chest's contents were set by
    a completely different mechanism (§17). The "suggestively named" connection was a coincidence.
- **INFERRED, and now the best-supported reading: `0x03EE` is pure telemetry.** It is fire-and-forget,
  registers no response callback, and sits in the same family as `RequestNotifyKillEnemy`,
  `RequestNotifyBuyItem` and `RequestNotifyDeath` — all of which report a thing the player did so
  FromSoftware could count it. "How many players have rung each belfry bell" is exactly the kind of
  progression funnel a publisher instruments. Our handler already treats it that way; the only change
  warranted is to **stop expecting it to be the chest trigger**.
- One loose thread worth recording: Fextralife states Bell Keeper **rank 3 additionally requires
  having rung the bells in both belfries**, while wiki.gg lists only the 100 kills. **The sources
  conflict.** Even on Fextralife's reading it is a flag on your own save, not a server event — but if
  a capture ever shows the server *replying* to `0x03EE`, this is the mechanic to look at.
- Corroborating detail on why the message name misleads: the DS3OS schema comment on
  `RequestNotifyRingBell` reads *"Bells, this is for archdragon peak I believe"* — a **DS3** guess
  (DS3's Archdragon Peak bell summons a phantom). DS2 has no Archdragon Peak. The message name is
  shared across both games' schemas, so that comment says nothing about DS2.
- The PC map lists a `PushRequestNotifyRingBell` at `0x03C9` and calls it "not registered". On PS3
  `0x03C9` **is** registered — as the first alias of the Visitor push block. **There is no ring-bell
  push opcode on PS3**, so even if the server wanted to react to a bell, it has no channel back.
- **Still worth capturing the payload** during a Belfry Luna run, because it is free: if it carries an
  area id, that confirms the telemetry reading outright and retires the question.

---

## 16. Staying connected

**What the player does.** Nothing visible — until it fails, and they get dropped mid-invasion.

**Opcodes**

| Opcode | Message | Kind |
|---|---|---|
| `0x038D` | `ServerPing` | R/R |
| `0x038E` | `RequestMeasureUploadBandwidth` | R/R |
| `0x038F` | `RequestMeasureDownloadBandwidth` | R/R |
| `0x03B7` | `RequestBenchmarkThroughput` | R/R |
| `0x03EA` / `0x03EB` | `RequestNotifyJoinSession` / `LeaveSession` | M |
| `0x03E8` / `0x03E9` | `RequestNotifyJoinGuestPlayer` / `LeaveGuestPlayer` | M |
| `0x03F9` | `RequestNotifyDisconnectSession` | M |
| `0x03EF` | session-disconnect **push** | P — **never sent** |

**Status in dso: Working.** `connection.go` and `telemetry.go`.

**Notes**

- **The PC map calls all four of the first group DS3-only.** They are present in DS2 on PS3, at the
  same numbers, as full request/response pairs. ds3os implements none of them for DS2, so there was
  no reference to copy — but every one of the eight messages involved is defined with **no fields at
  all**, so an empty reply is the complete answer rather than a stub.
- An unanswered `ServerPing` was a plausible cause of the periodic disconnects nobody had
  investigated. Now that it is answered, the hypothesis is testable by leaving a client connected.
- `0x03EF` would let the server evict a client cleanly instead of waiting out the 60 s idle timeout.
  We never send it.
- `RequestNotifyDisconnectSession` is not acted on: it carries a session identifier we do not track,
  and dropping the wrong session would be worse than reaping it late.
- The guest join/leave notifies are the only record we get of a co-op or invasion session actually
  forming, since the peer-to-peer half is invisible to us. They are the raw material for the visit-
  session concept QuickMatch will need.

---

## 17. World statistics, and the content channels we do not use

**What the player does.** Kills things and buys things; the game reports both. Separately — and this
is the part worth reading — DS2 was a **live service with a real content channel**, and the players
of the day knew it as *the chest in the basement of the Majula Mansion*.

**The event chest, as it actually worked.** Its contents were **set by the server**. Offline players
found it empty. While online an item appeared in it automatically, and **the chest re-closed itself**
to hold each new item, including for players who had already opened it — one open per account per
event. Bonfire Ascetics did not reset it; reaching NG+ did. FromSoftware ran it **weekly through
mid-2014**: Petrified Something ×3 on 5 June, Twinkling Titanite ×2 on the 11th, Cracked Red Eye Orb
×5 on the 18th, Bonfire Ascetic ×4 on the 25th, then Poison/Bleed Stones, Elizabeth Mushrooms, Smooth
& Silky Stones, and a re-skinned **Murakumo** on 22 July to coincide with the first DLC. The DLC2 and
DLC3 launches each carried a unique re-skinned weapon — a Blacksteel Katana (26 Aug – 5 Sep 2014) and
a Longsword (30 Sep – 13 Oct 2014) — and **Patch 1.10 shipped a four-weapon event that ran 5–12
February 2015**. Those re-skins were obtainable by no other means, which is why there is still a
community petition asking for the chest to be re-run.

**Opcodes**

| Opcode | Message | Kind | Status |
|---|---|---|---|
| `0x03F6` | `RequestNotifyKillEnemy` | M | Working — feeds `world.enemies_killed` |
| `0x03F7` | `RequestNotifyBuyItem` | M | Working — feeds `world.items_bought`, `world.souls_spent` |
| `0x038B` | `RegulationFileUpdatePushMessage` | P | **Never sent — parked** |
| `0x038C` | `PlayerInfoUploadConfigPushMessage` | P | **Never sent** |
| `0x0390` | NRLogging-related | — | Unidentified, never seen live |

**Notes**

- `RequestNotifyKillEnemy` batches: one message carries many `(enemy_id, count)` pairs.
- **The event schedule is strong new evidence for `0x038B`.** Every documented event has a **start
  date and an end date** — "5–12 February 2015", "26 August to 5 September" — and
  `RegulationFileDiffData` inside `RegulationFileUpdatePushMessage` carries **`start_at` and
  `end_at`**. That is not a coincidence in shape. Combined with the negative result below, `0x038B`
  is now the leading hypothesis for the chest by a wide margin, and §15 has removed its main rival.
- `0x038B` is **confirmed to parse** on PS3 — the client special-cases it and calls `ParseFromArray`
  — but has **never been shown to apply**: no param reload or file write was reached.
- **The chest is not a calibration problem.** Calibration 0114 — the only payload that changes
  `ItemLotParam2_SvrEvent.param` — installed successfully and the chest stayed empty. The lots are in
  the client's regulation and nothing appears, so something must *select* an active lot. Given the
  dated event windows above, that selection looks exactly like a time-bounded server push.
- The remaining candidates are `0x038B`, `0x038C`, and a server-set event flag we have not found.

---

## 18. The client-to-client tunnel

Not a feature a player names, but load-bearing for two of them.

| Opcode | Message | Direction |
|---|---|---|
| `0x0320` | `RequestSendMessageToPlayers` | client→server: relay request |
| `0x0320` | (transport frame) | server→client: **how every push is framed** |

**Status in dso: Working.** `relay.go`, capped at 6 recipients. That cap is **CVE-2022-24125**
mitigation: without it a client can address the entire server and inject arbitrary pushes into every
session — the same class of bug that made DS2's announcement channel abusable (§11) and that became
remote code execution in DS3.

Two push classes are never built by the server at all — `PushRequestAllowBreakInTarget` and
`PushRequestAllowQuickMatch`. The client serialises them itself and tunnels them through here; both
`0x0320` send sites in the binary sit inside functions constructing exactly those two classes.

Push transport was an open question the decompilation could not answer — the dispatcher at
`0x158C138` keys on a u32 that could have come from the header or from a parsed field. **It is
resolved:** the PC model (msg_type `0x0320`, `msg_index 0xFFFFFFFF`, identity in the protobuf's first
field) is correct on PS3, proven by a rating notification arriving live.

---

# The gap list — player-facing features we cannot account for

This is the most valuable part of the document. Everything below is something a DS2 player can
observe or do, that **no opcode in `docs/protocol-map-ps3.md` explains**. These are the things to
look for next.

### Genuine gaps — a real mechanic, no candidate opcode

1. **Way of Blue's side of the Blue Sentinel rescue.** *The highest-value item here.* A Way of Blue
   member who is being invaded is supposed to have a Sentinel pulled in to defend them, and the
   victim needs **only covenant membership** to qualify — not humanity, not an equipped ring, no
   action at all. Our Visitor implementation is entirely *requester-driven*: a Sentinel asks for a
   list and picks a target. But nothing in the protocol lets a host announce *"I am being invaded,
   send help"*, and nothing lets the server tell a Sentinel *"go here now"* unprompted. Three
   possibilities, and they are distinguishable by capture:
   - the Sentinel polls `RequestGetVisitorList` continuously and the **server** is expected to answer
     only with currently-invaded Way of Blue hosts — which makes this a *matchmaking* gap, since the
     server would need to know an invasion is in progress (it does: `0x03D3` passed through it);
   - the host's `RequestUpdatePlayerStatus` blob carries an "under invasion" flag we have never
     parsed;
   - one of the four unidentified opcodes is involved.
   This matters beyond completeness: Way of Blue was the most visibly broken covenant in the original
   release, and knowing which of the three is true would tell us whether that was a client bug or a
   server policy.

2. **Sin.** The Cracked Blue Eye Orb hunts *sinners*, and `BreakInType` has a value the schema calls
   `BlueEyeOrb`. Nothing in the protocol reports, queries or clears a sin level, and nothing in the
   status blob has been identified as carrying it. Most likely it lives inside the opaque `AllStatus`
   blob — testable by diffing the blob before and after killing an NPC — but if it does not, there is
   a message we have not found. Note that sin is also *cleared* by paying Cromwell, which would need
   to reach the server somehow.

3. **The bonfire warp screen's "best chance of connecting" hint.** Patch 1.10 made the warp menu
   **highlight the three areas with the most online activity**. That is a server-supplied population
   statistic, and **no opcode we know of carries it.** It is a strong candidate for one of `0x0387`,
   `0x0388`, `0x038A` — they fire early in boot from a single function, which is where you would ask
   for a population summary. **This is a new gap that nothing in our existing docs records.** It only
   applies to v1.10 clients, which is testable: warp on a v1.00 install versus a v1.10 one and
   compare the traffic.

4. **The event-item chest in the Majula mansion.** Confirmed *not* to be a regulation problem, and
   §15 now rules out `RequestNotifyRingBell`. `0x038B`'s `start_at`/`end_at` fields match the
   documented event windows exactly (§17), which makes it the leading candidate — but nothing has
   been shown to *apply* it, so this stays a gap.

5. **Arena map selection.** Three statues per venue, and the higher-ranked player's choice decides the
   map. `RequestRegisterQuickMatch` carries only area, cell, matching parameter and mode. Either the
   cell id distinguishes the statues — plausible, and cheap to confirm from a capture — or the map is
   negotiated peer-to-peer after brokering. Unresolved, and it will block a faithful arena
   implementation.

6. **Negative blood-message ratings.** DS2 rating is described as positive *or* negative; our schema
   exposes only a `good` counter and our handler always increments. Either the client never sends a
   negative on this platform or we are silently inverting downvotes. One log line settles it.

7. **Where announcements render.** We answer `0x03EC` and have never put content in it. Purely a
   live-test gap, and an easy one.

### Ambiguities that block features we have already written

8. **28 unassigned push aliases.** BreakIn 14, Visitor 6, QuickMatch 8. Static analysis cannot
   separate them: every registration site in a manager loads the same callback vtable and the
   distinguishing state passes through the callback object at runtime. Each is one live capture from
   being settled, and the invasion result (`0x03B9`, the *first* value of its group) is the only data
   point we have on the pattern.
   - **A declined invasion** — is it `0x03BA`?
   - **A visit** — do `0x03CF` / `0x03D0` work, or is it `0x03C9` / `0x03CC` / `0x03CF`?
   - **An arena match** — all eight unknown.

9. **The Rat King direction inversion** (§6). Our visit handler is symmetric; the game's Rat King
   mechanic is not. Untested, and one Grave of Saints session would settle it.

10. **Whether `RequestNotifyMirrorKnight` (`0x03D8`) expects the server to act.** The boss decides
    when to summon, so a message announcing that is the one part of Mirror Knight that is not just
    "signs without placement". We only log it.

11. **Four unidentified opcodes: `0x0387`, `0x0388`, `0x038A`, `0x0390`.** All emitted from TOC-B net
    functions with no reachable Frpg2 message construction, so the vtable trick yields nothing. **None
    has ever been observed live.** `0x0390` is the `NRLoggingMessage` uploader and is probably opt-in
    telemetry. The other three all fire from the same function (`0x16633A8`) early in boot — see gap
    3, which gives them their first concrete hypothesis.

12. **Whether the Champion's Tablet can render names.** We echo the client's own opaque `data` blob,
    so it should. If it renders blank, the client is resolving names through
    `RequestGetPlayerCharacter` and the two features are coupled.

### Not gaps — recorded so nobody re-investigates

- **Ringing a bell** has no online consequence in DS2. §15.
- **The Majula obelisk** is string id 100 in `regulation<Language>.fmg`, and it is the **same
  monument** whose plaque shows the world death count. Not a push of its own — but reachable, by
  replacing that FMG over `0x038B`. Done live 2026-08-07; see `docs/worn-writing.md`.
- **The Agape Ring** suppresses soul gain client-side; the server sees a frozen number.
- **The Crushed Eye Orb** is a scripted NPC invasion against Licia. No second player, no traffic.
- **Covenant rank for everything except Company of Champions** — Bell Keeper kills, Blue Sentinel
  Tokens, Brotherhood duel points, Rat Tails, Sunlight Medals, Dragon Scales. None has an opcode
  because none needs one: they are local save counters, and the game deliberately shows no arena
  record. Only the Awestone board is server-side, and that is §9.
- **Seed of a Tree of Giants**, arena healing restrictions, Company of Champions damage multipliers,
  phantom time limits, the 10-message cap, the 2-phantom cap — all client-side rules, invisible to
  the protocol.
- **`0x03FA`, `0x03FB`, `0x03FC`, `0x03FD`, `0x03FF`, `0x0400`.** Not gaps, not features: they do not
  exist in this binary. `opcodes_test.go` fails the build if any is dispatched.

---

## Corrections this document makes to our own notes

1. **Mirror Knight is the Looking Glass Knight in King's Passage, Drangleic Castle — not "the Belfry
   Sol arena".** `internal/server/game/mirrorknight.go` (file header) and `tasks/remaining-features.md`
   §6 both say Belfry Sol, which is a Bell Keeper belfry with no boss in it. The summoned player is
   **hostile**, fights *for* the boss, and gets there with a Red Sign Soapstone placed anywhere in
   the castle — which is exactly why `RequestCreateMirrorKnightSign` carries no area or cell.
2. **`RequestNotifyRingBell` is almost certainly plain telemetry, and is not the event-chest
   trigger.** §15. `tasks/remaining-features.md` §2 and `tasks/calibration-reverse-engineering.md`
   both name it as the leading chest candidate; that should be retired and the effort moved to
   `0x038B`, whose `start_at`/`end_at` fields match the documented event windows.
3. **`tasks/remaining-opcodes.md` §A3 is stale.** `RequestGetPlayerCharacter` (`0x03A9`) and
   `RequestGetPlayerCharacterList` (`0x03B5`) are implemented in `playerdata.go`. **59** request
   opcodes are dispatched, not 57.
4. **DS2's arenas are 1v1 only, and there are two of them.** "Undead Match" is DS3 terminology.
   `QuickMatchGameMode_Blue` / `_Brotherhood` are two *venues* — Cathedral of Blue and Undead
   Purgatory — not two match formats. Soul Memory does not apply in either.
5. **The world-death-count monument and the "Majula obelisk" are the same object.** Two faces, two
   investigations, one stone.
6. **A new gap nothing in our docs records:** the v1.10 warp screen's top-three-areas-by-population
   hint has no known opcode, and gives `0x0387`/`0x0388`/`0x038A` their first testable hypothesis.

---

## Sources

Player-facing behaviour only. Nothing here overrides `docs/protocol-map-ps3.md`.

**Covenants and PvP**
- [Bell Keepers — Fextralife](https://darksouls2.wiki.fextralife.com/Bell_Keepers) · [wiki.gg](https://darksouls2.wiki.gg/wiki/Bell_Keepers)
- [Bell Keeper's Seal — wiki.gg](https://darksouls2.wiki.gg/wiki/Bell_Keeper%27s_Seal)
- [Belfry Luna — Fextralife](https://darksouls2.wiki.fextralife.com/Belfry+Luna) · [wiki.gg](https://darksouls2.wiki.gg/wiki/Belfry_Luna)
- [Belfry Sol — Fextralife](https://darksouls2.wiki.fextralife.com/Belfry_Sol) · [wiki.gg](https://darksouls2.wiki.gg/wiki/Belfry_Sol)
- [Rat King Covenant — Fextralife](https://darksouls2.wiki.fextralife.com/Rat+King+Covenant) · [wiki.gg](https://darksouls2.wiki.gg/wiki/Rat_King_Covenant)
- [Blue Sentinels — Fextralife](https://darksouls2.wiki.fextralife.com/Blue_Sentinels) · [wikidot](http://darksouls2.wikidot.com/blue-sentinels)
- [Way of Blue — wiki.gg](https://darksouls2.wiki.gg/wiki/Way_of_Blue) · [Guardian's Seal — Fextralife](https://darksouls2.wiki.fextralife.com/Guardian's+Seal)
- [Brotherhood of Blood — Fextralife](https://darksouls2.wiki.fextralife.com/Brotherhood+of+Blood) · [wiki.gg](https://darksouls2.wiki.gg/wiki/Brotherhood_of_Blood)
- [Company of Champions — Fextralife](https://darksouls2.wiki.fextralife.com/Company_of_Champions) · [wiki.gg](https://darksouls2.wiki.gg/wiki/Company_of_Champions)
- [Heirs of the Sun — Fextralife](https://darksouls2.wiki.fextralife.com/Heirs_of_the_Sun)
- [Dragon Remnants — Fextralife](https://darksouls2.wiki.fextralife.com/Dragon+Remnants) · [Dragon Eye — Fandom](https://darksouls.fandom.com/wiki/Dragon_Eye_(Dark_Souls_II))
- [Sin (Dark Souls II) — Fandom](https://darksouls.fandom.com/wiki/Sin_(Dark_Souls_II))

**Items and summoning**
- [White Sign Soapstone](https://darksouls2.wiki.fextralife.com/White+Sign+Soapstone) · [Small White Sign Soapstone](https://darksouls2.wiki.fextralife.com/Small+White+Sign+Soapstone) ([wikidot](http://darksouls2.wikidot.com/small-white-sign-soapstone)) · [Red Sign Soapstone](https://darksouls2.wiki.fextralife.com/Red_Sign_Soapstone)
- [Cracked Red Eye Orb](https://darksouls2.wiki.fextralife.com/Cracked_Red_Eye_Orb) · [Cracked Blue Eye Orb](https://darksouls2.wiki.fextralife.com/Cracked+Blue+Eye+Orb) · [Crushed Eye Orb](https://darksouls2.wiki.fextralife.com/Crushed_Eye_Orb)
- [Dragon Talon — Fextralife](https://darksouls2.wiki.fextralife.com/Dragon_Talon) (a DLC key item, not multiplayer)
- [Co-op — wiki.gg](https://darksouls2.wiki.gg/wiki/Co-op) · [PvP — wiki.gg](https://darksouls2.wiki.gg/wiki/PvP)

**Matchmaking**
- [Online Matchmaking — wiki.gg](https://darksouls2.wiki.gg/wiki/Online_Matchmaking) · [wikidot](http://darksouls2.wikidot.com/online-matchmaking)
- [Soul Memory — wiki.gg](https://darksouls2.wiki.gg/wiki/Soul_Memory) · [Fextralife](https://darksouls2.wiki.fextralife.com/Soul_Memory)
- [Name-engraved Ring — Fextralife](https://darksouls2.wiki.fextralife.com/Name-engraved+Ring)
- [Agape Ring — wiki.gg](https://darksouls2.wiki.gg/wiki/Agape_Ring) · [Fextralife](https://darksouls2.wiki.fextralife.com/Agape+Ring)
- [Patch 1.10 — Fextralife](https://darksouls2.wiki.fextralife.com/Patch_1.10)
- [Soul Memory Ranges for Co-op and PvP — Steam guide](https://steamcommunity.com/sharedfiles/filedetails/?id=259425063)

**Bosses, messages, stains, ghosts**
- [Mirror Knight — Fextralife](https://darksouls2.wiki.fextralife.com/Mirror+Knight) · [Looking Glass Knight — Fextralife](https://darksouls2.wiki.fextralife.com/Looking+Glass+Knight) · [wiki.gg](https://darksouls2.wiki.gg/wiki/Looking_Glass_Knight)
- [How to be summoned by Looking Glass Knight — Steam guide](https://steamcommunity.com/sharedfiles/filedetails/?id=257137199)
- [Messages — Fandom](https://darksouls.fandom.com/wiki/Messages) · [message rating benefits — Gameranx](https://gameranx.com/updates/id/17788/article/dark-souls-2-rating-messages-provides-authors-with-benefits/)
- [Bloodstains — wiki.gg](https://darksouls2.wiki.gg/wiki/Bloodstains) · [Phantoms — wiki.gg](https://darksouls2.wiki.gg/wiki/Phantoms)

**Rankings, the death counter, and server-side content**
- [Champion's Tablet — wiki.gg](https://darksouls2.wiki.gg/wiki/Champion's_Tablet) · [Fextralife](https://darksouls2.wiki.fextralife.com/Champion's+Tablet) · [Fandom](https://darksouls.fandom.com/wiki/Champion%27s_Tablet)
- [Majula — Fextralife](https://darksouls2.wiki.fextralife.com/Majula) · [wikidot](http://darksouls2.wikidot.com/majula) · [Majula Mansion — wikidot](http://darksouls2.wikidot.com/majula-mansion)
- [Events — Fextralife](https://darksouls2.wiki.fextralife.com/Events) · [Majula chest weekly items — Giant Bomb](https://www.giantbomb.com/forums/dark-souls-ii-8967/check-the-chest-in-majulas-mansion-weekly-for-item-1487828/)
- [Online — Fextralife](https://darksouls2.wiki.fextralife.com/Online)
- [PS3/360 server shutdown — Video Games Chronicle](https://www.videogameschronicle.com/news/dark-souls-2-ps3-and-xbox-360-servers-are-shutting-down-in-march-2024/)

**Protocol context (background, not authority)**
- [Reverse engineering Dark Souls 3 networking — Tim Leonard](https://timleonard.uk/2022/06/18/reverse-engineering-dark-souls-3-networking-part-5)
- [Blue Acolyte / announcement-message abuse — LukeYui](https://www.patreon.com/LukeYui/posts/info-blue-and-of-148777896)
