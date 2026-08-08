> **CORRECTION 2026-08-08 — `0x03B9` is NOT `PushRequestRemoveVisitor`.** Six live pushes captured
> from FromSoftware's own server carry
> `{push_id 953, player_id, psn_id, type 0, online_area_id, cell_id}` — six fields, exactly
> `PushRequestBreakInTarget`. `PushRequestRemoveVisitor` has four fields and no area or cell.
> The matching allow-relay is `0x03BB` (955), which is `0x03B9 + 2` and fits the BreakIn alias block
> `base + 4*mode + role` with role 2 = Allow. So the PC build uses the same `0x03B9` block for
> break-ins that PS3 does, and additionally sends `0x03FB` for type 4. See
> `tasks/live-capture-corpus.md`.

# DS2 protocol map

> **VERIFIED AGAINST LIVE PC TRAFFIC (2026-08-07).** Nine sessions against FromSoftware's own
> servers were decrypted with keys pulled from the running client and filed by opcode
> (`tasks/live-capture-corpus.md`). All 28 opcodes observed land exactly where this table places
> them, so DS2 SOTFS on PC shares the numbering recovered from the PS3 binary. Specifically
> confirmed on the wire: `0x0386`, `0x0392`, `0x0397`, `0x03A8`, `0x03AB`, `0x03AD`, `0x03AE`,
> `0x03B0`, `0x03B1`, `0x03B2`, `0x03B3`, `0x03B6`, `0x03B8`, `0x03D5`, `0x03D7`, `0x03EA`,
> `0x03EB`, `0x03EC`, `0x03ED`, `0x03F1`, `0x03F6`, `0x03FA`, `0x03FF`, `0x0400`, plus pushes
> `0x038C`, `0x03AA`, `0x03CC` and `0x03EF`.
>
> **Pushes ride wrapper opcode `0x0320`** with `msg_index = 0xFFFFFFFF`; the real push id is
> protobuf field 1. Filing by message opcode alone hides every push in one bucket.
>
> **Responses carry opcode `0`** and are matched to requests by `msg_index`.

# Frpg2 Client ⇄ Server Protocol Reference (Dark Souls 2 / Dark Souls 3)

> **Provenance.** Derived from the **DS3OS** C++ reference implementation checked out at `ref/ds3so`,
> which is **gitignored and not part of this repository** — you must obtain it separately for the
> citations below to resolve. Every `file:line` citation in this document points into that tree
> (root: `ref/ds3so/ds3os/`), except where a path begins with `/mnt/f/ClaudeHole/dso/`, which refers
> to our own Go implementation. Produced **2026-08-05**.

**Scope:** every message a game client exchanges with the *server*. Peer-to-peer (client⇄client) traffic used once a summon/invasion/visit is established is deliberately excluded, but the server-side brokering exchanges that set those links up are included in full.

All `file:line` citations are relative to the DS3OS checkout root **`/mnt/f/ClaudeHole/dso/ref/ds3so/ds3os/`** unless the path starts with `/mnt/f/ClaudeHole/dso/` (our own Go tree). Everything stated as fact was read out of the reference; anything inferred is marked **(inferred)**. Note throughout that DS3OS targets **PC**: Dark Souls 2 there means **Scholar of the First Sin (Steam 335300)**, while our M1 target is **BLUS41045, original DS2 on PS3** — deltas are flagged inline and collected in the caveats section.

---

## 0. Orientation

A client goes through three network phases. First it opens a **TCP** connection to the *login service* and asks a single question — "where do I authenticate?" — under RSA. It then opens a second **TCP** connection to the *auth service* named in that answer and runs a four-stage handshake that negotiates an AES-CWC-128 session key, checks the app version, validates a platform ticket, and returns a raw 184-byte struct containing the game server's address and an 8-byte auth token. Finally it opens a **reliable-UDP** session to the *game service*, prefixing every datagram with that auth token in the clear so a connectionless listener can demux it, and from there speaks a protobuf request/response protocol with ~90 opcodes (DS2) or ~95 (DS3), grouped by feature area. Almost every game-service message is a client-initiated request that gets exactly one reply; a small set are server-initiated **pushes**, all of which share transport opcode `0x0320` and are disambiguated by the first protobuf field. Matchmaking (signs, invasions, visits, arena) is entirely brokered through this request/push machinery — the server introduces two clients to each other and then steps out of the way.

---

## 1. Transport primer (needed to read the rest)

### 1.1 Login/auth message framing (TCP)

`Frpg2Message` — `Source/Server/Server/Streams/Frpg2Message.h:32`:

| Field | Size | Encoding |
|---|---|---|
| `header_size` | 4 | big-endian, always `12` |
| `msg_type` | 4 | big-endian, `Frpg2MessageType` |
| `msg_index` | 4 | **little-endian** (`Frpg2Message.h:44`) |
| response header | 16 | present **only when `msg_type == Reply`**; big-endian `{0, 1, 0, 0}` (`Frpg2Message.h:48`) |
| payload | — | encrypted body |

`Frpg2MessageType` (`Frpg2Message.h:17`):

| Value | Name |
|---|---|
| `0x0` | `Reply` |
| `1` | `KeyMaterial` |
| `2` | `GetServiceStatus` |
| `3` | `SteamTicket` |
| `5` | `RequestQueryLoginServerInfo` |
| `6` | `RequestHandshake` |

A reply carries `msg_type = Reply` and copies the request's `msg_index`; the *type* of the reply is recovered by matching the index against the outstanding request. Our Go implementation mirrors this exactly at `/mnt/f/ClaudeHole/dso/internal/frpg/message/message.go:21` and `:109`.

### 1.2 Login/auth crypto

`Source/Server/Server/Streams/Frpg2MessageStream.cpp:23-36`. Server-side: decrypt inbound with **RSA / PKCS#1 OAEP**, encrypt outbound with **RSA / X9.31** (the pairing is swapped for the client role). After `RequestHandshake` the stream drops the cipher entirely for one 27-byte plaintext blob (`AuthClient.cpp:104-117`), then installs `CWCCipher` in both directions using the client-supplied key (`AuthClient.cpp:120`).

Confirmed on PS3 (see `/mnt/f/ClaudeHole/dso/docs/ps3-vs-pc.md`): the CWC tag algorithm, the AES-CTR keystream, the `IV(11) || tag(16) || ciphertext` TCP framing, the 27-byte handshake blob (11 random + 16 zero), and the login RSA public key are all **byte-identical** between PC/SOTFS and PS3/BLUS41045.

### 1.3 Game-service transport (UDP)

Every datagram is prefixed with the 8-byte auth token in the clear; the server peeks it to find the session's CWC key (`Source/Server/Server/GameService/GameService.cpp:182-200`). Above the cipher sits a reliable-UDP layer with these opcodes (`Source/Server/Server/Streams/Frpg2ReliableUdpPacket.h:20`):

`SYN 0x02`, `RACK 0x03` (unused), `DAT 0x04`, `HBT 0x05`, `FIN 0x06`, `RST 0x07`, `PT_DAT_FRAG 0x08` (unused), `ACK 0x31`, `SYN_ACK 0x32`, `DAT_ACK 0x34`, `FIN_ACK 0x36`, `PT_DAT_FRAG_ACK 0x38` (unused).

Above that, fragments reassemble into `Frpg2ReliableUdpMessage` with the same 12-byte header shape as the TCP layer (`Source/Server/Server/Streams/Frpg2ReliableUdpMessage.h:28`) — `header_size`, `msg_type` (BE), `msg_index` (LE), plus a 16-byte response header on replies.

### 1.4 The three message kinds

Defined by macros in the per-game `.inc` files (`Source/Server.DarkSouls2/Server/Streams/DS2_Frpg2ReliableUdpMessageTypes.inc:15-18`):

- **`DEFINE_REQUEST_RESPONSE(op, Type, Req, Resp)`** — client→server, server replies with `Resp` (`msg_type = Reply`, `msg_index` copied).
- **`DEFINE_MESSAGE(op, Type, Req)`** — client→server, **no reply at all**. Only two exist: DS2 `RequestCreateBloodstain` (0x0391) and the whole DS3 logging block.
- **`DEFINE_PUSH_MESSAGE(op, Type, Msg)`** — server→client, unsolicited. On the wire the header always carries `msg_type = 0x0320` and `msg_index = 0xFFFFFFFF` (`Source/Server/Server/Streams/Frpg2ReliableUdpMessageStream.cpp:38-43`). The opcode listed in the `.inc` for a push is **not** a transport opcode — it is the value of the push's protobuf field 1 (`PushMessageId`), which is how the client disambiguates. Confirmed by the enum at `Protobuf/DarkSouls2/DS2_Frpg2RequestMessage.proto:24` and the comment at `DS2_Frpg2ReliableUdpMessageTypes.inc:18`.

This resolves the apparent `0x0320` conflict: **client→server 0x0320 is `RequestSendMessageToPlayers`; server→client 0x0320 with index `0xFFFFFFFF` is a push.**

`ReliableUdpMessageType_Expects_Response` (`Source/Server.DarkSouls2/Server/DS2_Game.cpp:96`) is the authoritative "does this expect a reply" oracle; `ReliableUdpMessageType_To_Protobuf` (`DS2_Game.cpp:59`) maps opcode → protobuf class.

### 1.5 Dispatch model

`GameClient::HandleMessage` (`Source/Server/Server/GameService/GameClient.cpp:107`) walks the registered managers in order, calling `OnMessageReceived` on each until one returns `Handled`. DS2 manager registration order is `DS2_Game.cpp:133-145`: Boot, PlayerData, Ghost, BloodMessage, Bloodstain, BreakIn, Logging, Misc, Visitor, Ranking, MirrorKnight, Sign, QuickMatch.

---

## 2. Phase 1 — Login service (TCP)

**One message type only.** DS3OS listens on port 50050 (`Source/Server/Config/RuntimeConfig.h:198`); **PS3 DS2 uses 50011** (our `cf0d25c`, `docs/ps3-vs-pc.md`).

Whole service: `Source/Server/Server/LoginService/LoginClient.cpp:36-108`. Any message whose type is not `RequestQueryLoginServerInfo` causes an immediate disconnect (`LoginClient.cpp:68-72`).

### `RequestQueryLoginServerInfo` — type `5`
- **Direction:** client → server. **Expects reply:** yes.
- **Payload** (`Protobuf/Shared/Shared_Frpg2RequestMessage.proto:22`):
  ```
  required string steam_id   = 1;
  optional string f2         = 2;
  required uint64 app_version = 3;
  ```
- **Crypto:** RSA. Server decrypts with OAEP.
- **Handler:** `LoginClient.cpp:76-81`.
- **Shared** (DS2 + DS3, all platforms).
- **PS3 note:** `steam_id` carries the **PSN online ID** verbatim (observed: `comradesean`). The field name is Steam-flavoured but the field is generic.

