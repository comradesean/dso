# DS2 PS3 (BLUS41045) — Game-Service Protocol, Recovered From the Binary

> **Provenance: DECOMPILATION.** Everything in this document was derived by static analysis of the
> **BLUS41045 `EBOOT.elf`** itself — *not* from DS3OS, and not from `docs/protocol-map.md`.
> Produced **2026-08-05**.
>
> `docs/protocol-map.md` is derived from the **DS3OS** C++ reference implementation, which targets
> **Dark Souls 2: Scholar of the First Sin on PC (Steam)**. This document targets **original Dark
> Souls 2 on PS3**. The two differ on platform *and* edition.
>
> **Where this document disagrees with `docs/protocol-map.md`, THIS document wins for PS3.**
> The disagreements are enumerated in full in §8.2.

---

## ⛔ DO NOT IMPLEMENT THESE SIX OPCODES

The PC/SOTFS map lists all six as DS2 opcodes. **This client contains no code for any of them.**
Anyone working from the PC map will implement all six by default. Don't.

| Opcode | PC/SOTFS name | Status on BLUS41045 |
|---|---|---|
| `0x03FA` | `RequestGetRightMatchingArea` | **Absent** |
| `0x03FB` | `PushRequestBreakInTarget` | **Absent** |
| `0x03FC` | `PushRequestRejectBreakInTarget` | **Absent** |
| `0x03FD` | `PushRequestAllowBreakInTarget` | **Absent** |
| `0x03FF` | `RequestGetAreaBloodMessageList` | **Absent** |
| `0x0400` | `RequestGetAreaBloodstainList` | **Absent** |

No `li r4` with any of those values exists anywhere in `.text`. The maximum opcode constant across
all 132 send/register call sites is `0x03F9`. The BreakIn pushes live at **`0x03B9`–`0x03C8`**
instead (§5.2) — a server that sends `0x03FB` will simply never be dispatched by a PS3 client.
Full evidence in §6.3 / §6.4.

---

**Target:** `EBOOT.elf`, BLUS41045, PPC64 BE, 30,756,464 bytes. File offset + `0x10000` = vaddr (re-verified: RSA PEM `-----BEGIN RSA PUBLIC KEY-----` at file `0x17EB338` / vaddr `0x17FB338`).
**Tools:** `powerpc64-linux-gnu-objdump`, `readelf`, custom Python decoders. Full `.text` disassembly (`0x10230`–`0x17DFA54`, 251 MB) produced once and indexed.
**The reference doc was not opened until the comparison section was written.**

---

## 0. Executive summary

