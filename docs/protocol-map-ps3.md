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

## ⛔ DO NOT IMPLEMENT THESE FIVE OPCODES

The PC/SOTFS map lists six as DS2 opcodes. **The v1.00 client contains no code for any of them,
and five are still absent in v1.10.** Anyone working from the PC map will implement all six by
default. Don't.

| Opcode | PC/SOTFS name | v1.00 | v1.10 |
|---|---|---|---|
| `0x03FA` | `RequestGetRightMatchingArea` | **Absent** | ✅ **PRESENT — implement it** (`li r4,0x03FA` ×2; sent live at boot with `{1: MatchingParameter}`) |
| `0x03FB` | `PushRequestBreakInTarget` | **Absent** | **Absent** |
| `0x03FC` | `PushRequestRejectBreakInTarget` | **Absent** | **Absent** |
| `0x03FD` | `PushRequestAllowBreakInTarget` | **Absent** | **Absent** |
| `0x03FF` | `RequestGetAreaBloodMessageList` | **Absent** | **Absent** |
| `0x0400` | `RequestGetAreaBloodstainList` | **Absent** | **Absent** |

No `li r4` with any of the five remaining values exists anywhere in `.text` in either build. The
BreakIn pushes live at **`0x03B9`–`0x03C8`** instead (§5.2) — a server that sends `0x03FB` will
simply never be dispatched by a PS3 client. Full evidence in §6.3 / §6.4.

**The real BreakIn rejection push is `0x03BA`** (mode 0; `0x03BE`/`0x03C2`/`0x03C6` for the other
three invasion modes) — see §5.2, resolved 2026-08-05.

---

**Target:** `EBOOT.elf`, BLUS41045, PPC64 BE, 30,756,464 bytes. File offset + `0x10000` = vaddr (re-verified: RSA PEM `-----BEGIN RSA PUBLIC KEY-----` at file `0x17EB338` / vaddr `0x17FB338`).

> **BUILD VERSION — read this first.** Unless a section says otherwise, every address in this
> document is from the **v1.00** disc EBOOT at
> `games/DARK SOULS Ⅱ [BLUS41045]/PS3_GAME/USRDIR/EBOOT.elf` (30,756,464 B, net-module TOC
> `0x1D2EFE0`). **The clients we test against run the v1.10 title update**, at
> `dev_hdd0/game/BLUS41045/USRDIR/EBOOT.elf` (31,301,872 B, net-module TOC `0x1DB3530`).
> Addresses do not carry over between builds and the opcode sets are not identical (`0x03FA`
> is v1.10-only). §5.2 is dual-annotated for both builds; nothing else is.

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
| `0x03B9`–`0x03C8` | P ×16 | **BreakIn push block** — `0x3B9 + 4×mode + role`, role `0`=Target `1`=Reject `2`=Allow `3`=**dead** (§5.2) | regs `0x15964EC`…`0x1596910` |
| `0x03C9`–`0x03D1` | P ×9 | **Visitor push block** — `0x3C9 + 3×mode + role`, role `0`=Visit `1`=RejectVisit `2`=RemoveVisitor (§5.2) | regs `0x157A624`…`0x157AB38` |
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
| `0x03E0`–`0x03E7` | P ×8 | **QuickMatch push block** — `0x3E0 + 2×role + (1−mode)` (**mode-inverted**, §5.2.4), role `0`=Join `1`=Reject `2`=Allow `3`=Remove | regs `0x15724AC`…`0x157291C` |
| `0x03E8` | M | `RequestNotifyJoinGuestPlayer` | `0x15656B4` |
| `0x03E9` | M | `RequestNotifyLeaveGuestPlayer` | `0x1565354` |
| `0x03EA` | M | `RequestNotifyJoinSession` | `0x1564A3C` |
| `0x03EB` | M | `RequestNotifyLeaveSession` | `0x1565898` |
| `0x03EC` | R/R | `RequestGetAnnounceMessageList` | `0x1558870`; resp `0x155C2D4` — **confirmed live** |
| `0x03ED` | M | `RequestNotifyKillPlayer` | `0x15654C4` |
| `0x03EE` | M | `RequestNotifyRingBell` | `0x1566040` |
| `0x03EF` | P | `PushRequestNotifyRingBell` | v1.10: reg `0x15CFCD8` (`li r4,0x3EF`); handler `0x15D0528` → `0x15D0630`; ctor `0x16418E4`, vtable `0x1CE1AE0`; `GetTypeName` confirms the name. Serializer `0x1634BA0`: `1 uint32`, `2 uint32`, `3 uint32`, `4 bytes` |
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