### `RequestQueryLoginServerInfoResponse` — sent as type `0` (`Reply`)
- **Direction:** server → client. Reply to the above.
- **Payload** (`Shared_Frpg2RequestMessage.proto:29`): `required int64 port = 1; required string server_ip = 2;`
- **Handler:** `LoginClient.cpp:94-102`. Note the server substitutes its *private* IP when the peer is on a private subnet (`LoginClient.cpp:88-92`) — relevant to us, since we run on a LAN.
- Our implementation: `/mnt/f/ClaudeHole/dso/internal/server/login/login.go:92-110`.

### `RequestQueryLoginServerInfoForXboxOne`
- Declared at `Shared_Frpg2RequestMessage.proto:36` with **no reversed fields**. Never handled by DS3OS. Presumed to replace `RequestQueryLoginServerInfo` on Xbox. **(inferred)** Not relevant to PS3.

---

## 3. Phase 2 — Auth service (TCP)

Four stages, strictly sequential; any out-of-order type disconnects. State machine: `Source/Server/Server/AuthService/AuthClient.cpp:75-309`. DS3OS port 50000 (`RuntimeConfig.h:201`).

**Confirmed identical on PS3** (`docs/ps3-vs-pc.md`): the stage sequence `RequestHandshake → GetServiceStatus → KeyMaterial → ticket`.

### Stage 1 — `RequestHandshake` (type `6`)
- **Direction:** client → server. **Expects reply:** yes.
- **Payload** (`Shared_Frpg2RequestMessage.proto:46`): `required bytes aes_cwc_key = 1;` — the AES-CWC-128 key for the rest of *this TCP connection*.
- **Crypto:** RSA (OAEP inbound).
- **Reply:** **not a protobuf.** A raw 27-byte payload sent *with the cipher disabled*: 11 random bytes followed by 16 zero bytes (`AuthClient.cpp:106-117`). DS3OS's comment calls this mysterious; it is simply `IV length (11) + tag length (16)` — confirmed against a real PS3 client. `RequestHandshakeResponse` exists in the proto (`:51`) but is an empty placeholder and is not what goes on the wire.
- After sending it, both directions switch to `CWCCipher(aes_cwc_key)` (`AuthClient.cpp:120`).
- **Handler:** `AuthClient.cpp:78-127`.

### Stage 2 — `GetServiceStatus` (type `2`)
- **Direction:** client → server. **Expects reply:** yes.
- **Payload** (`Shared_Frpg2RequestMessage.proto:56`):
  ```
  required int64  id          = 1;
  required string steam_id    = 2;
  optional string unknown_1   = 3;
  required int64  app_version = 4;
  ```
- **Reply — `GetServiceStatusResponse`** (`:73`): `optional int64 id = 1; optional string steam_id = 2; optional int64 unknown_1 = 3; optional int64 app_version = 4;`. Server sets `id = 2`, `steam_id = "\0"`, `unknown_1 = 0`, and echoes `app_version` (`AuthClient.cpp:152-159`). **If the version is out of range it sends a completely empty response**, which is how the client is told to update (`AuthClient.cpp:160-163`).
- **Version gate:** `Source/Server/Config/BuildConfig.h:45-59` — DS2/SOTFS PC `MIN = MAX = 17039619`; DS3 `MIN=114, MAX=116`.
  **PS3 DS2 sends `16912640` (`0x1020000`)** — different, and the gate must be widened. Recorded in memory and `docs/ps3-vs-pc.md`.
- **Handler:** `AuthClient.cpp:130-176`. The `steam_id` from this message is stashed and used for ticket validation in stage 4.
- `GetServiceStatusForXboxOne` (`:64`) exists, un-reversed, unhandled.

### Stage 3 — `KeyMaterial` (type `1`)
- **Direction:** client → server. **Expects reply:** yes.
- **Payload: raw bytes, not protobuf.** Exactly **8 bytes**; any other size disconnects (`AuthClient.cpp:190-194`).
- **Reply: raw 16 bytes** — the client's 8 bytes in the low half, 8 server-random bytes in the high half (`AuthClient.cpp:197-203`). **This 16-byte value is the CWC key for the entire subsequent UDP game session.**
- **Handler:** `AuthClient.cpp:180-215`.

### Stage 4 — `SteamTicket` (type `3`)
- **Direction:** client → server. **Expects reply:** yes.
- **Payload: raw bytes.** Layout (`AuthClient.cpp:226-229`): bytes `0..15` = the 16-byte game CWC key echoed back; bytes `16..` = the platform ticket (`GetAuthSessionTicket` output on Steam).
- **Validation:** `SteamGameServer()->BeginAuthSession` (`AuthClient.cpp:249`), gated on `BuildConfig::AUTH_ENABLED`.
- **Reply: raw `Frpg2GameServerInfo` struct**, 184 bytes (`Source/Server/Server/AuthService/AuthClient.h:27-48`):

  | Offset | Field | Notes |
  |---|---|---|
  | 0 | `uint64 auth_token` | random; **not** byte-swapped |
  | 8 | `char game_server_ip[16]` | NUL-terminated ASCII |
  | 24 | `uint8 stack_data[112]` | zeroed by DS3OS — the retail server leaked stack here |
  | 136 | `uint16 game_port` | big-endian |
  | 138 | `uint16 padding` | 0 |
  | 140 | `uint32 unknown_1..11` | big-endian; `0x8000, 0x8000, 0xA000, 0xA000, 0x80, 0x8000, 0xA000, 0x493E0, 0x61A8, 0x0C, 0x00` — buffer sizes **(inferred)** |

  Our encoder: `/mnt/f/ClaudeHole/dso/internal/server/auth/gameserverinfo.go:28`.
- **Side effect:** the auth token is registered with the game service against the 16-byte CWC key (`AuthClient.cpp:292-293`), which is what makes the UDP session possible.
- **Handler:** `AuthClient.cpp:218-301`.
- **PS3 delta:** the ticket is a **PSN NP ticket**, not a Steam ticket. Format not yet captured — this stage has never been reached against a real PS3 client. The message *type number* is still 3 (the enum name is Steam-flavoured, the slot is generic) **(inferred, unverified)**.

After stage 4 the auth connection goes idle until the client drops it (`AuthClient.cpp:304-308`).

---

## 4. Phase 3 — Game service (reliable UDP)

DS3OS port 50010 (`RuntimeConfig.h:204`). Below, every entry gives: **opcode**, direction, reply, payload shape, and handler. Proto line numbers are `Protobuf/DarkSouls2/DS2_Frpg2RequestMessage.proto` (**DS2P**) and `Protobuf/DarkSouls3/DS3_Frpg2RequestMessage.proto` (**DS3P**). Both files are **proto2** with `optimize_for = LITE_RUNTIME` (DS2P:12-14) — `required` is genuinely enforced by the parser, so an omitted required field is a parse failure, not a default.

Empty responses come in three flavours in the reference and the distinction matters: `// Never received.` means the server's reply body is genuinely empty and the message is effectively fire-and-forget; `// TODO` means the message is **un-reversed** and real fields probably exist on the wire; no comment at all is ambiguous.

### 4.1 Boot

| Opcode | Name | Dir | Reply |
|---|---|---|---|
| `0x0386` | `RequestWaitForUserLogin` | C→S | `RequestWaitForUserLoginResponse` |
| `0x038C` | `PlayerInfoUploadConfigPushMessage` | push | — |
| `0x03EC` (DS2) / `0x03C6` (DS3) | `RequestGetAnnounceMessageList` | C→S | `...Response` |

`DS2_Frpg2ReliableUdpMessageTypes.inc:23-26`; `DS3_Frpg2ReliableUdpMessageTypes.inc:23-25`.

**`RequestWaitForUserLogin`** — the first game-service message; establishes identity.
- DS2P:58 — `required string steam_id = 1; required uint32 unknown_1..unknown_4 = 2..5; optional uint32 unknown_5 = 6; optional uint32 unknown_6 = 7;` (observed `1, 0, 1, 2`; one of these is probably the profile index).
- DS3P:54 — same first five fields, **no optional 6/7**. DS2-only extension.
- Reply `RequestWaitForUserLoginResponse` (DS2P:68 / DS3P:62): `required string steam_id = 1; required uint32 player_id = 2;` — the server-assigned player id used by everything else.
- **Handler:** DS2 `Source/Server.DarkSouls2/Server/GameService/GameManagers/Boot/DS2_BootManager.cpp:46`; DS3 `.../Boot/DS3_BootManager.cpp:45`.

**`PlayerInfoUploadConfigPushMessage`** — pushed **immediately after** the login response, unsolicited, in the same handler (`DS2_BootManager.cpp:106-122`).
- DS2P:116 — `required PushMessageId push_message_id = 1; required PlayerStatusUploadConfig config = 2; required uint32 player_character_update_send_delay = 3; required uint32 player_status_send_delay = 4;`
- `PlayerStatusUploadConfig` (DS2P:73): `repeated uint32 player_data_mask = 1; required uint32 upload_interval = 2;`
- **Biggest DS2/DS3 divergence in this area:** DS2 sends a flat decimal mask of ~95 ids — `0..4, 100..115, 200..206, 300..315, 400..408, 500..523, 600..605, 700..703, 800..805` (`DS2_BootManager.cpp:97-104`). DS3 sends a ~340-entry interleaved hex mask (`DS3_BootManager.cpp:96-141`). The encodings are not interchangeable.

**`RequestGetAnnounceMessageList`**
- DS2P:95 — `required uint32 max_entries = 1; optional uint32 unknown_1 = 2; optional uint32 unknown_2 = 3;` DS3P:89 has only `max_entries`.
- DS3OS asserts `max_entries == 10` for DS2 (`DS2_BootManager.cpp:136`) and `== 100` for DS3 (`DS3_BootManager.cpp:173`).
- Reply (DS2P:101): `required AnnounceMessageDataList changes = 1; required AnnounceMessageDataList notices = 2;` where `AnnounceMessageDataList` (DS2P:91) is `repeated AnnounceMessageData items = 1;` and `AnnounceMessageData` (DS2P:82) is `required uint32 unknown_1 = 1; required uint32 index = 2; required uint32 unknown_2 = 3; required string header = 4; required string message = 5; required DS2_Frpg2PlayerData.DateTime datetime = 6;`
- Both servers populate **`changes` only**; `notices` is created via `mutable_notices()` and left empty (`DS2_BootManager.cpp:139`).
- This is also the ban/warning delivery channel: a banned player gets a "BANNED" announcement plus `Client->Banned = true` and a 2-second delayed disconnect (`DS2_BootManager.cpp:145-154`).
- **Handler:** `DS2_BootManager.cpp:131` / `DS3_BootManager.cpp:168`.

### 4.2 Player data

| DS2 op | DS3 op | Name | Dir | Reply |
|---|---|---|---|---|
| `0x03B3` | `0x039F` | `RequestGetLoginPlayerCharacter` | C→S | `...Response` |
| `0x03B6` | `0x03A2` | `RequestUpdateLoginPlayerCharacter` | C→S | `...Response` |
| `0x03B8` | `0x03A4` | `RequestUpdatePlayerStatus` | C→S | `...Response` (empty) |
| `0x03A8` | `0x0394` | `RequestUpdatePlayerCharacter` | C→S | `...Response` (empty) |
| `0x03A9` | `0x0395` | `RequestGetPlayerCharacter` | C→S | `...Response` |
| — | `0x03A1` | `RequestGetPlayerCharacterList` | C→S | `...Response` — **DS3-only** |