I recovered the game-service opcode table **from the binary**, not from a switch/jump table (there isn't one — see §6 negative results), but from the four *message-send / handler-register* helper functions and their call sites. The method was **independently validated against five byte-confirmed auth opcodes** before I trusted any game opcode (§2).

**Result: 105 opcodes**, spanning `0x0320` and `0x0386`–`0x03F9`. Nothing above `0x03F9`. The PC/SOTFS reference's `0x03FA`, `0x03FB`, `0x03FC`, `0x03FD`, `0x03FF`, `0x0400` **do not exist in this binary**.

Headline contradictions vs. the DS3OS-derived map:

1. **BreakIn push opcodes are completely different.** Reference says `0x03FB/0x03FC/0x03FD`; PS3 registers a 16-opcode block at **`0x03B9`–`0x03C8`**. `0x03FB`–`0x03FD` appear nowhere.
2. **Three PC-only "DS2 extras" are absent on PS3:** `0x03FA RequestGetRightMatchingArea`, `0x03FF RequestGetAreaBloodMessageList`, `0x0400 RequestGetAreaBloodstainList`. Strongly suggests these are SOTFS additions.
3. **Five messages the reference calls "DS3-only" are present in DS2 on PS3:** `0x038D ServerPing`, `0x038E RequestMeasureUploadBandwidth`, `0x038F RequestMeasureDownloadBandwidth`, `0x03B5 RequestGetPlayerCharacterList`, `0x03B7 RequestBenchmarkThroughput`.
4. **Four opcodes exist that the reference lists for neither game:** `0x0387`, `0x0388`, `0x038A`, `0x0390`.
5. **Visitor and QuickMatch push blocks are supersets** of the reference's (9 and 8 aliases respectively, not 3 and 4).
6. **16 opcodes are sent through a send path that registers no response callback** — the reference has only one such message (`0x0391`).

### 0.1 Live-capture confirmations of this table

Confirmed against a real BLUS41045 client running against our server (these arrived *after* the
static analysis was complete, so they are independent checks of the method, not inputs to it):

| Opcode | Message | Live behaviour |
|---|---|---|
| `0x0386` | `RequestWaitForUserLogin` | First message after the rUDP session establishes. Payload observed: protobuf `{1:"<psn-id>", 2:2, 3:0, 4:1, 5:2}`. Server replies `{1:steam_id, 2:player_id}`; client accepts. |
| `0x03EC` | `RequestGetAnnounceMessageList` | Sent immediately after `0x0386`; 2-byte payload. |
| `0x03B6` | `RequestUpdateLoginPlayerCharacter` | Request/response, with `character_id` allocation semantics — exactly as this table predicts. |

Three-for-three against live traffic, on top of the six-for-six auth validation in §2.

Also confirmed live and unchanged from the existing notes: the reliable-UDP layer (7-byte header, magic `F5 02`, two packed 12-bit sequence counters, 1-byte opcode, trailing `0xFF`).

`Frpg2GameServerInfo` was solved separately and is **not** re-derived here: the client requires a payload of exactly **56 bytes** (hard equality check at vaddr `0x167091C`), with a binary `u32` IP at offset 8, `u16` port at 12, and ten trailing `u32` transport params from offset 16. See §7 for a binary-derived cross-check on those ten params.

---

## 1. How the binary is laid out (needed to read everything below)

### 1.1 Two TOC bases

`.opd` is at `0x1CC2E90`, size `0x541D8`, 8-byte descriptors `{u32 code, u32 toc}`. Descriptor TOC values are **not** uniform:

| TOC value | # functions |
|---|---|
| `0x01D1F068` (call it **TOC-A**) | 36,230 |
| `0x01D2EFE0` (**TOC-B**) | 6,837 |

**The entire Frpg2 network stack + protobuf runtime lives in the TOC-B group.** This matters enormously: a naive `lwz rD, disp(r2)` resolver that assumes one TOC base produces silent garbage. (I burned an hour on exactly that before catching it.) Verified: `lwz r0,-11156(r2)` at `0xF73E24` resolves under TOC-A to `0x1D1C4D4` (an EzState symbol table), not the login strings it superficially appeared to reference.

### 1.2 Protobuf is 2.5.0 LITE_RUNTIME

- `0x17E62B8`: `N:\FRPG2SV\src\auxlib\client\protobuf-2.5.0-src\src\google\protobuf\io\coded_stream.cc`
- `0x1896548`: `N:\FRPG2SV\src\Game\Common\src\Frpg2RequestMessage.pb.cc`
- `0x17E67F8` region: the `%d.%d.%d` version-mismatch strings.

There is **no serialized `FileDescriptorProto`** in the image — `grep -abo ".proto"` returns zero hits containing `Frpg2`. Consequence: **the opcode enum is not a protobuf enum in this build.** It is a C++ enum, materialised only as `li rX, imm` immediates at call sites. That is why no opcode array exists in `.rodata` (§6).

### 1.3 The message-type name table (the key that unlocked everything)

Because LITE_RUNTIME still generates `MessageLite::GetTypeName()`, every message class has a tiny function returning its fully-qualified name.

- **212 name strings**, `0x18965B8` … `0x1898FC8`, pointed at by a contiguous TOC-B slot run `0x1D2BC14` … `0x1D2BF60` (TOC-B offsets `-0x33CC` … `-0x3080`).
- **212 `GetTypeName()` functions**, `0x15FEAA4` … `0x1601F64`, **exactly `0x40` bytes apart**, emitted in reverse `.proto` declaration order (`RequestQueryLoginServerInfo`, declared first, is last at `0x1601F64`).
- **212 vtables**, first function slot at `0x1C5F948` and every `0x40` bytes thereafter. `GetTypeName` sits at vtable+8; three zero words precede each vtable.

From this I built a complete map **vtable pointer → message class name**, and then found every `lwz rD, disp(r2)` that loads a message vtable pointer — i.e. every constructor / placement-new of a specific Frpg2 message, anywhere in the 30 MB image. That is the substrate for everything below.

**Confidence: high.** Every one of the 212 names resolves cleanly and the `0x40` stride is exact.

---

## 2. Validating the method against byte-confirmed ground truth

Before trusting any game opcode I applied the identical technique to the **auth/login state machine**, whose opcodes are already byte-confirmed against the live client.

Function `0x166DC88` (TOC-B) is the login task. Disassembly, in program order:

| Site | Instruction | Preceding class ctor | Sender called | Derived type |
|---|---|---|---|---|
| `0x166E058` | `li r4,5` | `RequestQueryLoginServerInfo` vptr @ `0x166DD60` | `bl 0x166BBD0` | **5** |
| `0x166E3A0` | `li r4,2` | `GetServiceStatus` vptr @ `0x166E25C` | `bl 0x166CAE8` | **2** |
| `0x166E440` | `li r4,6` | — | `bl 0x16731B4` | **6** |
| `0x166E79C` | `li r4,3` | — | `bl 0x166ACC0` | **3** |
| `0x166EBD8` | `li r4,1` | — | `bl 0x16731B4` | **1** |
| `0x166F1E4` | `li r4,6` | `RequestHandshake` vptr @ `0x166F0BC` | `bl 0x166BBD0` | **6** |

```
 166e3a0:	38 80 00 02 	li      r4,2
 166e3ac:	4b ff e7 3d 	bl      0x166cae8
 ...
 166e440:	38 80 00 06 	li      r4,6
 166e444:	38 a0 00 00 	li      r5,0
 166e448:	38 c0 00 00 	li      r6,0
 166e44c:	48 00 4d 69 	bl      0x16731b4
```

This reproduces the verified set exactly: **1 = KeyMaterial, 2 = GetServiceStatus, 3 = ticket, 5 = login/QueryLoginServerInfo, 6 = RequestHandshake**. Five independent hits, zero misses.

**Conclusion: the `li r4, <constant>` immediately preceding a Frpg2 send helper IS the wire `msg_type`.** Confidence in the *method*: **high**. (Subsequently reinforced by three live-traffic confirmations — see §0.1.)

---

## 3. The four game-service send/register helpers

| Address | Class name string | Role | Call sites |
|---|---|---|---|
| `0x15899F0` | `RequestStatusForDefault` (`0x1895728`) | send request, register response callback | **48** |
| `0x15878A8` | `RequestStatusForPolling` (`0x1895740`) | await/poll response for opcode | **26** |
| `0x1587DE8` | (same base class family) | send, **no response callback** | **17** |
| `0x1588900` | — | **register push-message handler** for opcode | **41** |

Every single one of the 132 call sites passes a compile-time constant in `r4`; there are **zero** call sites with a computed `r4`.

Evidence that `r4` is stored as the opcode — `0x15899F0` allocates a 60-byte object and writes `r4` (saved in `r27`) at `+36`:

```
 1589a20:	7c 9b 23 78 	mr      r27,r4
 1589a8c:	38 60 00 3c 	li      r3,60          ; operator new(60)
 1589a90:	48 25 44 15 	bl      0x17ddea4
 1589ab4:	80 02 c2 c8 	lwz     r0,-15672(r2)  ; vptr -> 0x1C5F400 "RequestStatusForDefault"
 1589ab8:	93 7f 00 24 	stw     r27,36(r31)    ; <-- opcode stored at +0x24
 1589abc:	90 1f 00 00 	stw     r0,0(r31)
```

Class-name virtuals confirming the identity:
```
 158c360:	80 62 c2 d4 	lwz     r3,-15660(r2)   ; -> 0x1895728 "RequestStatusForDefault"
 158c368:	80 62 c2 d8 	lwz     r3,-15656(r2)   ; -> 0x1895740 "RequestStatusForPolling"
```

That `0x1588900` really registers *push* handlers (server→client) is proven two ways in §5.

**Confidence: high** for `0x15899F0` / `0x15878A8` / `0x1588900`. **Medium-high** for `0x1587DE8` (see §4.2 caveat).

---

## 4. The recovered opcode table

Legend: **R/R** = request with response callback (`0x15899F0`, plus a `0x15878A8` witness where present) · **M** = sent with no response callback (`0x1587DE8`) · **P** = push handler registered by the client (`0x1588900`).

Message names come from the *nearest preceding* Frpg2 message-class vtable load inside the same function. Where two independent witnesses exist (send site + response-parse site) they agree in every case.

### 4.1 Confirmed opcode → message

| Opcode | Kind | Message | Send site / register site |
|---|---|---|---|
| `0x0320` | M | `RequestSendMessageToPlayers` | `0x1575DFC`, `0x1597DA4` |
| `0x0386` | — | `RequestWaitForUserLogin` | `0x166FF4C` (fn `0x166DC88` → `bl 0x1664470`) |
| `0x0387` | — | *(unidentified)* | `0x16638F4` (fn `0x16633A8` → `bl 0x1798AA0`) |
| `0x0388` | — | *(unidentified)* | `0x1663994` |
| `0x0389` | P | *(push, special-cased)* | dispatcher `0x158C1D8` → handler `0x1587F60` |
| `0x038A` | — | *(unidentified)* | `0x1663A34` |
| `0x038B` | P | *(push, special-cased)* | dispatcher `0x158C1E0` → handler `0x158B150` |
| `0x038C` | P | *(push, special-cased)* | dispatcher `0x158C1E8` → handler `0x1588218` |
| `0x038D` | R/R | `ServerPing` | `0x1559118`; resp `0x155833C` |
| `0x038E` | R/R | `RequestMeasureUploadBandwidth` | `0x155AB04`; resp `0x1559710` |
| `0x038F` | R/R | `RequestMeasureDownloadBandwidth` | `0x1559588`; resp `0x1559408` |
| `0x0390` | — | *(NRLogging-related)* | `0x166806C` (fn `0x1667770`, builds `NRLoggingMessage` @ `0x16678A0`) |
| `0x0391` | **M** | `RequestCreateBloodstain` | `0x1592F6C` |
| `0x0392` | R/R | `RequestGetBloodstainList` | `0x1592238`; resp `0x1594CDC` |
| `0x0393` | R/R | `RequestGetDeadingGhost` | `0x15923F4`; resp `0x15931A4` |
| `0x0394` | R/R | `RequestCreateSign` | `0x1577CE8`; resp `0x15765F0` |
| `0x0395` | R/R | `RequestUpdateSign` | `0x1576A14` |
| `0x0396` | R/R | `RequestRemoveSign` | `0x157670C` |
| `0x0397` | R/R | `RequestGetSignList` | `0x15770B8`; resp `0x1577FAC` |
| `0x0398` | R/R | `RequestSummonSign` | `0x1577930` |
| `0x039A` | R/R | `RequestRejectSign` | `0x1576898` |
| `0x039B` | P | **`PushRequestSummonSign`** | reg `0x15773E4`; dispatch `0x1579100` → `0x15791F8` |
| `0x039C` | P | **`PushRequestRejectSign`** | reg `0x1577480`; dispatch `0x15790F0` → `0x1579180` |
| `0x039D` | P | **`PushRequestRemoveSign`** | reg `0x157751C`; dispatch `0x15790F8` → `0x1579680` |
| `0x039E` | R/R | `RequestCreateMirrorKnightSign` | `0x1560A74`; resp `0x155F698` |
| `0x039F` | R/R | `RequestUpdateMirrorKnightSign` | `0x155FEAC` |
| `0x03A0` | R/R | `RequestRemoveMirrorKnightSign` | `0x155F79C` |
| `0x03A1` | R/R | `RequestGetMirrorKnightSignList` | `0x155FCEC`; resp `0x1562B14` |
| `0x03A2` | R/R | `RequestSummonMirrorKnightSign` | `0x15606AC` |
| `0x03A4` | R/R | `RequestRejectMirrorKnightSign` | `0x155FA40` |
| `0x03A5` | P | **`PushRequestSummonMirrorKnightSign`** | reg `0x15600A0`; dispatch `0x1563770` → `0x1563868` |
| `0x03A6` | P | **`PushRequestRejectMirrorKnightSign`** | reg `0x156013C`; dispatch `0x1563760` → `0x15637F0` |
| `0x03A7` | P | **`PushRequestRemoveMirrorKnightSign`** | reg `0x15601D8`; dispatch `0x1563768` → `0x1563C9C` |
| `0x03A8` | M | `RequestUpdatePlayerCharacter` | `0x157E1A4` |
| `0x03A9` | R/R | `RequestGetPlayerCharacter` | `0x1567EB8`; resp `0x15686A8` |
| `0x03AA` | P | `PushRequestEvaluateBloodMessage` *(name inferred)* | reg `0x158ECEC`; dispatch `0x158F0A8` → `0x158F114` |
| `0x03AB` | R/R | `RequestCreateBloodMessage` | `0x158F498`; resp `0x158E170` |
| `0x03AC` | R/R | `RequestRemoveBloodMessage` | `0x158EA3C` |
| `0x03AD` | R/R | `RequestReentryBloodMessage` | `0x158E8C8` |
| `0x03AE` | R/R | `RequestGetBloodMessageList` | `0x158E768`; resp `0x1591324` |
| `0x03AF` | R/R | `RequestEvaluateBloodMessage` | `0x158E400` |
| `0x03B0` | R/R | `RequestGetBloodMessageEvaluation` | `0x158E28C`; resp `0x158E000` |
| `0x03B1` | R/R | `RequestCreateGhostData` | `0x155D320` |
| `0x03B2` | R/R | `RequestGetGhostDataList` | `0x155CFC4`; resp `0x155ED14` |
| `0x03B3` | R/R | `RequestGetLoginPlayerCharacter` | `0x15671EC`; resp `0x156A520` |
| `0x03B5` | R/R | **`RequestGetPlayerCharacterList`** | `0x1567D1C`; resp `0x156AC04` |
| `0x03B6` | R/R | `RequestUpdateLoginPlayerCharacter` | `0x1567A0C`; resp `0x1566FB8` — **confirmed live** |
| `0x03B7` | R/R | **`RequestBenchmarkThroughput`** | `0x155A27C`; resp `0x155929C` |
| `0x03B8` | M | `RequestUpdatePlayerStatus` | `0x1580094` |
| `0x03B9`–`0x03C8` | P ×16 | **BreakIn push block** (see §5.2) | regs `0x15964EC`…`0x1596910` |
| `0x03C9`–`0x03D1` | P ×9 | **Visitor push block** | regs `0x157A624`…`0x157AB38` |
| `0x03D2` | R/R | `RequestGetBreakInTargetList` | `0x1595AC8`; resp `0x159A4CC` |
| `0x03D3` | R/R | `RequestBreakInTarget` | `0x1595664` |
| `0x03D4` | R/R | `RequestRejectBreakInTarget` | `0x15957F0` |
| `0x03D5` | R/R | `RequestGetVisitorList` | `0x157A248`; resp `0x157CE44` |
| `0x03D6` | R/R | `RequestVisit` | `0x157AFF0` |
| `0x03D7` | R/R | `RequestRejectVisit` | `0x1579F60` |
| `0x03D8` | M | `RequestNotifyMirrorKnight` | `0x15650C4` |
| `0x03D9` | R/R | `RequestRegisterQuickMatch` | `0x157175C` |
| `0x03DA` | R/R | `RequestUnregisterQuickMatch` | `0x1572108` |
| `0x03DB` | R/R | `RequestUpdateQuickMatch` | `0x1572274` |
| `0x03DC` | R/R | `RequestSearchQuickMatch` | `0x1571F24`; resp `0x1574FCC` |
| `0x03DD` | R/R | `RequestJoinQuickMatch` | `0x1571C28` |
| `0x03DE` | R/R | `RequestRejectQuickMatch` | `0x1571A90` |
| `0x03E0`–`0x03E7` | P ×8 | **QuickMatch push block** | regs `0x15724AC`…`0x157291C` |
| `0x03E8` | M | `RequestNotifyJoinGuestPlayer` | `0x15656B4` |
| `0x03E9` | M | `RequestNotifyLeaveGuestPlayer` | `0x1565354` |
| `0x03EA` | M | `RequestNotifyJoinSession` | `0x1564A3C` |
| `0x03EB` | M | `RequestNotifyLeaveSession` | `0x1565898` |
| `0x03EC` | R/R | `RequestGetAnnounceMessageList` | `0x1558870`; resp `0x155C2D4` — **confirmed live** |
| `0x03ED` | M | `RequestNotifyKillPlayer` | `0x15654C4` |
| `0x03EE` | M | `RequestNotifyRingBell` | `0x1566040` |
| `0x03EF` | P | *(session-disconnect push)* | reg `0x1565BC0`; dispatch `0x156646C` → `0x15664E8` |
| `0x03F0` | R/R | `RequestGetTotalDeathCount` | `0x15675E0`; resp `0x1566E50` |
| `0x03F1` | M | `RequestNotifyDeath` | `0x1564F04` |
| `0x03F2` | M | `RequestNotifyOfflineDeathCount` | `0x1564D28` |
| `0x03F3` | R/R | `RequestRegisterPowerStoneData` | `0x1568254` |
| `0x03F4` | R/R | `RequestGetPowerStoneRanking` | `0x15674A0`; resp `0x156CE18` |
| `0x03F5` | R/R | `RequestGetPowerStoneMyRanking` | `0x1567340`; resp `0x156B59C` |
| `0x03F6` | M | `RequestNotifyKillEnemy` | `0x158D2EC` |
| `0x03F7` | M | `RequestNotifyBuyItem` | `0x1564BA0` |
| `0x03F8` | R/R | `RequestGetPowerStoneRankingRecordCount` | `0x1567718`; resp `0x1566D20` |
| `0x03F9` | M | `RequestNotifyDisconnectSession` | `0x15659D8` |

**Confidence: high** for every row that carries a message name and both a send and a response witness (≈80 rows). **Medium-high** for send-only rows. **Medium** for `0x03AA`, `0x03EF` (opcode certain, name inferred from the enclosing manager). **Opcode certain / name unknown** for `0x0387`, `0x0388`, `0x038A`, `0x0389`, `0x038B`, `0x038C`, `0x0390`, and the three push blocks.

### 4.2 The "no response callback" set (M)

These 16 opcodes are sent via `0x1587DE8`, which — unlike `0x15899F0` — does **not** allocate a response-tracking object:

`0x0320, 0x0391, 0x03A8, 0x03B8, 0x03D8, 0x03E8, 0x03E9, 0x03EA, 0x03EB, 0x03ED, 0x03EE, 0x03F1, 0x03F2, 0x03F6, 0x03F7, 0x03F9`

**What I can say:** the PS3 client never parses a reply *body* for these. **What I cannot say:** whether the transport still expects a `Reply` frame (I did not locate the header serialiser — §6.6). Treat as **medium** confidence and a practical warning: the reference classifies most of these as request/response.

---

## 5. Push messages

### 5.1 The push dispatcher

Function `0x158C138`. It compares an incoming u32 against three special-cased values and otherwise does a red-black-tree lookup in a handler map:

```
 158c1d4:	80 81 00 94 	lwz     r4,148(r1)
 158c1d8:	2f 84 03 89 	cmpwi   cr7,r4,905      ; 0x389
 158c1dc:	41 9e 01 44 	beq     cr7,0x158c320
 158c1e0:	2f 84 03 8b 	cmpwi   cr7,r4,907      ; 0x38B
 158c1e4:	41 9e 01 50 	beq     cr7,0x158c334
 158c1e8:	2f 84 03 8c 	cmpwi   cr7,r4,908      ; 0x38C
 158c1ec:	41 9e 01 5c 	beq     cr7,0x158c348
 158c1f0:	38 7f 00 28 	addi    r3,r31,40       ; map root
 ...
 158c218:	80 0a 00 0c 	lwz     r0,12(r10)      ; node key
 158c21c:	7f 80 20 40 	cmplw   cr7,r0,r4       ; compare against opcode
```

The map is populated by the 41 `0x1588900` calls. Two independent confirmations that the map keys are wire push opcodes:

- **Sign manager** (`0x1579060`): direct `cmpwi` on `0x39C/0x39D/0x39B`, and the branch targets load `PushRequestRejectSign` (`0x1579194`) and `PushRequestSummonSign` (`0x157920C`) vtables.
- **MirrorKnight manager** (`0x15636D0`): `cmpwi` on `0x3A6/0x3A7/0x3A5`, targets load `PushRequestRejectMirrorKnightSign` (`0x1563804`), `PushRequestRemoveMirrorKnightSign` (`0x1563CB0`), `PushRequestSummonMirrorKnightSign` (`0x156387C`).

Both sets match the reference's DS2 push numbers exactly. **Confidence: high.**

### 5.2 The three large push *alias blocks* — the biggest divergence

Three subsystems register far more push opcodes than there are distinct push message types, in obvious grouped runs:

**BreakIn — 16 opcodes `0x03B9`–`0x03C8`**, registered in the region `0x1595E2C`–`0x1596980` in four groups of four (call-site order): `(0x3BD,0x3BE,0x3C0,0x3BF)`, `(0x3C1,0x3C2,0x3C4,0x3C3)`, `(0x3B9,0x3BA,0x3BC,0x3BB)`, `(0x3C5,0x3C6,0x3C8,0x3C7)`. The BreakIn subsystem has exactly four push classes in the proto (`PushRequestBreakInTarget` @`0x1598B68`, `PushRequestAllowBreakInTarget` @`0x1597898`/`0x1598BF8`, `PushRequestRejectBreakInTarget` @`0x1598F90`, `PushRequestRemoveBreakInTarget`), i.e. **4 message types × 4 aliases**.

**Visitor — 9 opcodes `0x03C9`–`0x03D1`**, three groups of three; three push classes (`PushRequestVisit` @`0x157C20C`, `PushRequestRejectVisit` @`0x157C6CC`, `PushRequestRemoveVisitor` @`0x157C658`) — **3 × 3**.

**QuickMatch — 8 opcodes `0x03E0`–`0x03E7`**, two interleaved groups of four (odds `0x3E1,0x3E3,0x3E5,0x3E7` registered first, then evens `0x3E0,0x3E2,0x3E4,0x3E6`); four push classes — **4 × 2**.

I could **not** determine which alias maps to which message type: every registration site in a given manager loads the same callback vtable (e.g. `0x1C5F530` at `0x1595FD0`, `0x159626C`, `0x15964C4`, `0x1596714`), and the distinguishing state is passed through the callback object, not visible statically.

The reference explicitly notes DS3's QuickMatch pushes have "7 further aliases", so the alias mechanism is real and known — but the **DS2 alias values on PS3 do not match the DS3OS DS2 `.inc`**.

**Confidence: high** that these 33 opcodes are registered push handlers. **Low/none** on the individual name mapping — I am deliberately not guessing.

---

## 6. Negative results (please read these; they are load-bearing)

1. **There is no opcode dispatch switch or jump table.** I scanned the entire image for runs of ≥24 consecutive plausible code addresses within a `0x30000` span: 13 candidates, all in `.data` regions that decode as float/audio data. I also scanned all data (`0x17D38E0`→EOF) for u32 values in `[0x300,0x4A0]`: **only 417 total**, none in an arithmetic progression consistent with an opcode table. **The opcode enum exists only as immediates in code.**

2. **The one large PPC switch in the net module is not the opcode dispatch.** Function `0x1586190`:
   ```
   1586230:	2b 89 03 25 	cmplwi  cr7,r9,805
   1586234:	41 9d 0c d8 	bgt     cr7,0x1586f0c
   1586238:	81 62 c2 90 	lwz     r11,-15728(r2)   ; -> 0x1586254
   1586240:	7c 09 58 2e 	lwzx    r0,r9,r11
   1586250:	4e 80 04 20 	bctr
   ```
   806-entry offset table at `0x1586254`, 92 distinct targets, index groups `0–4, 100–115, 200–206, 300–314, 400–408, 500–507, …`. That grouping is an **internal client job/state enum**, not the wire opcode space (which is `0x320`/`0x386`–`0x3F9`). Do not mistake it for the protocol table.

3. **`0x03FA`, `0x03FB`, `0x03FC`, `0x03FD`, `0x03FE` appear nowhere as opcodes.** No `li r4` with those values exists anywhere in `.text`. The maximum `r4` constant across all 132 send/register call sites is `0x03F9`.

4. **`0x03FF` and `0x0400` are not opcodes here.** All `0x400` immediates in the net region are `li r8,0x400` / `li r5,0x400` (buffer sizes, at `0x1586F84`, `0x1587044`, `0x1587104`, `0x15871C4`, `0x1587284`, `0x1587344`, `0x1659F3C`, `0x1659F54`, `0x1679050`); the sole `0x3FF` (`0x1669834`) is `cmplwi cr7,r4,1023` in a boolean range predicate, not a dispatch.

5. **Unused within the block:** `0x0399`, `0x03A3`, `0x03B4`, `0x03DF`. Also nothing in `0x0321`–`0x0385`.

6. **I did not locate the 12-byte message-header serialiser**, so I can neither confirm nor contradict `header_size=12` / BE `msg_type` / LE `msg_index` from the binary. Two dead ends worth recording so nobody repeats them:
   - There are **zero** byte-reverse instructions (`stwbrx`, `lwbrx`, `sthbrx`, `lhbrx`) in the whole `.text`. LE conversion, if any, is done with shifts, so that shortcut does not exist.
   - The `li rX,12` sites in the net module (`0x165F8C0`, `0x1663498`) are **not** header code — see §7.

7. **The opcode enum has no string table.** There are no `"RequestCreateSign"`-style bare enum-name strings; the only names are protobuf type names, which is why the vtable route was necessary.

---

## 7. Bonus: the client's own default transport-parameter block

Since `Frpg2GameServerInfo` carries **ten trailing u32 transport params from offset 16**, this is worth flagging. At `0x165F890`–`0x165F930` and again at `0x1663470`–`0x16634F0` the client builds **two adjacent 10×u32 blocks on the stack** before handing them to the connection setup:

`0x165F890` block 1 (stack `+112`…`+148`):
`[16384, 16384, 20480, 20480, 128, 16384, 20480, 300000, 25000, 12]`

`0x165F890` block 2 (stack `+152`…`+188`):
`[0x01000000, 0x01000000, 0x01100000, 0x01100000, 128, 16384, 20480, 20000, 300000, 15]`

(`0x1663470` builds a near-identical pair with `163840` in place of `16384`.) These are almost certainly the client-side *defaults* for the same ten fields the server sends in the 56-byte `Frpg2GameServerInfo` struct — useful as a sanity range if any of those params turn out to matter. **Confidence: medium** (the 10-wide grouping and the timing are strong; I did not trace them into the struct).

---

## 8. Comparison against `docs/protocol-map.md` (PC/SOTFS, DS3OS-derived)

### 8.1 Confirmed (binary agrees with the DS3OS-derived map)

- **Auth message types** `0=Reply, 1=KeyMaterial, 2=GetServiceStatus, 3=ticket, 5=RequestQueryLoginServerInfo, 6=RequestHandshake` — six-for-six (§2). **High.**
- **`0x0320 = RequestSendMessageToPlayers`, client→server.** Confirmed at two sites (`0x1575DFC`, `0x1597DA4`). **High.**
- The reference's claim that `PushRequestAllowQuickMatch` / `PushRequestAllowBreakInTarget` are **"client-relayed only"** is directly visible: both `0x0320` send sites sit inside functions that construct exactly those two push classes (`0x1575B1C`, `0x1597C38`). **High.**
- **`0x0386 RequestWaitForUserLogin`** — derived independently at `0x166FF4C` inside the login task that constructs `RequestWaitForUserLogin` (`0x166F494`); also confirmed by live capture. **High.**
- **`0x0389` and `0x038C` are pushes** — both special-cased in the push dispatcher. **High.**
- Every named `R/R` opcode in the range `0x0391`–`0x03B3`, `0x03B6`, `0x03B8`, `0x03D2`–`0x03DE`, `0x03E8`–`0x03F9`, and `0x03EC`: **identical numbering and identical message names.** ~55 opcodes matched exactly. **High.**
- **Sign pushes `0x039B/0x039C/0x039D`** and **MirrorKnight pushes `0x03A5/0x03A6/0x03A7`** match exactly, name-for-name. **High.**
- `0x0391 RequestCreateBloodstain` is indeed in the no-response-callback set. **High.**

### 8.2 Contradicted

| # | Reference says | Binary says | Confidence |
|---|---|---|---|
| 1 | BreakIn pushes are `0x03FB`/`0x03FC`/`0x03FD` | Those values **do not exist**. BreakIn registers **16** push handlers at `0x03B9`–`0x03C8` | **High** on the numbers; the reference's values would never be dispatched by a PS3 client |
| 2 | DS2 has `0x03FA RequestGetRightMatchingArea` | **Absent** | High |
| 3 | DS2 has `0x03FF RequestGetAreaBloodMessageList` | **Absent** | High |
| 4 | DS2 has `0x0400 RequestGetAreaBloodstainList` | **Absent** | High |
| 5 | `0x038D ServerPing` is DS3-only | Present in DS2/PS3 as a full request/response pair | High |
| 6 | `0x038E`/`0x038F` bandwidth measurement are DS3-only | Present in DS2/PS3, **same numbers**, request/response | High |
| 7 | `RequestGetPlayerCharacterList` is DS3-only at `0x03A1` | Present in DS2/PS3 at **`0x03B5`** | High |
| 8 | `RequestBenchmarkThroughput` is DS3-only at `0x03A3` | Present in DS2/PS3 at **`0x03B7`** | High |
| 9 | `RegulationFileUpdatePushMessage` (`0x038B`) is DS3-only, never sent | PS3 DS2 **special-cases `0x038B`** in its push dispatcher, ahead of the handler map | High that `0x038B` is a live push opcode; medium on the name |
| 10 | `0x03C9` is `PushRequestNotifyRingBell`, "not registered" | `0x03C9` **is** registered — as the first entry of the Visitor push block | High |
| 11 | Visitor pushes are `0x03CF`/`0x03D0`/`0x03D1` | Those three exist, but they are the **last group of nine** (`0x03C9`–`0x03D1`) | High |
| 12 | QuickMatch pushes are `0x03E1`/`0x03E3`/`0x03E5`/`0x03E7` | Those four exist, but so do `0x03E0`/`0x03E2`/`0x03E4`/`0x03E6` — **eight** total | High |
| 13 | Only `0x0391` is "no reply" in DS2 | **16 opcodes** are sent with no response callback (§4.2) | Medium (see caveat) |
| 14 | "~90 opcodes, roughly `0x0320`–`0x0463`" | **105 opcodes**, `0x0320` + `0x0386`–`0x03F9`. Nothing at or above `0x03FA` | High |

**New on PS3, present in neither the reference's DS2 nor DS3 tables:** `0x0387`, `0x0388`, `0x038A` (all three sent from function `0x16633A8` via `bl 0x1798AA0`), and `0x0390` (sent from `0x1667770`, the `NRLoggingMessage` builder).

### 8.3 Neither source can currently answer

- **What `0x0387`, `0x0388`, `0x038A`, `0x0390` are.** They are emitted from TOC-B net-module functions that contain no Frpg2 message-class construction in the reachable window, so the vtable trick yields nothing. Raising confidence would need dynamic tracing or reading the encrypted-blob layout at those call sites.
- **Which alias in each push block corresponds to which push message type** (the 33 opcodes in §5.2). Static analysis cannot separate them; a live capture of a BreakIn/visit/arena summon would settle it immediately, and is by far the cheapest way to close this gap.
- **Whether the push transport opcode is `0x0320` with `msg_index = 0xFFFFFFFF`, as the reference claims.** The PS3 dispatcher (`0x158C138`) keys on a u32 that the *caller* has already deposited at `148(r1)`; I could not determine statically whether that u32 came from the transport header or from a parsed protobuf field. Both models are consistent with what I see. **This is an open question and I am explicitly not resolving it.**
- **Whether the "M" opcodes still require a `Reply` frame on the wire.**
- **The 12-byte message header layout** — not derivable from this pass (§6.6). The existing byte-confirmed facts remain the only source.

---

## 9. Suggested next steps, in value order

1. Reply to a PS3 client's `0x03D2 RequestGetBreakInTargetList` and drive a real invasion; capture which of `0x03B9`–`0x03C8` the server must use. That single capture converts 16 low-confidence opcodes to high.
2. Same for a visit (`0x03C9`–`0x03D1`) and an arena match (`0x03E0`–`0x03E7`).
3. Log what the client sends for `0x0387`/`0x0388`/`0x038A`/`0x0390` during boot — they fire early (the `0x0390` path is the `NRLoggingMessage` uploader, so it may be an opt-in telemetry channel you can safely stub).
4. **Do not implement `0x03FA`, `0x03FB`, `0x03FC`, `0x03FD`, `0x03FF`, `0x0400`.** This client has no code for them.