**Confidence: high** for every row that carries a message name and both a send and a response witness (≈80 rows). **Medium-high** for send-only rows. **Medium** for `0x03AA` (opcode certain, name inferred from the enclosing manager). `0x03EF` was
in that category and is now **high**: its name was read directly off `GetTypeName`, and it is
`PushRequestNotifyRingBell`, not the session-disconnect push it was previously guessed to be. **Opcode certain / name unknown** for `0x0387`, `0x0388`, `0x038A`, `0x0389`, `0x038B`, `0x038C`, `0x0390`, and the three push blocks.

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

### 5.2 The three large push *alias blocks* — **SOLVED**

> **This section was rewritten 2026-08-05.** The earlier text said the alias→message mapping
> "could not be determined statically". That was wrong; it is fully determinable, and the
> reason the earlier pass missed it is recorded at the end of this section so nobody repeats it.
>
> **Addresses are given for both builds.** v1.00 = `PS3_GAME/USRDIR/EBOOT.elf` (30,756,464 B,
> net TOC `0x1D2EFE0`); v1.10 = `dev_hdd0/game/BLUS41045/USRDIR/EBOOT.elf` (31,301,872 B,
> net TOC `0x1DB3530`). **The two builds are byte-identical in logic here** — same masks, same
> jump table, same class names, only relocated. Live clients run v1.10.

#### 5.2.1 The mechanism

Each of the three managers is instantiated **N times**, once per gameplay *mode*, and each
instance registers the **same set of message types** at a **different opcode quartet/triple**.
All registrations in a manager therefore legitimately share one callback (vtable `0x1C5F530`,
member fn OPD `0x1D0B4D0` → `0x1598A30`, v1.00) — the callback then separates the aliases itself
with a **bitmask on `opcode − block_base`**.

BreakIn instance ctor `0x1595EE0` (v1.00) takes the mode in `r5` and switches on it; the four
instances are constructed back-to-back at `0x15586D0`, `0x15586E8`, `0x1558700`, `0x1558718` with
`li r5,0/1/2/3`. Visitor (`0x157A510`) and QuickMatch are built the same way.

The receive-side demultiplexer, all CONFIRMED:

| Manager | callback | range guard | discriminator |
|---|---|---|---|
| BreakIn | `0x1598A30` (v1.00) / `0x1604D98` (v1.10) | `addi r0,r28,-953` + `cmplwi r0,14` (**note: 14, not 15**) | `sld r0,1,r0` then `and` vs `0x1111` / `0x2222` / `0x4444` |
| Visitor | `0x157C0C8` (v1.00) / `0x15E80B8` (v1.10) | `addi r0,r28,-969` + `cmplwi r0,8` | `and` vs `0x049` / `0x092` / `0x124` |
| QuickMatch | `0x1572B40` (v1.00) / `0x15E0E08` (v1.10) | `addi r4,r28,-992` + `cmplwi r4,7` | **8-entry jump table** at `0x1572C68` (v1.00) / `0x15E0F30` (v1.10), offsets `20,20,9c,9c,118,118,4bc,4bc` |

Each branch stack-constructs the protobuf message (vtable from TOC, then `bl 0x1655F68` =
`ParseFromArray`). The class is read off vtable **slot 2 = `GetTypeName()`**, which loads a
`const char*` from the TOC — the same technique that identified `0x0389`.

#### 5.2.2 The mapping — BreakIn `0x03B9`–`0x03C8` (CONFIRMED)

**`opcode = 0x03B9 + 4×mode + role`**, `mode ∈ {0,1,2,3}`, `role ∈ {0=Target, 1=Reject, 2=Allow, 3=Remove}`.

| Role | Opcodes | Message class | Evidence (v1.00 / v1.10) |
|---|---|---|---|
| 0 | `0x3B9` `0x3BD` `0x3C1` `0x3C5` | `Frpg2RequestMessage.PushRequestBreakInTarget` | mask `0x1111` @ `0x1598B40`/`0x1604EA8`; vt `0x1C60C88`/`0x1CE2920`; `GetTypeName` `0x15FFDE4`/`0x1646DE0`; str `0x1897558`/`0x190CFF8` |
| 1 | `0x3BA` `0x3BE` `0x3C2` `0x3C6` | `Frpg2RequestMessage.PushRequestRejectBreakInTarget` | mask `0x2222` @ `0x1598BCC`/`0x1604F34`; vt `0x1C60B88`/`0x1CE2820`; `GetTypeName` `0x15FFCE4`/`0x1646CE0`; str `0x1897480`/`0x190CF20` |
| 2 | `0x3BB` `0x3BF` `0x3C3` `0x3C7` | `Frpg2RequestMessage.PushRequestAllowBreakInTarget` | mask `0x4444` @ `0x1598BDC`/`0x1604F44`; vt `0x1C60C48`/`0x1CE28E0`; `GetTypeName` `0x15FFDA4`/`0x1646DA0`; str `0x1897520`/`0x190CFC0` |
| 3 | `0x3BC` `0x3C0` `0x3C4` `0x3C8` | **none — dead** | see below |