`DS2_..._Types.inc:31-35`; `DS3_..._Types.inc:30-35`. Handlers: `Source/Server.DarkSouls2/Server/GameService/GameManagers/PlayerData/DS2_PlayerDataManager.cpp`, dispatch at `:32`.

- **`RequestUpdateLoginPlayerCharacter`** (DS2P:123) `required uint32 character_id = 1; repeated uint32 local_character_ids = 2;` → reply (DS2P:128) `required uint32 character_id = 1;`. If `character_id == 0` the server allocates the lowest id not in `local_character_ids` and not already in the DB (`DS2_PlayerDataManager.cpp:66-95`). Handler `:58`.
- **`RequestUpdatePlayerStatus`** (DS2P:132) `required bytes status = 1;` — the blob is a `DS2_Frpg2PlayerData.AllStatus` protobuf which the server parses and **merges** into cached state (`DS2_PlayerDataManager.cpp:125-135`). This is the periodic heartbeat driven by `PlayerInfoUploadConfigPushMessage`'s `upload_interval`, and it is the only source for soul memory / current area used by every matchmaking filter. Reply is empty (DS2P:136, `// Never received.`). Handler `:125`.
- **`RequestUpdatePlayerCharacter`** (DS2P:140) `required uint32 character_id = 1; required bytes character_data = 2;` — opaque save blob stored verbatim. Empty reply. Handler `:354`.
- **`RequestGetPlayerCharacter`** (DS2P:149) `required uint32 player_id = 1; required uint32 character_id = 2;` → `RequestGetPlayerCharacterResponse` (DS2P:154) `player_id, character_id, bytes character_data`. Handler `:380`.
  ⚠️ The DS2 `.inc` declares the response class as **`RequestGetPlayerCharacterList`** (`DS2_..._Types.inc:35`), not `RequestGetPlayerCharacterResponse`. The handler sends the latter. The two are **wire-identical** (DS2P:1260 is `player_id=1, charatcer_id=2 [sic], character_data=3`), so this is harmless — but a client emulator decoding replies from the `.inc` table will name it wrong.
- **`RequestGetLoginPlayerCharacter`** (DS2P:160) `required int64 player_id = 1;` → (DS2P:164) `int64 player_id, uint32 character_id, bytes character_data`. Note the `int64`/`uint32` inconsistency between the two "get character" pairs — that is in the proto as written. Handler `:408`.
- **`RequestGetPlayerCharacterList` (DS3-only, `0x03A1`)** — DS3P:1046/1050, both un-reversed stubs (repeated `PlayerCharacterID` / `PlayerCharacterData`, themselves fieldless at DS3P:102/108). Handler `DS3_PlayerDataManager.cpp:366` is `Ensure(false)` + empty reply; DS3OS has never seen the client send it.

### 4.3 Blood messages

| DS2 op | DS3 op | Name | Dir |
|---|---|---|---|
| `0x03AB` | `0x0397` | `RequestCreateBloodMessage` | C→S |
| `0x03AC` | `0x0398` | `RequestRemoveBloodMessage` | C→S |
| `0x03AD` | `0x0399` | `RequestReentryBloodMessage` | C→S |
| `0x03AE` | `0x039A` | `RequestGetBloodMessageList` | C→S |
| `0x03AF` | `0x039B` | `RequestEvaluateBloodMessage` | C→S |
| `0x03B0` | `0x039C` | `RequestGetBloodMessageEvaluation` | C→S |
| `0x03FF` | — | `RequestGetAreaBloodMessageList` | C→S | **DS2-only** |
| — | `0x03DA` | `RequestReCreateBloodMessageList` | C→S | **DS3-only** |
| `0x03AA` | `0x0396` | `PushRequestEvaluateBloodMessage` | push |

`DS2_..._Types.inc:42-50`; `DS3_..._Types.inc:40-49`. Handlers: `.../BloodMessage/DS2_BloodMessageManager.cpp`, dispatch `:77`.

- **`BloodMessageData`** (DS2P:174) — the record type: `player_id, character_id, message_id, good, bytes message_data, string player_steam_id, cell_id, optional string unknown_8`. DS3 splits this differently (`LocatedBloodMessage` DS3P:180, `BloodMessageDomainLimitData` DS3P:185).
- `RequestCreateBloodMessage` (DS2P:200) `online_area_id, cell_id, character_id, bytes message_data` → `message_id` (DS2P:207). Handler `:184`.
- `RequestRemoveBloodMessage` (DS2P:211) `online_area_id, cell_id, message_id` → empty (`// TODO`, so possibly un-reversed). Handler `:234`.
- `RequestReentryBloodMessage` (DS2P:191) same three fields → empty. This is how a client re-registers messages it already holds locally. Handler `:111`.
- `RequestGetBloodMessageList` (DS2P:221) `online_area_id, max_messages, repeated BloodMessageCellLimitData search_areas` where `BloodMessageCellLimitData` (DS2P:185) is `cell_id, max_type_1, max_type_2` → (DS2P:227) `online_area_id, repeated BloodMessageData messages`. Handler `:264`.
- `RequestGetAreaBloodMessageList` (DS2P:260, **DS2-only**) `online_area_id, count, max_type_1, max_type_2` — shares the `RequestGetBloodMessageListResponse` reply class (`DS2_..._Types.inc:50`); no dedicated response message exists. Handler `:316`.
- `RequestEvaluateBloodMessage` (DS2P:232) `online_area_id, cell_id, message_id` → empty. **Side effect:** if the message's author is online, the server pushes `PushRequestEvaluateBloodMessage` to them (`DS2_BloodMessageManager.cpp:404-418`). Evaluating your own message disconnects you (`:387-391`). Handler `:358`.
- `PushRequestEvaluateBloodMessage` (DS2P:253) `push_message_id, player_id, message_id, player_steam_id`. The `.inc` comments call it unregistered, but the DS2 manager does send it.
- `RequestGetBloodMessageEvaluation` (DS2P:242) `online_area_id, cell_id, message_id` → (DS2P:248) `int64 message_id, int64 rating`. Handler `:143`.
- **`RequestReCreateBloodMessageList` (DS3-only, `0x03DA`)** — DS3P:213, `character_id = 2` plus `repeated group Blood_message_info_list = 3 { online_area_id, bytes message_data, unknown_1, unknown_2 }` → `repeated uint32 message_ids` (DS3P:223). Follow-up to `RequestReentryBloodMessage` when the server asks for full bodies. Handler `DS3_BloodMessageManager.cpp:155`; NRSSR-validated.

### 4.4 Bloodstains

| DS2 op | DS3 op | Name | Dir | Reply |
|---|---|---|---|---|
| `0x0391` | `0x0391` | `RequestCreateBloodstain` | C→S | **none** (`DEFINE_MESSAGE`) |
| `0x0392` | `0x0392` | `RequestGetBloodstainList` | C→S | `...Response` |
| `0x0400` | — | `RequestGetAreaBloodstainList` | C→S | shares `RequestGetBloodstainListResponse` — **DS2-only** |

`DS2_..._Types.inc:55-57`. Handlers: `.../Bloodstain/DS2_BloodstainManager.cpp`, dispatch `:69`.

- `BloodstainInfo` (DS2P:271) `online_area_id, cell_id, bloodstain_id, bytes data`.
- `RequestCreateBloodstain` (DS2P:278) `online_area_id, cell_id, bytes data, bytes ghost_data` — **the only DS2 game-service message with no reply**. Confirmed: handler `:91` returns `Handled` at `:162` without ever calling `Send`. No `RequestCreateBloodstainResponse` exists in the proto.
- `RequestGetBloodstainList` (DS2P:285) `online_area_id, max_stains, repeated CellLimitData search_areas` (`CellLimitData` DS2P:1252 = `cell_id, max_items`) → (DS2P:298) `repeated BloodstainInfo bloodstains`. Handler `:165`.
- `RequestGetAreaBloodstainList` (DS2P:291) `online_area_id, count, max_type_1, max_type_2`. Handler `:212`.

### 4.5 Ghosts

| DS2 op | DS3 op | Name | Reply |
|---|---|---|---|
| `0x0393` | `0x0393` | `RequestGetDeadingGhost` | `...Response` |
| `0x03B1` | `0x039D` | `RequestCreateGhostData` | `...Response` (empty) |
| `0x03B2` | `0x039E` | `RequestGetGhostDataList` | `...Response` |

`DS2_..._Types.inc:78-80`. `RequestGetDeadingGhost` is handled by the **Bloodstain** manager, not the Ghost manager (`DS2_BloodstainManager.cpp:83`, handler `:250`) — the "deading ghost" is the replay attached to a bloodstain and is keyed by `bloodstain_id`.

- `RequestGetDeadingGhost` (DS2P:302) `online_area_id, cell_id, bloodstain_id` → (DS2P:308) same three + `bytes data`. If the stain isn't found the server still replies, with an empty `data` (`DS2_BloodstainManager.cpp:282-285`).
- `GhostData` (DS2P:775) `cell_id, ghost_id, bytes data`.
- `RequestCreateGhostData` (DS2P:792) `online_area_id, cell_id, bytes data` → empty. Handler `.../Ghosts/DS2_GhostManager.cpp:83`.
- `RequestGetGhostDataList` (DS2P:781) `online_area_id, max_ghosts, repeated CellLimitData search_areas` → `required uint32 online_area_id = 1; repeated GhostData ghosts = 2;` — **`ghosts` is field 2 on PS3.** This entry previously said field 3 and that "writing `ghosts = 2` breaks wire compatibility"; that is the PC/reference shape and is **exactly backwards for BLUS41045**. The client's parser (v1.10 `0x1685110`) tests only fields 1 and 2 and sends anything else to `SkipField`, and its serialiser (`0x1634668`) emits `li r3,2`. A list written as field 3 is discarded whole — which is why no ghost was ever seen in game despite 80 recorded and lists returning 8. Identical in v1.00. Handler `:153`.

### 4.6 Summon signs (co-op brokering)

| DS2 op | DS3 op | Name | Dir | Reply |
|---|---|---|---|---|
| `0x0394` | `0x0456` | `RequestCreateSign` | C→S | `...Response` (`sign_id`) |
| `0x0395` | `0x0457` | `RequestUpdateSign` | C→S | empty (keepalive) |
| `0x0396` | `0x0458` | `RequestRemoveSign` | C→S | empty |
| `0x0397` | `0x0459` | `RequestGetSignList` | C→S | `...Response` |
| `0x0398` | `0x045A` | `RequestSummonSign` | C→S | empty |
| `0x039A` | `0x045B` | `RequestRejectSign` | C→S | empty |
| `0x03FA` | `0x03D9` | `RequestGetRightMatchingArea` | C→S | `...Response` |
| `0x039B` | `0x033E` | `PushRequestSummonSign` | push | — |
| `0x039C` | `0x033F` | `PushRequestRejectSign` | push | — |
| `0x039D` | `0x033D` | `PushRequestRemoveSign` | push | — |

`DS2_..._Types.inc:62-73`; `DS3_..._Types.inc:60-70`. Handlers: `.../Signs/DS2_SignManager.cpp`, dispatch `:82`.

**Flow (the part of summoning the server brokers).** From the DS3 proto's own flow comment (`DS3P:528-547`), which applies structurally to DS2 too:

```
Host (sign placer)                       Server                       Summoner
  -- RequestCreateSign ------------------->
  <------------------ CreateSignResponse{sign_id}
  -- RequestUpdateSign (repeatedly, keepalive) ->
                                            <---- RequestGetSignList --
                                            ---- GetSignListResponse -->
                                            <---- RequestSummonSign ----
  <-- PushRequestSummonSign ---------------                (empty reply sent to summoner)
  [ if host declines: ]
  -- RequestRejectSign ------------------->
                                            -- PushRequestRejectSign -->
  [ sign consumed / host disconnects: ]
                     -- PushRequestRemoveSign --> (every player aware of the sign)
```

After `PushRequestSummonSign` the two clients establish the P2P link themselves; the server is done. **(P2P leg out of scope.)**

- **`MatchingParameter` (DS2P:451)** — the DS2 matchmaking vector, present in nearly every matchmaking request:
  ```
  calibration_version=1, soul_level=2, clear_count=3, unknown_4=4, covenant=6,
  unknown_7=7 (nat type?), disable_cross_region_play=8, unknown_9=9 (region?),
  unknown_10=10, name_engraved_ring=11, soul_memory=12
  ```
  (field 5 is absent). **DS3's `MatchingParameter` (DS3P:609) is a different message** — `regulation_version, unknown_id_2, allow_cross_region, nat_type, unknown_id_5, soul_level, soul_memory, unknown_string, clear_count, password, covenant, weapon_level, unknown_id_15`. DS2 has **no password field**; DS3 does. Matching in DS2 is driven by `soul_memory` and `name_engraved_ring` (`DS2_SignManager.cpp:151`), not soul level.
- `RequestCreateSign` (DS2P:504) `online_area_id, MatchingParameter, bytes player_struct, cell_id, sign_type` → `sign_id` (DS2P:512). `SignType` (DS2P:574): `WhiteSoapstoneSunlight=0, WhiteSoapstone=1, SmallWhiteSoapstoneSunlight=2, SmallWhiteSoapstone=3, RedSoapstone=4, Dragon=6` (`MirrorKnight=99` is a DS3OS-internal sentinel, **not** on the wire). DS3 has only `WhiteSoapstone=0, RedSoapstone=1` (DS3P:595). Handler `:234`; the `player_struct` blob is NRSSR-validated (CVE-2022-24126 fix) at `:242-253`.
- `RequestGetSignList` (DS2P:516) `online_area_id, repeated SignCellInfo search_areas, max_signs, MatchingParameter, unknown_5..7` where `SignCellInfo` (DS2P:563) is `cell_id, repeated SignInfo local_signs, optional max_signs` and `SignInfo` (DS2P:569) is `player_id, sign_id`.
  Reply (DS2P:526) `repeated SignInfo sign_info = 1; repeated SignData sign_data = 2;` — signs the client **already has** come back as bare `SignInfo`, new ones as full `SignData` (DS2P:587: `sign_info, online_area_id, MatchingParameter, bytes player_struct, player_steam_id, cell_id, SignType`). Logic at `DS2_SignManager.cpp:192-212`. Requesting a sign also registers the requester in the sign's `AwarePlayerIds` set (`:215`), which is what drives `PushRequestRemoveSign`. Handler `:158`.
  **DS3 differs structurally:** `SignDomainGetInfo` (DS3P:643) replaces `SignCellInfo`, addressing is `map_id`+`online_area_id` rather than `online_area_id`+`cell_id`, and the reply wraps everything in `GetSignResult` (DS3P:585).
- `RequestSummonSign` (DS2P:477) `int64 online_area_id, SignInfo sign_info, bytes player_struct, int64 cell_id` → empty reply (DS2P:484, `// Never received`). On success the server pushes `PushRequestSummonSign` to the **sign owner** and marks the sign `BeingSummonedByPlayerId` (`DS2_SignManager.cpp:415-430`). On failure — sign gone, or already being summoned, or NRSSR validation failed — the server sends the *requester* a `PushRequestRejectSign` (`:445-455`). Handler `:366`.
- `SummonErrorId` (DS2P:445): `NoLongerBeSummonable=0, SignAlreadyUsed=1, SignHasDisappeared=2`.
- `PushRequestSummonSign` (DS2P:469) `push_message_id, int64 player_id, int64 sign_id, bytes player_struct, string player_steam_id`.
- `PushRequestRejectSign` (DS2P:488) `push_message_id, SignInfo sign_info, SummonErrorId error, string player_steam_id`.
- `PushRequestRemoveSign` (DS2P:497) `push_message_id, int64 player_id, int64 sign_id, string player_steam_id`. Broadcast to every aware player when the sign is removed or its owner disconnects (`DS2_SignManager.cpp:50-76`, called from `OnLostPlayer` at `:40`).
- `RequestRejectSign` (DS2P:531) `int64 online_area_id, int64 sign_id, SummonErrorId error, int64 unknown_4, int64 cell_id` → empty. Relays a `PushRequestRejectSign` to whoever was summoning (`:492-503`). Handler `:471`.
- `RequestRemoveSign` (DS2P:543) / `RequestUpdateSign` (DS2P:553), both `online_area_id, sign_id, cell_id` → empty. `UpdateSign` is a pure keepalive that DS3OS ignores (`:351-352`). Handlers `:310` / `:347`.
- `RequestGetRightMatchingArea` (DS2P:597) `MatchingParameter` → (DS2P:601) `repeated group Area_info = 1 { online_area_id = 1; population = 2; }` — a **proto2 group**, not a nested message; it uses START_GROUP/END_GROUP wire tags. Powers the "recommended area" UI. Handler `:525`.

**DS2/DS3 push-shape divergence.** DS3 wraps all three sign pushes in a two-field envelope (`push_message_id` + nested `SummonSignMessage`/`RemoveSignMessage`/`RejectSignMessage`, DS3P:563-578, DS3P:728-742); DS2 flattens the fields directly. DS2 uses `int64` ids and carries `player_steam_id` on all three; DS3 uses `uint32` and only carries it on the summon push. DS2's reject push has a typed `SummonErrorId`; DS3's is a bare `sign_id` + `unknown_2`.

### 4.7 Mirror Knight — **DS2-only**

The Mirror Knight boss fight summons a real player as the mirror knight. It is a complete parallel copy of the sign system with a **global** (non-area-keyed) sign pool.

| Opcode | Name | Dir | Reply |
|---|---|---|---|
| `0x039E` | `RequestCreateMirrorKnightSign` | C→S | `...Response` (`int64 sign_id`) |
| `0x039F` | `RequestUpdateMirrorKnightSign` | C→S | empty |
| `0x03A0` | `RequestRemoveMirrorKnightSign` | C→S | empty |
| `0x03A1` | `RequestGetMirrorKnightSignList` | C→S | `...Response` |
| `0x03A2` | `RequestSummonMirrorKnightSign` | C→S | empty |
| `0x03A4` | `RequestRejectMirrorKnightSign` | C→S | empty |
| `0x03A5` | `PushRequestSummonMirrorKnightSign` | push | — |
| `0x03A6` | `PushRequestRejectMirrorKnightSign` | push | — |
| `0x03A7` | `PushRequestRemoveMirrorKnightSign` | push | — |

`DS2_..._Types.inc:108-117`. Handlers: `.../MirrorKnight/DS2_MirrorKnightManager.cpp`, dispatch `:79`.

- `RequestCreateMirrorKnightSign` (DS2P:636) `MatchingParameter matching_parameter = 1; bytes data = 2;` → `int64 sign_id`. Handler `:165`.
- `RequestGetMirrorKnightSignList` (DS2P:645) `int64 max_signs, MatchingParameter` → (DS2P:650) `repeated SignData sign_data = 2;` (**tag 1 skipped**). Handler `:122`.
- `RequestSummonMirrorKnightSign` (DS2P:675) `SignInfo sign_info, bytes player_struct` → empty. Same push-to-owner / push-reject-to-requester pattern as regular signs. Handler `:260`; NRSSR-validated at `:270-280`.
- `RequestRemoveMirrorKnightSign` (DS2P:667) `int64 sign_id`; `RequestUpdateMirrorKnightSign` (DS2P:684) `int64 sign_id`. Handlers `:212` / `:241`.
- `RequestRejectMirrorKnightSign` (DS2P:655) has the same shape as `RequestRejectSign`. **The proto comment at DS2P:654 says the game never sends it and uses `RequestRejectSign` instead**, but DS3OS still registers a handler at `:368`.
- The three pushes (DS2P:613/621/628) mirror the regular sign pushes field-for-field.
- Matching uses a dedicated `DS2_MirrorKnightMatchingParameters` config band (`:119`).
- **There is no DS3 counterpart to any of this.**

### 4.8 Break-in (invasions)

| DS2 op | DS3 op | Name | Dir | Reply |
|---|---|---|---|---|
| `0x03D2` | `0x03B1` | `RequestGetBreakInTargetList` | C→S | `...Response` |
| `0x03D3` | `0x03B2` | `RequestBreakInTarget` | C→S | empty |
| `0x03D4` | `0x03B3` | `RequestRejectBreakInTarget` | C→S | empty |
| `0x03FB` | `0x03A5` | `PushRequestBreakInTarget` | push | — |
| `0x03FC` | `0x03A6` | `PushRequestRejectBreakInTarget` | push | — |
| `0x03FD` | `0x03A7` | `PushRequestAllowBreakInTarget` | push (relayed) | — |
| — | — | `PushRequestRemoveBreakInTarget` | never registered |

`DS2_..._Types.inc:122-131`. Handlers: `.../BreakIn/DS2_BreakInManager.cpp`, dispatch `:36`.

**Flow** (DS3 proto comment `DS3P:748-756`; DS2 behaves the same):

```
Invader                     Server                      Host
 -- RequestGetBreakInTargetList -->
 <-- ...Response{target_data[]} ---
 -- RequestBreakInTarget --------->
                            -- PushRequestBreakInTarget -->
                            <-- RequestRejectBreakInTarget  (decline)
                                 OR
                            <-- RequestSendMessageToPlayers  (accept — carrying
                                    a serialized PushRequestAllowBreakInTarget)
 <-- PushRequestAllowBreakInTarget / PushRequestRejectBreakInTarget --
 -- (DS3 only) RequestNotifyBreakInResult -->
```

**Key structural fact:** `PushRequestAllowBreakInTarget` is **never constructed by the server**. The host client serializes it itself and tunnels it via `RequestSendMessageToPlayers`; the server relays the opaque bytes. Verified for DS2: grepping the whole DS2 tree finds `PushRequestAllowBreakInTarget` only in the NRSSR allow-list (`Source/Server.DarkSouls2/Server/GameService/Utils/DS2_NRSSRSanitizer.h:121`), never in a send path. This is exactly why `RequestSendMessageToPlayers` needs the CVE-2022-24125 sanity checks.