`0x3B9` matches the live capture (a real invader's target replied to a `0x3B9` push), which is an
independent end-to-end check of the whole table.

**Role 3 is registered but has no handler.** There is no `0x8888` mask, and `0x3C8`
(offset 15) is additionally rejected by `cmplwi r0,14`. All four fall straight through to the
callback's cleanup/return path. `Frpg2RequestMessage.PushRequestRemoveBreakInTarget` *is* linked
into the client (`GetTypeName` `0x15FFEC0` v1.00 / `0x1646EBC` v1.10, vtable `0x1C60D48`/`0x1CE29E0`),
but the only four code sites that load its vtable are protobuf boilerplate (`New`/default-instance,
e.g. `0x15D3C10`, `0x15EA78C`), none inside the BreakIn manager. **A `RemoveBreakInTarget` push
sent on any of the 16 registered BreakIn opcodes will be silently discarded.** CONFIRMED.

**Independent send-side cross-check of role 2.** The invader's client relays its "allow" through
`0x0320 RequestSendMessageToPlayers`. That relay builds a `PushRequestAllowBreakInTarget`
(vtable load at `0x1597898`, same TOC slot as role 2 above), writes `this->mode` (`+0x30`) into the
message with has-bit `|= 8`, then picks the relay opcode by mode:
`li r0,0x3BB` (mode 0, `0x1597908`/`0x1597E90`), `li r0,0x3BF` (mode 1, `0x1597ADC`),
`li r0,0x3C3` (mode 2, `0x1597E74`), `li r0,0x3C7` (mode 3, `0x1597F80`) — exactly the `0x4444` set.
Two fully independent derivations agree.

Because the mode is a serialised field, **the invasion type is readable off the wire** from the
relayed Allow message (it is the 4th *declared* field — has-bit index 3 — of
`PushRequestAllowBreakInTarget`; field *number* is not recoverable statically). INFERRED that
mode is the DS2 invasion/covenant type; the four modes are not otherwise named in the binary.
Mode 0 (`0x3B9`–`0x3BC`) is the quartet exercised by ordinary invasions in live testing.

#### 5.2.3 The mapping — Visitor `0x03C9`–`0x03D1` (CONFIRMED)

**`opcode = 0x03C9 + 3×mode + role`**, `mode ∈ {0,1,2}`, `role ∈ {0=Visit, 1=RejectVisit, 2=RemoveVisitor}`.
All nine are live — masks `0x049 | 0x092 | 0x124 = 0x1FF` covers every bit, so there are **no dead
Visitor aliases**.

| Role | Opcodes | Message class | Evidence (v1.00 / v1.10) |
|---|---|---|---|
| 0 | `0x3C9` `0x3CC` `0x3CF` | `Frpg2RequestMessage.PushRequestVisit` | mask `0x049` @ `0x157C1E4`/`0x15E81D4`; vt `0x1C609C8`/`0x1CE2660`; `GetTypeName` `0x15FFB24`/`0x1646B20`; str `0x1897348`/`0x190CDE8` |
| 1 | `0x3CA` `0x3CD` `0x3D0` | `Frpg2RequestMessage.PushRequestRejectVisit` | mask `0x092` @ `0x157C62C`/`0x15E861C`; vt `0x1C60908`/`0x1CE25A0`; `GetTypeName` `0x15FFA64`/`0x1646A60`; str `0x18972C0`/`0x190CD60` |
| 2 | `0x3CB` `0x3CE` `0x3D1` | `Frpg2RequestMessage.PushRequestRemoveVisitor` | mask `0x124` @ `0x157C63C`/`0x15E862C`; vt `0x1C60A88`/`0x1CE2720`; `GetTypeName` `0x15FFBE4`/`0x1646BE0`; str `0x18973C8`/`0x190CE68` |

#### 5.2.4 The mapping — QuickMatch `0x03E0`–`0x03E7` (CONFIRMED)

> **CORRECTED 2026-08-05 (second pass).** This section previously said
> `opcode = 0x03E0 + 2×role + mode`. **The venue parity is inverted**: mode 0 owns the ODD
> aliases and mode 1 the EVEN ones. The correction is forced by a live capture (below) and
> confirmed twice over in the binary. BreakIn (§5.2.2) and Visitor (§5.2.3) were re-checked
> against their constructors and are **unaffected** — only QuickMatch was wrong.

QuickMatch is **mode-minor** (not role-minor like the others) **and mode-inverted**:

**`opcode = 0x03E0 + 2×role + (1 − mode)`**, `mode ∈ {0,1}`.

The jump table sends indices `{0,1}`, `{2,3}`, `{4,5}`, `{6,7}` to four targets. All eight are live.

**Evidence for the inversion (all v1.10, `dev_hdd0/…/EBOOT.elf`):**

1. **Constructor `0x15DDEC0`** takes the mode in `r5`, stores it at `this+0x30`
   (`stw r26,48(r31)` @ `0x15DDF2C`) and branches:
   `cmpwi r26,0 ; beq 0x15DDF68` → registers **`0x3E1 0x3E3 0x3E5 0x3E7`** (odd);
   `cmpwi r26,1 ; beq 0x15DE204` → registers **`0x3E0 0x3E2 0x3E4 0x3E6`** (even).
   Register immediates at `0x15DDF9C`/`0x15DE040`/`0x15DE0DC`/`0x15DE178` and
   `0x15DE238`/`0x15DE2DC`/`0x15DE378`/`0x15DE414`.
2. **Send side, same field.** The relayed "allow" (§5.2.7) reads the *same* `this+0x30`
   (`lwz r0,48(r23)` @ `0x15DEAE0`) and picks `li r0,0x3E5` for mode 0 (`0x15DEAF0`) or
   `li r0,0x3E4` for mode 1 (`0x15DECAC`); any other value falls back to `0x3E5`.
3. **`this+0x30` is the wire `mode`.** The `RequestRegisterQuickMatch` builder reads the same
   slot (`lwz r9,48(r30)` @ `0x15DD190`) straight into the request sent at
   `li r4,0x3D9` (`0x15DD264`).
4. **Live capture.** A BLUS41045 v1.10 client duelling at Undead Purgatory — for which our
   server decoded `mode = QuickMatchGameMode_Brotherhood (1)` out of its own
   `RequestRegisterQuickMatch` — relayed a **client-built** `PushRequestAllowQuickMatch`
   with `push_message_id = 996 = 0x3E4`. `0x3E4` is the even (mode-1) allow alias.
   The old formula predicted `0x3E5`.

**Why the old formula still worked live.** Both manager instances are constructed
unconditionally, back to back, at `0x15C25E0` (`li r5,0`) and `0x15C25F4` (`li r5,1`), so every
client has **all eight** aliases registered at all times. Both aliases of a role reach the same
jump-table branch, and the venue is carried in the message body
(`PushRequestJoinQuickMatch.mode`, field 6) rather than in the opcode — so a Brotherhood join
pushed as `0x3E1` is accepted (it lands on the Blue instance, which reads the venue out of the
payload). Sending the venue-matched alias is the correct behaviour; the other parity is merely
tolerated. **This means push-id parity is not, on its own, a testable hypothesis in-game — only
the role is.**

| Role | Opcodes (**mode 1** / **mode 0**) | Message class | Evidence (v1.00 / v1.10) |
|---|---|---|---|
| 0 | `0x3E0` `0x3E1` | `Frpg2RequestMessage.PushRequestJoinQuickMatch` | jt→`0x1572C88`/`0x15E0F50`; vt `0x1C60588`/`0x1CE2220`; `GetTypeName` `0x15FF6E4`/`0x16466E0`; str `0x1896FF8`/`0x190CA98` |
| 1 | `0x3E2` `0x3E3` | `Frpg2RequestMessage.PushRequestRejectQuickMatch` | jt→`0x1572D04`/`0x15E0FCC`; vt `0x1C60508`/`0x1CE21A0`; `GetTypeName` `0x15FF664`/`0x1646660`; str `0x1896F98`/`0x190CA38` |
| 2 | `0x3E4` `0x3E5` | `Frpg2RequestMessage.PushRequestAllowQuickMatch` | jt→`0x1572D80`/`0x15E1048`; vt `0x1C60488`/`0x1CE2120`; `GetTypeName` `0x15FF5E4`/`0x16465E0`; str `0x1896F30`/`0x190C9D0` |
| 3 | `0x3E6` `0x3E7` | `Frpg2RequestMessage.PushRequestRemoveQuickMatch` | jt→`0x1573120`/`0x15E13EC`; vt `0x1C60448`/`0x1CE20E0`; `GetTypeName` `0x15FF5A4`/`0x16465A0`; str `0x1896F00`/`0x190C9A0` |

#### 5.2.5 Correction to the previously recorded group structure

The old "four groups of four in call-site order" is **real but semantically inverted**. Those
groups are **per-instance (mode) groups**, not per-message-type groups; each group contains one of
*each* message type. Reading them as "four aliases of one type, take the leader" is what produced
three wrong live-test candidates (`0x3BD`, `0x3C1`, `0x3C5` — all three are in fact
`PushRequestBreakInTarget` for modes 1/2/3, which is exactly why an invader ignored them).

Within a BreakIn mode group the *call-site* order is Target, Reject, **Remove, Allow** (e.g. mode 0
registers `0x3B9, 0x3BA, 0x3BC, 0x3BB`) — the last two are swapped relative to numeric order.
Numeric order is the authoritative one: `+0` Target, `+1` Reject, `+2` Allow, `+3` Remove.

#### 5.2.6 Why the earlier pass missed this

The earlier conclusion ("the distinguishing state is passed through the callback object") was a
half-truth that stopped one step early. All 16 registration nodes really are identical 16-byte
`{vtable, this, member-fn OPD, adjust}` records differing only in the map key — but the
*distinguishing state* is `this`, one of four manager instances, **and the callback re-derives the
role from the opcode it is handed**. The mapping was never in the registration sites; it was always
in the one shared callback. **Do not stop at the registration site — always follow the callback.**

The reference explicitly notes DS3's QuickMatch pushes have "7 further aliases", so the alias
mechanism is real and known — but the **DS2 alias values on PS3 do not match the DS3OS DS2 `.inc`**.

#### 5.2.7 What the client tunnels through `0x0320` — **exhaustive** (v1.10)

`RequestSendMessageToPlayers` (`0x0320`) is a client→server→client relay: the server forwards
`{1: repeated recipient player_id, 2: opaque body}` verbatim. **The client is the only thing that
ever originates one.** Every send in the image goes through one helper,
`0x15F4150(net_client, opcode, msg, ctx, &err)`, whose opcode arrives in `r4`. Scanning the whole
`.text` for `li r4,0x320` yields exactly **three** sites:

| Site | Enclosing manager | Relayed body | Notes |
|---|---|---|---|
| `0x15DF124` (call `0x15DF134`) | QuickMatch (arena) | `Frpg2RequestMessage.PushRequestAllowQuickMatch` | vtable `0x1CE2120`; id `0x3E5`/`0x3E4` by mode |
| `0x16040FC` (call `0x160410C`) | BreakIn (invasion) | `Frpg2RequestMessage.PushRequestAllowBreakInTarget` | id `0x3BB`/`0x3BF`/`0x3C3`/`0x3C7` by mode (§5.2.2) |
| `0x15F516C` | the send library itself | — | not a send: registers the **receive** handler for `0x0320` |

Everything else that decodes as `0x320` in the net region is a stack displacement for a `lvx`
(`0x1571574`, `0x1575CE8`, `0x15782E0`, `0x157C524`, `0x157F734`, `0x157F868`, `0x1582B98`,
`0x159233C`, `0x15923F8`, `0x15926C4`), not an opcode.

**Consequence for a server: the two "allow" pushes must never be built server-side.** Neither
`PushRequestAllowQuickMatch` nor `PushRequestAllowBreakInTarget` originates on the server; both
arrive as a relay and are forwarded untouched. Visitor/co-op (`0x03C9`–`0x03D1`), MirrorKnight and
every other push block have **no** relay path — they are server-originated only.

The QuickMatch relay builder is the virtual at manager vtable `+0x24`
(`0x1CE0E3C` → OPD `0x1D8F568` → `0x15DE958`), signature
`f(this, ctx, online_area_id, cell_id, recipient_player_id, blob*)`; it fills
`push_message_id / player_id / online_area_id / cell_id / field_5` (has-bits `1|2|4|8|16`) and
**returns without sending if the blob is empty** (`0x15DE9E4`: `lwz r9,4(r28)` / `lwz r0,8(r28)`,
begin==end → early out). Carrying that blob is the entire purpose of the message.

#### 5.2.8 `PushRequestAllowQuickMatch.field_5` — a NexusRevolution2 matching packet

`field_5` (and `PushRequestAllowBreakInTarget.player_struct`, field 3) is an opaque byte string
lifted straight out of a `{begin,end}` buffer. Its first four bytes are the ASCII magic **`NXRV`**,
which occurs exactly once in the whole EBOOT, at `0x18C3C30` — inside a run of UTF-16BE Japanese
debug strings belonging to
`n:\Stable_10_0_FRPG2_Title\ApplicationLibrary\NexusRevolution2\source\matching\Protocol.cpp`
(`0x18C3ED0`). **`NXRV` = NexusRevolution**, FromSoftware's PS3 P2P matching library.

The serialiser's own debug strings name the wire order (all CONFIRMED strings, addresses v1.10):
signature `%c%c%c%c` (`0x18C3CF0`), version number `%hu` (`0x18C3D18`), "protocol ??" `%u`
(`0x18C3CB8`), player list — count `%u` and `プレイヤ<%s>` (`0x18C4258`, `0x18C41E0`), property list
— count `%u` and `ID=%u, Type=%u, Ope=%u` (`0x18C4330`, `0x18C4380`), phase number
(`0x18C4190`), session host, migration info. Its packet vocabulary is
`参加要求 / 参加ＯＫ / 参加ＮＧ` (join request / OK / NG), `セッション情報`, `セッションプロパティ`,
`ゲーム開始成功`, `調停…` (arbitration), `セッション表示/非表示`.

Five captured arena-duel bodies (players 100000 ↔ 100005, Undead Purgatory, `online_area_id`
10230000, `cell_id` 102350) give a 59-byte and a 65-byte `field_5`, differing only by the length
of an 8- vs 11-character PSN id:

```
00: 4e 58 52 56 | 00 64 | 00 | 00 6d 00 67 00 6e 00 6f 00 6d 00 61 00 64 00 32 | 00 00
    N  X  R  V    ver=100  ?   UTF-16BE  "m g n o m a d 2"                        NUL
19: 6d 67 6e 6f 6d 61 64 32 00 00 00 00 00 00 00 00 00      <- 17-byte ASCII buffer
    "mgnomad2" + NULs                                          (SceNpOnlineId: 16 + NUL)
2a: 10 | 00 00 00 00 00 00 00 00  00 00 00 00 00 00 00 0b    <- 17-byte tail
```

| Bytes | Field | Status |
|---|---|---|
| `0..3` | `"NXRV"` signature | **CONFIRMED** (magic at `0x18C3C30`; writer prints it `%c%c%c%c`) |
| `4..5` | `u16` BE version = **100** | **CONFIRMED** (writer prints version `%hu`; constant across all 5) |
| `6` | `u8` = 0 — the "protocol ??" / packet-type byte | INFERRED |
| `7 …` | UTF-16**BE** PSN online id, NUL-terminated (`2n+2` bytes) | INFERRED — BE is forced by the machine and by the 2-byte `00 00` terminator landing exactly before the ASCII copy; a UTF-16LE read starting at `8` also fits the bytes but leaves a 1-byte terminator |
| next `17` | ASCII PSN online id, `char[16] + NUL` (`SceNpOnlineId`) | **CONFIRMED from bytes** — 8+9 and 11+6 padding both total 17 |
| next `1` | `0x10` (16), constant in all five captures | unidentified |
| next `16` | zero except the last byte: `0b 0c 0d 0e 0f` across the five captures in time order, **across both consoles** | INFERRED a session id (`SessionId=%d` appears throughout the library's logging); the lockstep across two consoles fits one session object created per duel on each peer |

The sender writes **its own** identity: captures from 100005 carry `mgnomad2`, captures from
100000 carry `comradesean`. So the body is the P2P join handshake — "accept, and here is who and
which session to connect to".

**There is no map / level / statue id in this body**, and no room for one: every byte is
accounted for except the constant `0x10`. Note that NexusRevolution2 has its own
**`セッションプロパティ` (session property) packets**, sent and received *peer-to-peer* once the
session exists (`0x18C4010` send, `0x18C45D0` receive, with `ID/Type/Ope/Data` records) — that
channel never touches the game server. Anything negotiated after the join, arena map included,
travels there.

**Gap now closed (2026-08-05).** Three `0x0320` captures from the SAME sender across THREE
different statues at Undead Purgatory differ at exactly one byte offset — the trailing session
counter — and are byte-identical everywhere else. Combined with the five byte-identical
`RequestRegisterQuickMatch` payloads, **no part of the statue or map selection crosses the network
in either direction.** It is settled peer-to-peer through NexusRevolution2 session-property
packets, which never touch this server.

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

3. **`0x03FA`, `0x03FB`, `0x03FC`, `0x03FD`, `0x03FE` appear nowhere as opcodes — *in v1.00*.** No `li r4` with those values exists anywhere in `.text`. The maximum `r4` constant across all 132 send/register call sites is `0x03F9`.

   > **VERSION CAVEAT (added 2026-08-05).** This whole document was written against the **v1.00**
   > EBOOT. **`0x03FA` does exist in the v1.10 title update** — `li r4,0x03FA` occurs twice there,
   > zero times in v1.00 — and live v1.10 clients send it at boot with a 29-byte
   > `{1: MatchingParameter}` payload (i.e. `RequestGetRightMatchingArea`). The scan method is
   > sound; it was applied to the wrong build. Re-verify any negative result against
   > `dev_hdd0/game/BLUS41045/USRDIR/EBOOT.elf` before relying on it. Note that
   > `Frpg2RequestMessage.RequestGetRightMatchingArea` is **not** in the v1.00 `GetTypeName`
   > string run (212 distinct `Frpg2RequestMessage.*` names, none containing "Matching" except
   > `MatchingParameter`), which is consistent with the message being new in v1.10.

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
- ~~**Which alias in each push block corresponds to which push message type**~~ — **RESOLVED 2026-08-05, see §5.2.** Static analysis *can* separate them; the mapping lives in the shared push callback, not in the registration sites.
- **Whether the push transport opcode is `0x0320` with `msg_index = 0xFFFFFFFF`, as the reference claims.** The PS3 dispatcher (`0x158C138`) keys on a u32 that the *caller* has already deposited at `148(r1)`; I could not determine statically whether that u32 came from the transport header or from a parsed protobuf field. Both models are consistent with what I see. **This is an open question and I am explicitly not resolving it.**
- **Whether the "M" opcodes still require a `Reply` frame on the wire.**
- **The 12-byte message header layout** — not derivable from this pass (§6.6). The existing byte-confirmed facts remain the only source.

---

## 9. Suggested next steps, in value order

1. ~~Capture which of `0x03B9`–`0x03C8` the server must use~~ — **done statically, §5.2.** Use
   `0x03BA` for `PushRequestRejectBreakInTarget` on a mode-0 invasion (`0x03BE`/`0x03C2`/`0x03C6`
   for modes 1/2/3). Do **not** send a `RemoveBreakInTarget` push — the client has no handler.
2. ~~Same for a visit and an arena match~~ — **done statically, §5.2.**
3. Log what the client sends for `0x0387`/`0x0388`/`0x038A`/`0x0390` during boot — they fire early (the `0x0390` path is the `NRLoggingMessage` uploader, so it may be an opt-in telemetry channel you can safely stub).
4. **Do not implement `0x03FB`, `0x03FC`, `0x03FD`, `0x03FF`, `0x0400`.** No code for them in
   either build. **`0x03FA` is the exception — it is absent in v1.00 but present and actively sent
   in v1.10** (`RequestGetRightMatchingArea`); implement it, since the live clients are v1.10.
5. Re-run every negative result in §6 against the **v1.10** EBOOT
   (`dev_hdd0/game/BLUS41045/USRDIR/EBOOT.elf`, net TOC `0x1DB3530`). This document was written
   against v1.00; the two builds differ.

---

## 10. `RequestQueryLoginServerInfoResponse` — PS3 wire format (decompiled)

**This is a hard contradiction of the PC/SOTFS proto and it breaks LAN play if you follow the PC map.**

The PC/DS3OS proto (`Shared_Frpg2RequestMessage.proto:29`) declares:

```
required int64  port      = 1;
required string server_ip = 2;
```

**On BLUS41045 the message has three 32-bit VARINT fields and no string field at all.**

### 10.1 Evidence chain

**Class identification.** TOC-B slot `0x1D2C464` (`r2-11132`) holds `0x01C62DC8`, which is the
`RequestQueryLoginServerInfoResponse` vtable (`GetTypeName` at vtable+8 → `0x1601F24`). That vptr is
stored into the stack object at `r1+112` in function `0x166A59C` (`stw r0,112(r1)` @ `0x166ABD4`),
which is then parsed by `bl 0x1655F68` @ `0x166AC0C`.

**Parser.** vtable+32 = `0x15E0370` = `MergePartialFromCodedStream`. Its OPD descriptor
(`0x1D0E0F8`) is referenced by **exactly one** vtable slot (`0x1C62DE8`), so there is no
identical-code-folding ambiguity — this parser belongs solely to this class.

```
 15e03f0:	54 80 e8 fe 	srwi    r0,r4,3         ; field number = tag >> 3
 15e03f4:	2f 80 00 02 	cmpwi   cr7,r0,2
 15e03f8:	41 9e 00 9c 	beq     cr7,0x15e0494   ; -> field 2
 15e03fc:	2f 80 00 03 	cmpwi   cr7,r0,3
 15e0400:	41 9e 01 94 	beq     cr7,0x15e0594   ; -> field 3
 15e0404:	2f 80 00 01 	cmpwi   cr7,r0,1
 15e0408:	54 80 07 7e 	clrlwi  r0,r4,29        ; wire type = tag & 7
 15e040c:	41 9e 01 9c 	beq     cr7,0x15e05a8   ; -> field 1
 15e0410:	2f 80 00 04 	cmpwi   cr7,r0,4        ; WIRETYPE_END_GROUP -> done
 ...
 15e0420:	48 07 64 b9 	bl      0x16568d8       ; SkipField (everything else)
```

Every one of the three field arms begins with the same wire-type gate:

```
 15e0494:	54 80 07 7e 	clrlwi  r0,r4,29        ; field 2
 15e0498:	2f 80 00 00 	cmpwi   cr7,r0,0        ; must be WIRETYPE_VARINT
 15e049c:	40 9e ff 74 	bne     cr7,0x15e0410   ; else -> SkipField
```

(identically at `0x15E0594` for field 3 and `0x15E05A8` for field 1).

Storage is a **4-byte** `stw` into the message object, and the multi-byte path calls
`ReadVarint32Fallback` (`0x1650E9C`), not the 64-bit variant — so these are **32-bit** varint
fields, not `int64`:

| Field | Member offset | Store | Has-bit |
|---|---|---|---|
| 1 | `msg+24` (`stw r0,0(r26)` @ `0x15E05DC`) | `stw` | `ori r0,r0,1` @ `0x15E05F0` |
| 2 | `msg+28` (`stw r0,0(r28)` @ `0x15E04C8`) | `stw` | `ori r0,r0,2` @ `0x15E04DC` |
| 3 | `msg+32` (`stw r0,0(r27)` @ `0x15E0534`) | `stw` | `ori r0,r0,4` @ `0x15E0548` |

**Independent confirmation from the codegen's optimistic tag chain.** After parsing field 1 the
generated code hardcodes the tag byte it expects next; after field 2 likewise:

```
 15e0610:	2f 80 00 10 	cmpwi   cr7,r0,16       ; 0x10 = field 2, WIRETYPE_VARINT
 15e04fc:	2f 80 00 18 	cmpwi   cr7,r0,24       ; 0x18 = field 3, WIRETYPE_VARINT
```

If field 2 were a string, protoc would have emitted `0x12` here. It emits `0x10`.

### 10.2 Consumer — which field is which, and byte order

Immediately after a successful parse, function `0x166A59C`:

```
 166ac9c:	81 21 00 88 	lwz     r9,136(r1)     ; msg+24  = field 1
 166aca4:	80 01 00 8c 	lwz     r0,140(r1)     ; msg+28  = field 2
 166aca8:	b1 3d 00 e4 	sth     r9,228(r29)    ; -> ctx+0xE4, truncated to u16
 166acac:	90 1d 00 e0 	stw     r0,224(r29)    ; -> ctx+0xE0, full u32
```

The login task (`0x166DC88`) then dials the auth server:

```
 166eb90:	80 9f 00 e0 	lwz     r4,224(r31)    ; r4 = address (u32)
 166eb94:	a0 bf 00 e4 	lhz     r5,228(r31)    ; r5 = port (u16)
 166eb98:	4b ff 1a 11 	bl      0x16605a8      ; Connect(conn, addr, port)
```

and `MakeSockAddrIn` at `0x17C05E0` writes both **verbatim, with no byte swap**:

```
 17c05e0:	39 20 00 00 	li      r9,0
 17c05e4:	38 00 00 02 	li      r0,2
 17c05e8:	91 23 00 00 	stw     r9,0(r3)
 17c05ec:	91 23 00 0c 	stw     r9,12(r3)
 17c05f0:	90 83 00 04 	stw     r4,4(r3)       ; sin_addr  <- r4, no htonl
 17c05f4:	98 03 00 01 	stb     r0,1(r3)       ; sin_family = AF_INET (BSD sockaddr_in)
 17c05f8:	b0 a3 00 02 	sth     r5,2(r3)       ; sin_port  <- r5, no htons
 17c05fc:	91 23 00 08 	stw     r9,8(r3)
```

There is no `htonl`/`htons` anywhere on this path, and the binary contains **zero** byte-reverse
instructions (`stwbrx`/`lwbrx`/`sthbrx`/`lhbrx`) in the whole of `.text` (§6.6). Because PPC is
big-endian and `sin_addr.s_addr` is network byte order, the numeric value must therefore be
`(a<<24) | (b<<16) | (c<<8) | d`.

### 10.3 The answer

| Field | PC/SOTFS proto | **BLUS41045 (actual)** |
|---|---|---|
| 1 | `required int64 port` | `port` — **VARINT (wiretype 0), 32-bit**, client truncates to `u16` and uses it as `sin_port` verbatim (send plain host integer, e.g. `50000`) |
| 2 | `required string server_ip` | **`server_ip` — VARINT (wiretype 0), 32-bit binary IP**, written straight into `sin_addr.s_addr`. `192.168.1.100` → **`0xC0A80164` = `3232235876`** |
| 3 | *(absent)* | present, VARINT 32-bit, parsed into `msg+32`, **not consumed** by the login task — safe to omit |

**Why the observed bug happens:** sending field 2 as a length-delimited string (tag `0x12`) fails the
`wiretype == 0` gate at `0x15E0498`, falls through to `SkipField` (`0x16568D8`), and the member is
left at its default `0` → `0.0.0.0`. Field 1 is unaffected, which is exactly why the port worked and
the address didn't.

**Correct payload** for port 50000 / 192.168.1.100 (fields ascending, which also takes the parser's
fast path):

```
08 d0 86 03  10 e4 82 a0 85 0c
^^ f1 varint 50000
             ^^ f2 varint 3232235876 (0xC0A80164)
```

Emit field 2 as an **unsigned** 32-bit varint (5 bytes). A signed `int32` encoding of the same value
would be 10 bytes; `ReadVarint32Fallback` truncates either to 32 bits so both parse, but unsigned is
what the schema implies and is smaller.

**Confidence: high.** Sole-owner parser (no code folding), explicit wire-type gates, hardcoded
expected tag `0x10`, a matching 4-byte/2-byte consumer split, and a swap-free `MakeSockAddrIn`.
The one thing not proven exhaustively is that `ctx+0xE0`/`ctx+0xE4` have no other writer; I found
only `0x166ACAC`/`0x166ACA8` within the login module.

Note this is the **same convention as the 56-byte `Frpg2GameServerInfo`** (binary `u32` IP at offset
8, `u16` port at 12, no swap) — the PS3 client is consistent: **it never parses an IP as a string.**