- `BreakInType` (DS2P:696): `RedEyeOrb=0, BlueEyeOrb=2`.
- `RequestGetBreakInTargetList` (DS2P:745) `online_area_id, cell_id, max_targets, MatchingParameter, optional BreakInType type` → (DS2P:753) `optional online_area_id, optional cell_id, repeated BreakInTargetData target_data` where `BreakInTargetData` (DS2P:701) is `player_id, string steam_id`. Handler `:95`. Note DS3OS filters by the invader's *played areas* when `IgnoreInvasionAreaFilter` is set (`:108-125`).
- `RequestBreakInTarget` (DS2P:734) `online_area_id, cell_id, player_id, optional BreakInType type` → empty. Pushes `PushRequestBreakInTarget` to the target (`:179-191`); on failure pushes `PushRequestRejectBreakInTarget` back to the invader (`:210-226`). Handler `:159`.
- `PushRequestBreakInTarget` (DS2P:713) `push_message_id, player_id, string steam_id, BreakInType type, online_area_id, cell_id`. **DS3's field 4 is an always-zero `unknown_4` and fields 5/6 are `map_id`/`online_area_id`** (DS3P:784) — same wire positions, different meanings.
- `PushRequestAllowBreakInTarget` (DS2P:706) `push_message_id, player_id, bytes player_struct, unknown_4`. Byte-compatible with DS3's (DS3P:777).
- `PushRequestRejectBreakInTarget` (DS2P:722) `push_message_id, int64 player_id, int64 unknown_3, string steam_id, int64 unknown_5` (DS3 uses `uint32`).
- `RequestRejectBreakInTarget` (DS2P:759) `int64 player_id, unknown_2 (reason?), online_area_id, cell_id, unknown_5` → empty. Relays a reject push to the invader (`:247-257`). Handler `:232`.
- `PushRequestRemoveBreakInTarget` — DS2P:730 is entirely empty ("I don't think we actually need to implement this"); DS3P:793 has three speculative Ghidra-derived fields. Neither game's `.inc` registers it (`DS2_..._Types.inc:126-127`).

### 4.9 Visitors (covenant auto-summon)

| DS2 op | DS3 op | Name | Dir | Reply |
|---|---|---|---|---|
| `0x03D5` | `0x03B4` | `RequestGetVisitorList` | C→S | `...Response` |
| `0x03D6` | `0x03B5` | `RequestVisit` | C→S | empty |
| `0x03D7` | `0x03B6` | `RequestRejectVisit` | C→S | empty |
| `0x03CF` | `0x03B7` | `PushRequestVisit` | push | — |
| `0x03D0` | `0x03B8` | `PushRequestRejectVisit` | push | — |
| `0x03D1` | `0x03B9` | ~~`PushRequestRemoveVisitor`~~ **WRONG — see below** | push | — |

`DS2_..._Types.inc:136-142`. Handlers: `.../Visitor/DS2_VisitorManager.cpp`, dispatch `:33`.

**Flow** (DS3 proto comment `DS3P:869-877` — "same as invasions just without the `PushRequestAllowBreakInTarget` exchange step"):

```
Visitor                     Server                      Host
 -- RequestGetVisitorList ------->
 <-- ...Response{target_data[]} --
 -- RequestVisit ---------------->
                            -- PushRequestVisit ---------->
 <-- PushRequestRemoveVisitor ----   (server sends this to the *visitor*, immediately, on success)
   [ or, on failure/host decline: ]
 <-- PushRequestRejectVisit ------
```

The immediate `PushRequestRemoveVisitor` back to the requester on success is explicit and deliberate (`DS2_VisitorManager.cpp:212-228`, note the comment).

- `VisitorType` (DS2P:806): `None=-1 (sentinel), BlueSentinels=0 (guessed), BellKeepers=1, Rat=2, 3=3`. **DS3's pool enum is completely different** (`VisitorPool` DS3P:882: Way_of_Blue, Debug, Watchdog_of_Farron, Aldrich_Faithful, Spear_of_the_Church).
- `RequestGetVisitorList` (DS2P:820) `int64 online_area_id, cell_id, max_targets, MatchingParameter, VisitorType type, int64 field_6` → (DS2P:829) `online_area_id, cell_id, repeated VisitorData target_data` where `VisitorData` (DS2P:815) is `int64 player_id, string player_steam_id`. Handler `:89`.
- `RequestVisit` (DS2P:842) `online_area_id, cell_id, VisitorType type, player_id, bytes player_struct` → empty. NRSSR-validated (`:140-150`). Handler `:130`.
- `PushRequestVisit` (DS2P:875) `push_message_id, int64 player_id, string player_steam_id, bytes player_struct, VisitorType type, online_area_id, cell_id`.
- `PushRequestRejectVisit` (DS2P:866) `push_message_id, int64 player_id, int64 unknown_3 (reason?), string steam_id, VisitorType type`.
- `PushRequestRemoveVisitor` (DS2P:835) `push_message_id, int64 player_id, string player_steam_id, VisitorType type`.
- `RequestRejectVisit` (DS2P:854) `int64 player_id, unknown_2 (reason?), online_area_id, cell_id, optional VisitorType type` → empty. Handler `:242`.

### 4.10 Quick match (Undead Match / arena)

| DS2 op | Name | Dir | Reply |
|---|---|---|---|
| `0x03D9` | `RequestRegisterQuickMatch` | C→S | empty |
| `0x03DA` | `RequestUnregisterQuickMatch` | C→S | empty |
| `0x03DB` | `RequestUpdateQuickMatch` | C→S | empty (keepalive) |
| `0x03DC` | `RequestSearchQuickMatch` | C→S | `...Response{matches[]}` |
| `0x03DD` | `RequestJoinQuickMatch` | C→S | empty |
| `0x03DE` | `RequestRejectQuickMatch` | C→S | empty |
| `0x03E1` | `PushRequestJoinQuickMatch` | push | — |
| `0x03E3` | `PushRequestRejectQuickMatch` | push | — |
| `0x03E5` | `PushRequestAllowQuickMatch` | push (relayed) | — |
| `0x03E7` | `PushRequestRemoveQuickMatch` | push (relayed) | — |

`DS2_..._Types.inc:163-173`. Handlers: `.../QuickMatch/DS2_QuickMatchManager.cpp`, dispatch `:56`.

- `QuickMatchGameMode` (DS2P:989): **only two values** — `Blue=0` (Blue Sentinel), `Brotherhood=1` (Brotherhood of Blood). DS3's has 8+ (duel, brawls, team modes; DS3P:1209).
- `RequestRegisterQuickMatch` (DS2P:1052) `int64 online_area_id, cell_id, MatchingParameter, QuickMatchGameMode mode` → empty. Creates a host entry. Handler `:185`.
- `RequestSearchQuickMatch` (DS2P:1076) `online_area_id, cell_id, MatchingParameter, max_results, mode` → (DS2P:1084) `repeated QuickMatchData matches` where `QuickMatchData` (DS2P:1032) is `player_id, online_area_id, cell_id, MatchingParameter, player_steam_id, optional mode`. Matching requires exact area+cell+mode plus a soul-memory band (`:98-111`). Handler `:150`.
- `RequestJoinQuickMatch` (DS2P:1041) `online_area_id, cell_id, player_id, mode` → empty. Pushes `PushRequestJoinQuickMatch` to the host (`:294-306`); on any failure pushes `PushRequestRejectQuickMatch` back to the joiner (`:316-329`) **and returns without sending the normal empty reply** (`:331`) — worth noting for a client emulator. Handler `:277`.
- `RequestRejectQuickMatch` (DS2P:1063) `player_id, online_area_id, cell_id, mode, unknown_5` → empty; relays a reject push to the joiner (`:354-366`). Handler `:344`.
- `RequestUnregisterQuickMatch` (DS2P:1088) / `RequestUpdateQuickMatch` (DS2P:1098), both `online_area_id, cell_id, mode` → empty. Handlers `:230` / `:261`.
- `PushRequestJoinQuickMatch` (DS2P:1002) `push_message_id, player_id, player_steam_id, online_area_id, cell_id, mode`.
- `PushRequestRejectQuickMatch` (DS2P:1011) same + `unknown_7` (reason).
- `PushRequestAllowQuickMatch` (DS2P:994) `push_message_id, player_id, online_area_id, cell_id, bytes field_5` and `PushRequestRemoveQuickMatch` (DS2P:1023) `push_message_id, player_id, online_area_id, cell_id, player_steam_id, mode` — **neither is ever sent by DS3OS**; both appear only in the NRSSR allow-list (`DS2_NRSSRSanitizer.h:141, :156`), i.e. they are client-tunnelled through `RequestSendMessageToPlayers` like the break-in allow push. This is why `DS2_MiscManager.cpp:64-66` disables the "break-in only" restriction and permits up to 6 recipients (`:74`).
- **DS2 has no arena result reporting.** There is no `RequestAcceptQuickMatch`, `RequestSendQuickMatchStart`, or `RequestSendQuickMatchResult` in DS2 — those are DS3-only (`0x0450`, `0x0454`, `0x0455`), and DS3's are server-authoritative for rank/XP (`DS3_QuickMatchManager.cpp:499`, XP thresholds at `:552-564`). DS2 rank progression, if any, is not visible in this protocol. **(gap)**

### 4.11 Ranking / leaderboards

**DS2 (Power Stone / "rat king" style board)** — `DS2_..._Types.inc:100-103`; handlers `.../Ranking/DS2_RankingManager.cpp`, dispatch `:28`.

| Op | Name | Reply |
|---|---|---|
| `0x03F3` | `RequestRegisterPowerStoneData` | empty |
| `0x03F4` | `RequestGetPowerStoneRanking` | `...Response{repeated PowerStoneRankingData}` |
| `0x03F5` | `RequestGetPowerStoneMyRanking` | `...Response{PowerStoneRankingData}` |
| `0x03F8` | `RequestGetPowerStoneRankingRecordCount` | `...Response{count}` |

- `PowerStoneRankingData` (DS2P:940) `player_id, character_id, serial_rank, rank, score, bytes data`.
- `RequestRegisterPowerStoneData` (DS2P:976) `character_id, increment, bytes data` — note **`increment`**, not an absolute score; the server adds it to the existing total (`:60-65`). Handler `:50`. **DS2 uses a single implicit board (id 0)** (`:61, :67`) — there is no `board_id` field, unlike DS3.
- `RequestGetPowerStoneRanking` (DS2P:960) `offset, count`. Handler `:87`.
- `RequestGetPowerStoneMyRanking` (DS2P:952) `character_id`; if the player has no row the server still replies with a zero-filled record rather than omitting it (`:135-143`). Handler `:116`.
- `RequestGetPowerStoneRankingRecordCount` (DS2P:969) is an **empty request**. Handler `:154`.
- `RankingRecordCount` (DS2P:934), `RankingRotationID` (DS2P:937), `PowerStoneRankingDataPack` (DS2P:949) are declared with **no fields** — un-reversed. `PowerStoneRankingData.data` is probably a serialized `PowerStoneRankingDataPack`. **(inferred)**

**DS3 (generic multi-board)** — `DS3_..._Types.inc:97-100`; handlers `.../Ranking/DS3_RankingManager.cpp:28`. `RequestRegisterRankingData` (DS3P:1132) `board_id, character_id, score, bytes data`; `RequestGetRankingData` (DS3P:1143) `board_id, offset, count`; `RequestGetCharacterRankingData` (DS3P:1153) `board_id, character_id`; `RequestCountRankingData` (DS3P:1162) `board_id`. Record type `RankingData` (DS3P:1108) is field-identical to DS2's `PowerStoneRankingData`. `RequestGetCurrentRank` (DS3P:1170) exists but has **no registered opcode** (`DS3_..._Types.inc:102-103`).

### 4.12 Logging / telemetry

**DS2 — eleven fire-and-forget notifications, every one of which still gets an `EmptyResponse` reply.** `DS2_..._Types.inc:85-95`; handlers `.../Logging/DS2_LoggingManager.cpp`, dispatch `:31`. All eleven `...Response` types are empty with `// Never received.` (DS2P:326, 341, 349, 365, 376, 387, 399, 410, 421, 429, 437), and the server literally sends `DS2_Frpg2RequestMessage::EmptyResponse` (DS2P:51) — e.g. `DS2_LoggingManager.cpp:96`.

| Op | Name | Payload (DS2P) | Handler |
|---|---|---|---|
| `0x03E8` | `RequestNotifyJoinGuestPlayer` | :353 — `field_1..field_8` (int64), `bytes field_9` | `:145` |
| `0x03E9` | `RequestNotifyLeaveGuestPlayer` | :403 — `field_1..field_4` (int64) | `:226` |
| `0x03EA` | `RequestNotifyJoinSession` | :369 — `field_1..field_4` (int64) | `:161` |
| `0x03EB` | `RequestNotifyLeaveSession` | :414 — same shape as JoinSession | `:242` |
| `0x03ED` | `RequestNotifyKillPlayer` | :391 — `field_1..field_5` (int64; field_2 is a player id) | `:210` |
| `0x03D8` | `RequestNotifyMirrorKnight` | :425 — `int64 field_1` | `:263` — **DS2-only** |
| `0x03F1` | `RequestNotifyDeath` | :330 — `online_area_id, cell_id (uint32), field_3..field_7 (int64), bytes field_8` | `:106` |
| `0x03F2` | `RequestNotifyOfflineDeathCount` | :433 — `int64 count` | `:279` |
| `0x03F6` | `RequestNotifyKillEnemy` | :380 — `repeated group Enemy_count = 1 { enemy_id, enemy_count }` (**proto2 group**) | `:177` |
| `0x03F7` | `RequestNotifyBuyItem` | :319 — `merchant_id, item_id, souls_spent, quantity` (uint32) | `:81` |
| `0x03F9` | `RequestNotifyDisconnectSession` | :345 — `int64 field_1` | `:129` |

Most of these are un-reversed `field_N` placeholders. `RequestNotifyBuyItem` and `RequestNotifyOfflineDeathCount` are the only two with fully meaningful names.

**DS3** takes an entirely different approach: all its logging messages are `DEFINE_MESSAGE` (**no reply at all**, `DS3_..._Types.inc:82-92`), they are far better reversed, and most of the detail is funnelled through a single generic envelope:

| DS3 op | Name | Notes |
|---|---|---|
| `0x03CD` | `RequestNotifyKillEnemy` | DS3P:427 — `LogCommonInfo`, `repeated KillEnemyInfo`, `map_id`, `Vector location` |
| `0x03CF` | `RequestNotifyDisconnectSession` | DS3P:501 — un-reversed; "seemingly unused" |
| `0x03D0` | `RequestNotifyRegisterCharacter` | DS3P:510 — `AllStatus` + 9 unknowns |
| `0x03D1` | `RequestNotifyDie` | DS3P:415 — `map_id`, `Vector`, `CauseOfDeath`, `souls_dropped/lost`, `actor_id`, `KillerInfo` |
| `0x03D2` | `RequestNotifyKillBoss` | DS3P:435 — `boss_id`, `in_coop`, `boss_died`, `cooperator_count`, `fight_duration`, `map_id` (**field 7 skipped**) |
| `0x03D3` | `RequestNotifyJoinMultiplay` | DS3P:446 |
| `0x03D4` | `RequestNotifyLeaveMultiplay` | DS3P:456 — one extra field vs Join |
| `0x03D5` | `RequestNotifyCreateSignResult` | DS3P:467 |
| `0x03D6` | `RequestNotifySummonSignResult` | DS3P:479 |
| `0x03D7` | `RequestNotifyBreakInResult` | DS3P:491 — un-reversed |
| `0x03D8` | `RequestNotifyProtoBufLog` | DS3P:390 — `LogType type = 1; bytes common = 2; bytes data = 3;` where `data` is a nested protobuf chosen by `type` from `Protobuf/DarkSouls3/DS3_FpdLogMessage.proto` |

`LogType` (DS3P:334-347): `UseMagicLog=2020, ActGestureLog=2021, UseItemLog=3000, PurchaseItemLog=3001, GetItemLog=3002, DropItemLog=3003, LeaveItemLog=3004, SaleItemLog=3005, StrengthenWeaponLog=3010, GlobalEventLog=5001, VisitResultLog=7040, QuickMatchResultLog=7050, QuickMatchEndLog=7060, SystemOptionLog=8001`. Handler `DS3_LoggingManager.cpp:81` dispatches 10 of the 14 (`:90-99`); the other four fall through silently. **DS2 has no `RequestNotifyProtoBufLog` and no FpdLogMessage layer at all.**

### 4.13 Bell (Belfry / Archdragon Peak)

| DS2 op | DS3 op | Name | Dir | Reply |
|---|---|---|---|---|
| `0x03EE` | `0x03C8` | `RequestNotifyRingBell` | C→S | `...Response` |
| `0x03C9` (commented out) | `0x03C9` | `PushRequestNotifyRingBell` | push | — |

- **DS2: opcode exists, message is un-reversed, and nothing handles it.** `RequestNotifyRingBell` (DS2P:894) and its response (DS2P:898) are both empty with only `// TODO`; `PushRequestNotifyRingBell` (DS2P:889) has just `push_message_id` + `// TODO`. `DS2_MiscManager::OnMessageReceived` dispatches only `RequestSendMessageToPlayers` and `RequestGetTotalDeathCount` (`DS2_MiscManager.cpp:39, :43`), so a DS2 client sending `0x03EE` would fall through every manager. Its push is commented out of the `.inc` at `:149`. **If a DS2 client actually rings a bell, DS3OS does not answer — and we don't know the payload.** (gap)
- **DS3: fully implemented.** `RequestNotifyRingBell` (DS3P:967) `online_area_id, bytes data`; the server finds every client in one of 8 hardcoded Archdragon Peak areas and relays `PushRequestNotifyRingBell` (DS3P:976) `push_message_id, player_id, online_area_id, bytes data` to each (`DS3_MiscManager.cpp:71`, area list `:79-88`, relay `:96-106`).

### 4.14 Misc / networking

| DS2 op | DS3 op | Name | Dir | Reply |
|---|---|---|---|---|
| `0x0320` | `0x0320` | `RequestSendMessageToPlayers` | C→S | `...Response` (empty) |
| `0x0389` | `0x0389` | `ManagementTextMessage` | push | — |
| `0x03F0` | — | `RequestGetTotalDeathCount` | C→S | `...Response{total_death_count}` — **DS2-only** |

`DS2_..._Types.inc:183-184, :178`. Handlers `.../Misc/DS2_MiscManager.cpp`, dispatch `:37`.

- **`RequestSendMessageToPlayers`** (DS2P:1172) `repeated uint32 player_ids = 1; required bytes message = 2;` — the client-to-client relay primitive. `message` is a serialized push protobuf whose field 1 is a `PushMessageId`. The server looks each id up and calls `SendRawProtobuf` on the target's stream, forwarding the bytes untouched (`DS2_MiscManager.cpp:100-119`). `SendRawProtobuf` forces `msg_type = Push` (`Frpg2ReliableUdpMessageStream.cpp:135-142`).
  This is the mechanism behind `PushRequestAllowBreakInTarget`, `PushRequestAllowQuickMatch`, `PushRequestRemoveQuickMatch`, and (per the comment at `:64`) some ids DS3OS still hasn't identified.
  Security: **CVE-2022-24125** (arbitrary message injection) is mitigated by a ≤6-recipient cap (`:74`) and **CVE-2022-24126** (malformed NRSSR entry lists) by `DS2_NRSSRSanitizer::ValidatePushMessages` (`:84`), whose per-push-id switch at `DS2_NRSSRSanitizer.h:121-165` is a useful independent enumeration of which pushes the game legitimately tunnels. Handler `:55`.
- **`ManagementTextMessage`** (DS2P:1131) `push_message_id, string message, DateTime timestamp, unknown_4, unknown_5` — the server-to-client admin banner. Sent from `DS2_Game.cpp:188`. Push, never a reply.
- **`RequestGetTotalDeathCount`** (DS2P:1112) is an **empty request** → (DS2P:1115) `required uint32 total_death_count = 1;`. Global death counter for the "you died N times" UI. Handler `:133`. No DS3 equivalent.

**DS3-only Misc/networking** (`DS3_..._Types.inc:169-171, :176, :185, :198, :200`): `RequestMeasureUploadBandwidth` (`0x038E`), `RequestMeasureDownloadBandwidth` (`0x038F`), `RequestBenchmarkThroughput` (`0x03A3`), `RequestGetOnlineShopItemList` (`0x03DB`) — all four are `Ensure(false)` stubs DS3OS has never observed (`DS3_MiscManager.cpp:200, :217, :234, :251`), with un-reversed bodies. Plus two pushes DS3OS never sends: `RegulationFileUpdatePushMessage` (`0x038B`, DS3P:1016) and `ServerPing` (`0x038D`, DS3P:1556, `required uint32 unknown_1 = 1` — the client reportedly ignores it).

### 4.15 Regulation files

Declared in both protos (DS2P:906-928, DS3P:989-1019) but **no request opcode is registered in either game** (`DS2_..._Types.inc:157-158`, `DS3_..._Types.inc:182-183`). DS3 registers only the push (`0x038B`) and never sends it. All bodies are un-reversed. Effectively dead for our purposes.

### 4.16 Ritual marks — DS3-only, cut content

`0x0460` `RequestCreateMark`, `0x0461` `RequestRemoveMark`, `0x0462` `RequestReentryMark`, `0x0463` `RequestGetMarkList` (`DS3_..._Types.inc:190-193`). All bodies un-reversed (DS3P:1452-1512). All four handlers are `Ensure(false)` and — unlike the Misc stubs — **return without sending any response at all** (`DS3_MarkManager.cpp:49, :59, :69, :79`). DS3P:1450 calls it "cut content from map ceremonies". No DS2 equivalent.

### 4.17 Anti-cheat (DS3-only)

**Consumes no network messages.** `DS3_AntiCheatManager` does not override `OnMessageReceived` and never appears in the dispatch chain; it is a polling inspector (`DS3_AntiCheatManager.cpp:41`). Three of its four triggers read the cached `AllStatus` blob populated by `RequestUpdatePlayerStatus` (`0x03A4`, merged at `DS3_PlayerDataManager.cpp:103-135`); the fourth (`Exploit`) reads a flag nothing in the tree ever sets. Enforcement is out-of-band: ban/disconnect/warning via the announce list and `ManagementTextMessage` (`DS3_AntiCheatManager.cpp:98-155`). **DS2 has no anti-cheat manager at all** — only the shared NRSSR payload sanitizers, which live inline in the handlers.

---

## 5. Summary opcode tables

### 5.1 Login / auth (TCP) — shared

| Type | Name | Dir | Reply | Crypto |
|---|---|---|---|---|
| `0` | `Reply` | S→C | — | context |
| `1` | `KeyMaterial` | C→S | raw 16 bytes | CWC |
| `2` | `GetServiceStatus` | C→S | `GetServiceStatusResponse` | CWC |
| `3` | `SteamTicket` | C→S | raw `Frpg2GameServerInfo` (184 B) | CWC |
| `5` | `RequestQueryLoginServerInfo` | C→S | `...Response` | RSA |
| `6` | `RequestHandshake` | C→S | raw 27 bytes, **plaintext** | RSA in, none out |

### 5.2 DS2 game service — full opcode list

R/R = request/response · M = no reply · P = push (transport 0x0320, id in field 1)

| Op | Kind | Name | Area | Handler (`Source/Server.DarkSouls2/Server/GameService/GameManagers/…`) |
|---|---|---|---|---|
| `0x0320` | R/R | `RequestSendMessageToPlayers` | Misc | `Misc/DS2_MiscManager.cpp:55` |
| `0x0386` | R/R | `RequestWaitForUserLogin` | Boot | `Boot/DS2_BootManager.cpp:46` |
| `0x0389` | P | `ManagementTextMessage` | Misc | sent `DS2_Game.cpp:188` |
| `0x038C` | P | `PlayerInfoUploadConfigPushMessage` | Boot | sent `Boot/DS2_BootManager.cpp:106` |
| `0x0391` | **M** | `RequestCreateBloodstain` | Bloodstain | `Bloodstain/DS2_BloodstainManager.cpp:91` |
| `0x0392` | R/R | `RequestGetBloodstainList` | Bloodstain | `…:165` |
| `0x0393` | R/R | `RequestGetDeadingGhost` | Ghost | `Bloodstain/DS2_BloodstainManager.cpp:250` |
| `0x0394` | R/R | `RequestCreateSign` | Sign | `Signs/DS2_SignManager.cpp:234` |
| `0x0395` | R/R | `RequestUpdateSign` | Sign | `…:347` |
| `0x0396` | R/R | `RequestRemoveSign` | Sign | `…:310` |
| `0x0397` | R/R | `RequestGetSignList` | Sign | `…:158` |
| `0x0398` | R/R | `RequestSummonSign` | Sign | `…:366` |
| `0x039A` | R/R | `RequestRejectSign` | Sign | `…:471` |
| `0x039B` | P | `PushRequestSummonSign` | Sign | sent `…:415` |
| `0x039C` | P | `PushRequestRejectSign` | Sign | sent `…:445`, `…:492` |
| `0x039D` | P | `PushRequestRemoveSign` | Sign | sent `…:61` |
| `0x039E` | R/R | `RequestCreateMirrorKnightSign` | MirrorKnight | `MirrorKnight/DS2_MirrorKnightManager.cpp:165` |
| `0x039F` | R/R | `RequestUpdateMirrorKnightSign` | MirrorKnight | `…:241` |
| `0x03A0` | R/R | `RequestRemoveMirrorKnightSign` | MirrorKnight | `…:212` |
| `0x03A1` | R/R | `RequestGetMirrorKnightSignList` | MirrorKnight | `…:122` |
| `0x03A2` | R/R | `RequestSummonMirrorKnightSign` | MirrorKnight | `…:260` |
| `0x03A4` | R/R | `RequestRejectMirrorKnightSign` | MirrorKnight | `…:368` |
| `0x03A5` | P | `PushRequestSummonMirrorKnightSign` | MirrorKnight | `…:260` path |
| `0x03A6` | P | `PushRequestRejectMirrorKnightSign` | MirrorKnight | sent `…:392` |
| `0x03A7` | P | `PushRequestRemoveMirrorKnightSign` | MirrorKnight | — |
| `0x03A8` | R/R | `RequestUpdatePlayerCharacter` | PlayerData | `PlayerData/DS2_PlayerDataManager.cpp:354` |
| `0x03A9` | R/R | `RequestGetPlayerCharacter` | PlayerData | `…:380` |
| `0x03AA` | P | `PushRequestEvaluateBloodMessage` | BloodMsg | sent `BloodMessage/DS2_BloodMessageManager.cpp:408` |
| `0x03AB` | R/R | `RequestCreateBloodMessage` | BloodMsg | `…:184` |
| `0x03AC` | R/R | `RequestRemoveBloodMessage` | BloodMsg | `…:234` |
| `0x03AD` | R/R | `RequestReentryBloodMessage` | BloodMsg | `…:111` |
| `0x03AE` | R/R | `RequestGetBloodMessageList` | BloodMsg | `…:264` |
| `0x03AF` | R/R | `RequestEvaluateBloodMessage` | BloodMsg | `…:358` |
| `0x03B0` | R/R | `RequestGetBloodMessageEvaluation` | BloodMsg | `…:143` |
| `0x03B1` | R/R | `RequestCreateGhostData` | Ghost | `Ghosts/DS2_GhostManager.cpp:83` |
| `0x03B2` | R/R | `RequestGetGhostDataList` | Ghost | `…:153` |
| `0x03B3` | R/R | `RequestGetLoginPlayerCharacter` | PlayerData | `PlayerData/DS2_PlayerDataManager.cpp:408` |
| `0x03B6` | R/R | `RequestUpdateLoginPlayerCharacter` | PlayerData | `…:58` |
| `0x03B8` | R/R | `RequestUpdatePlayerStatus` | PlayerData | `…:125` |
| `0x03C9` | P | `PushRequestNotifyRingBell` | Bell | **not registered** (`.inc:149`) |
| `0x03CF` | P | `PushRequestVisit` | Visitor | sent `Visitor/DS2_VisitorManager.cpp:163` |
| `0x03D0` | P | `PushRequestRejectVisit` | Visitor | sent `…:195`, `…:257` |
| `0x03D1` | P | `PushRequestRemoveVisitor` | Visitor | sent `…:215` |
| `0x03D2` | R/R | `RequestGetBreakInTargetList` | BreakIn | `BreakIn/DS2_BreakInManager.cpp:95` |
| `0x03D3` | R/R | `RequestBreakInTarget` | BreakIn | `…:159` |
| `0x03D4` | R/R | `RequestRejectBreakInTarget` | BreakIn | `…:232` |
| `0x03D5` | R/R | `RequestGetVisitorList` | Visitor | `Visitor/DS2_VisitorManager.cpp:89` |
| `0x03D6` | R/R | `RequestVisit` | Visitor | `…:130` |
| `0x03D7` | R/R | `RequestRejectVisit` | Visitor | `…:242` |
| `0x03D8` | R/R | `RequestNotifyMirrorKnight` | Logging | `Logging/DS2_LoggingManager.cpp:263` |
| `0x03D9` | R/R | `RequestRegisterQuickMatch` | QuickMatch | `QuickMatch/DS2_QuickMatchManager.cpp:185` |
| `0x03DA` | R/R | `RequestUnregisterQuickMatch` | QuickMatch | `…:230` |
| `0x03DB` | R/R | `RequestUpdateQuickMatch` | QuickMatch | `…:261` |
| `0x03DC` | R/R | `RequestSearchQuickMatch` | QuickMatch | `…:150` |
| `0x03DD` | R/R | `RequestJoinQuickMatch` | QuickMatch | `…:277` |
| `0x03DE` | R/R | `RequestRejectQuickMatch` | QuickMatch | `…:344` |
| `0x03E1` | P | `PushRequestJoinQuickMatch` | QuickMatch | sent `…:294` |
| `0x03E3` | P | `PushRequestRejectQuickMatch` | QuickMatch | sent `…:316`, `…:354` |
| `0x03E5` | P | `PushRequestAllowQuickMatch` | QuickMatch | **client-relayed only** |
| `0x03E7` | P | `PushRequestRemoveQuickMatch` | QuickMatch | **client-relayed only** |
| `0x03E8` | R/R | `RequestNotifyJoinGuestPlayer` | Logging | `Logging/DS2_LoggingManager.cpp:145` |
| `0x03E9` | R/R | `RequestNotifyLeaveGuestPlayer` | Logging | `…:226` |
| `0x03EA` | R/R | `RequestNotifyJoinSession` | Logging | `…:161` |
| `0x03EB` | R/R | `RequestNotifyLeaveSession` | Logging | `…:242` |
| `0x03EC` | R/R | `RequestGetAnnounceMessageList` | Boot | `Boot/DS2_BootManager.cpp:131` |
| `0x03ED` | R/R | `RequestNotifyKillPlayer` | Logging | `Logging/DS2_LoggingManager.cpp:210` |
| `0x03EE` | R/R | `RequestNotifyRingBell` | Bell | **no handler** |
| `0x03F0` | R/R | `RequestGetTotalDeathCount` | Misc | `Misc/DS2_MiscManager.cpp:133` |
| `0x03F1` | R/R | `RequestNotifyDeath` | Logging | `Logging/DS2_LoggingManager.cpp:106` |
| `0x03F2` | R/R | `RequestNotifyOfflineDeathCount` | Logging | `…:279` |
| `0x03F3` | R/R | `RequestRegisterPowerStoneData` | Ranking | `Ranking/DS2_RankingManager.cpp:50` |
| `0x03F4` | R/R | `RequestGetPowerStoneRanking` | Ranking | `…:87` |
| `0x03F5` | R/R | `RequestGetPowerStoneMyRanking` | Ranking | `…:116` |
| `0x03F6` | R/R | `RequestNotifyKillEnemy` | Logging | `Logging/DS2_LoggingManager.cpp:177` |
| `0x03F7` | R/R | `RequestNotifyBuyItem` | Logging | `…:81` |
| `0x03F8` | R/R | `RequestGetPowerStoneRankingRecordCount` | Ranking | `Ranking/DS2_RankingManager.cpp:154` |
| `0x03F9` | R/R | `RequestNotifyDisconnectSession` | Logging | `Logging/DS2_LoggingManager.cpp:129` |
| `0x03FA` | R/R | `RequestGetRightMatchingArea` | Sign | `Signs/DS2_SignManager.cpp:525` |
| `0x03FB` | P | `PushRequestBreakInTarget` | BreakIn | sent `BreakIn/DS2_BreakInManager.cpp:179` |
| `0x03FC` | P | `PushRequestRejectBreakInTarget` | BreakIn | sent `…:210`, `…:247` |
| `0x03FD` | P | `PushRequestAllowBreakInTarget` | BreakIn | **client-relayed only** |
| `0x03FF` | R/R | `RequestGetAreaBloodMessageList` | BloodMsg | `BloodMessage/DS2_BloodMessageManager.cpp:316` |
| `0x0400` | R/R | `RequestGetAreaBloodstainList` | Bloodstain | `Bloodstain/DS2_BloodstainManager.cpp:212` |

### 5.3 DS3 game service — full opcode list

| Op | Kind | Name | Area |
|---|---|---|---|
| `0x0320` | R/R | `RequestSendMessageToPlayers` | Networking |
| `0x033D` / `0x033E` / `0x033F` | P | `PushRequestRemoveSign` / `SummonSign` / `RejectSign` | Sign |
| `0x0340` / `0x0341` / `0x0342` | P | `PushRequestJoinQuickMatch` / `AcceptQuickMatch` / `RejectQuickMatch` | QuickMatch — **each has 7 further aliases** (`.inc:109-114`) |
| `0x0386` | R/R | `RequestWaitForUserLogin` | Boot |
| `0x0389` | P | `ManagementTextMessage` | Misc |
| `0x038B` | P | `RegulationFileUpdatePushMessage` | Regulation (never sent) |
| `0x038C` | P | `PlayerInfoUploadConfigPushMessage` | Boot |
| `0x038D` | P | `ServerPing` | Misc (never sent) |
| `0x038E` / `0x038F` | R/R | `RequestMeasureUploadBandwidth` / `DownloadBandwidth` | Networking (stubs) |
| `0x0391` | **M** | `RequestCreateBloodstain` | Bloodstain |
| `0x0392` | R/R | `RequestGetBloodstainList` | Bloodstain |
| `0x0393` | R/R | `RequestGetDeadingGhost` | Ghost |
| `0x0394` / `0x0395` | R/R | `RequestUpdatePlayerCharacter` / `GetPlayerCharacter` | PlayerData |
| `0x0396` | P | `PushRequestEvaluateBloodMessage` | BloodMsg |
| `0x0397`–`0x039C` | R/R | Create / Remove / Reentry / GetList / Evaluate / GetEvaluation BloodMessage | BloodMsg |
| `0x039D` / `0x039E` | R/R | `RequestCreateGhostData` / `GetGhostDataList` | Ghost |
| `0x039F` | R/R | `RequestGetLoginPlayerCharacter` | PlayerData |
| `0x03A1` / `0x03A2` / `0x03A4` | R/R | `GetPlayerCharacterList` / `UpdateLoginPlayerCharacter` / `UpdatePlayerStatus` | PlayerData |
| `0x03A3` | R/R | `RequestBenchmarkThroughput` | Misc (stub) |
| `0x03A5` / `0x03A6` / `0x03A7` | P | `PushRequestBreakInTarget` / `RejectBreakInTarget` / `AllowBreakInTarget` | BreakIn |
| `0x03B1`–`0x03B3` | R/R | `GetBreakInTargetList` / `BreakInTarget` / `RejectBreakInTarget` | BreakIn |
| `0x03B4`–`0x03B6` | R/R | `GetVisitorList` / `Visit` / `RejectVisit` | Visitor |
| `0x03B7` / `0x03B8` / `0x03B9` | P | `PushRequestVisit` / `RejectVisit` / `RemoveVisitor` | Visitor — **4 aliases each** (`.inc:159-164`) |
| `0x03C6` | R/R | `RequestGetAnnounceMessageList` | Boot |
| `0x03C8` / `0x03C9` | R/R / P | `RequestNotifyRingBell` / `PushRequestNotifyRingBell` | Bell |
| `0x03CD`, `0x03CF`–`0x03D8` | **M** | the eleven `RequestNotify*` telemetry messages | Logging |
| `0x03D9` | R/R | `RequestGetRightMatchingArea` | Sign |
| `0x03DA` | R/R | `RequestReCreateBloodMessageList` | BloodMsg |
| `0x03DB` | R/R | `RequestGetOnlineShopItemList` | Shop (stub) |
| `0x03E8`–`0x03EB` | R/R | Register / Get / GetCharacter / Count RankingData | Ranking |
| `0x044C`–`0x0452`, `0x0454`, `0x0455` | R/R | Search / Unregister / Update / Join / Accept / Reject / Register QuickMatch, SendQuickMatchStart, SendQuickMatchResult | QuickMatch |
| `0x0456`–`0x045B` | R/R | Create / Update / Remove / GetList / Summon / Reject Sign | Sign |
| `0x0460`–`0x0463` | R/R | Create / Remove / Reentry / GetList Mark | Marks (cut) |

### 5.4 DS2-only vs DS3-only, at a glance

| DS2-only | DS3-only |
|---|---|
| Mirror Knight (9 opcodes) | Ritual marks (4) |
| `RequestGetAreaBloodMessageList` (`0x03FF`) | `RequestReCreateBloodMessageList` (`0x03DA`) |
| `RequestGetAreaBloodstainList` (`0x0400`) | `RequestGetPlayerCharacterList` (`0x03A1`) |
| `RequestGetTotalDeathCount` (`0x03F0`) | `RequestNotifyProtoBufLog` + FpdLogMessage layer |
| `RequestNotifyBuyItem`, `RequestNotifyMirrorKnight`, `RequestNotifyOfflineDeathCount`, `RequestNotifyKillPlayer`, Join/Leave Guest Player & Session (11 R/R telemetry msgs) | 11 fire-and-forget `RequestNotify*` (`0x03CD`–`0x03D8`) |
| Power Stone ranking (single implicit board, `increment`-based) | multi-board ranking with `board_id` |
| Arena = Blue/Brotherhood only, no result reporting | Arena accept/start/result, server-authoritative XP + rank |
| — | Bell relay actually implemented; anti-cheat; bandwidth/benchmark/shop stubs; `ServerPing`; regulation-file push |

---

## 6. PS3 / vanilla-DS2 caveats

> # ⚠️ READ THIS BEFORE TRUSTING ANY OPCODE IN THIS DOCUMENT
>
> **No game-service byte has ever been observed from a PS3 client.** Every opcode, message name,
> field shape, and flow in section 4 and section 5.2 comes from DS3OS, which targets **PC /
> Scholar of the First Sin**. Our target is **PS3 / original (vanilla) Dark Souls 2, BLUS41045**.
> Testing has never progressed past the auth handshake, so **the opcode numbering itself is
> unverified for our target** — not the field shapes, not the message set, the *numbers*.
>
> This is the single largest risk in this document. Treat section 4 as a **hypothesis to be
> falsified against a capture**, not as a specification. The first PS3 game-service capture
> should be checked against the DS2 opcode table *before* any handler is written against it.
> See gap #12 in section 7.

Everything in section 4 comes from DS3OS, which targets **PC SOTFS (Steam app 335300, `app_version` 17039619)**. Confirmed deltas for **PS3 BLUS41045** (`/mnt/f/ClaudeHole/dso/docs/ps3-vs-pc.md`):

- **`app_version` = 16912640 (`0x1020000`)**, not 17039619. The `GetServiceStatus` gate at `AuthClient.cpp:153` must be widened or it sends the empty "please update" response.
- **Identity is a PSN online ID**, carried verbatim in the field named `steam_id`. Every `steam_id` / `player_steam_id` field in section 4 is really "platform account id".
- **Ticket is a PSN NP ticket**, not a Steam ticket; `SteamGameServer()->BeginAuthSession` must be replaced. Format not yet captured.
- **Login port 50011**, hostname `frpg2-ps3-ope.fromsoftware.jp`. The PS3 also does an HTTP GET for `contents_0101.bin` before going online — observed **non-blocking**.
- **Two embedded RSA keys** in the EBOOT (`0x17FB338` = login key, byte-identical to PC's; `0x189AB48` = something else, patching it breaks the flow).
- **Confirmed same:** CWC tag algorithm, AES-CTR keystream, TCP cipher framing, the 27-byte handshake blob, the login RSA key, the four-stage auth sequence.
- **Unverified, treat as hypothesis:** that the *game-service* message set is identical between vanilla DS2 and SOTFS — see the warning box above. Nothing past auth has been exercised on PS3. The message-set deltas most at risk are (a) the `PlayerInfoUploadConfigPushMessage` field mask, which is edition-specific data rather than protocol, (b) `MatchingParameter.calibration_version`, and (c) anything DS3OS marks "never seen in the wild".
- **Out of reach under emulation:** `sceNpMatching2` is stubbed in RPCS3, so the P2P leg after any successful brokering cannot be exercised there — and neither can the UDP ciphers until a PS3 client reaches the game service.

---

## 7. What I could not determine

Explicitly, so nobody mistakes silence for completeness:

1. **DS2 bell (`0x03EE`) payload.** `RequestNotifyRingBell`, its response, and `PushRequestNotifyRingBell` are all empty `// TODO` stubs (DS2P:889-900), and no DS2 manager handles the opcode. The DS3 shapes (`online_area_id, bytes data`) are a plausible starting guess but are **not** evidence for DS2.
2. **DS2 arena progression.** DS2 has no `SendQuickMatchStart` / `SendQuickMatchResult`. Whether DS2 reports arena results to the server at all, and through what, is unknown.
3. **Most DS2 logging payloads.** Nine of eleven `RequestNotify*` messages are un-reversed `field_N` placeholders (DS2P:330-437). The observed-value comments are the only semantics available.
4. **The `player_struct` blob** carried by `RequestCreateSign`, `RequestSummonSign`, `RequestVisit`, `PushRequestAllowBreakInTarget`, etc. is opaque to DS3OS — it only length-validates the NRSSR entry list (`DS2_NRSSRSanitizer.h`), never interprets it. Its internal layout is unknown.
5. **Character save blobs** (`character_data`) and **ghost/bloodstain `data`** are equally opaque; stored and echoed verbatim.
6. **`PowerStoneRankingData.data`** — probably a serialized `PowerStoneRankingDataPack`, but that message has no fields declared (DS2P:949). **(inferred, unverified)**
7. **DS2 `RequestWaitForUserLogin` fields 2-7** — five required + two optional uint32s with observed values `1, 0, 1, 2`. One is "probably the profile index" per the proto comment; which one is unknown, and the DS2-only optional 6/7 are entirely unexplained.
8. **`MatchingParameter` unknowns** — DS2 fields 4, 7, 9, 10 (candidates: NAT type, region) and the absent field 5.
9. **Regulation-file messages** — no opcode registered in either game; all bodies un-reversed.
10. **DS3 `RequestNotifyBreakInResult` (`0x03D7`)**, `RequestMeasure*Bandwidth`, `RequestBenchmarkThroughput`, `RequestGetOnlineShopItemList`, and all four Mark messages: field indices and wire types only, no semantics.
11. **Push aliasing in DS3.** `.inc:109-114` and `:159-164` note that several DS3 push ids have 4-8 equivalent alternate values in the client. DS3OS picks one arbitrarily. Why the client accepts multiple, and whether the choice matters, is unknown. **DS2 shows no such aliasing**, but nobody has checked exhaustively.
12. **⚠️ Whether PS3/vanilla-DS2 uses the same opcode numbering as PC/SOTFS at all.** **This is the single largest open risk in this document; no game-service byte has ever been observed from a PS3 client.** See the warning box at the top of section 6.
