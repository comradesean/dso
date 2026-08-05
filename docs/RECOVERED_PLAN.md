# Plan: Modern Go Server for Dark Souls 3 / Dark Souls 2 (clean-room, packet defs from ds3os)

## Status
READY FOR REVIEW.

## Context
The user wants a new game server implementation for Dark Souls 3 and Dark Souls 2 (SOTFS). The reference at `ref/ds3so/ds3os` (DS3OS, C++/CMake) is to be used **only for packet definitions** (its `Protobuf/` directory and wire-protocol knowledge) — not as a codebase to port. The new server should be built on modern 2026 standards and be easily portable via Docker.

## Decisions (confirmed with user)
- **Language:** Go
- **Scope:** Core online play — auth/login handshake, blood messages, bloodstains, ghosts, summoning/invasions, matchmaking. Leaderboards/telemetry stubbed initially.
- **Games:** DS2 and DS3 now (both **Frpg2** protocol), with the architecture designed to expand to **Demon's Souls** and **Dark Souls 1 (PtDE)** later. Shared abstractions from day one.
- **Game selection = env toggle.** A single binary/Docker image; an environment variable (e.g. `DSO_GAME=DarkSouls2|DarkSouls3|DemonsSouls|DarkSouls1`) selects which game runs. Mirrors the reference's `GameType` config key but env-first for containers (still overridable via config file).
- **Two protocol generations (key architectural point):** DS2/DS3 use FROM's **Frpg2** protocol (what the reference documents). **Demon's Souls and Dark Souls 1 use the older Frpg (v1) protocol** — different framing and crypto, and we have **no reference for them** in-repo. So the pluggable boundary is not just feature managers but the **entire protocol/transport family**. Each game registers: its protocol family (Frpg2 vs Frpg), its message dispatch table, and its own feature set. DeS/DS1 are "design the seams, implement later"; only DS2 (PS3) is built for M1.
- **Milestone 1 (explicit user priority):** A successful **Dark Souls 2 logon on PS3** — a real DS2 PS3 client completes login → auth against the new Go server — before any other feature work begins. **PS3 is the primary M1 target; PC/Steam DS2 and DS3 follow later.**
- **Test path:** **RPCS3 (PS3 emulator) first**, then a real jailbroken PS3. Acceptance gate is a real DS2 client (running under RPCS3) completing login+auth — not a synthetic emulator. (We may still build a Go client-emulator as an internal dev/CI accelerator, but it is not the milestone gate.)
- **Auth:** **no-op / pluggable validator first.** PS3 uses **PSN NP tickets, not Steam** — the validator accepts the ticket and extracts identity. Real validation is a later swap-in.

### PS3-specific constraints and unknowns (important)
- The reference DS3OS is **PC/Steam-only**; its DS2 packet defs were reverse-engineered from the Steam build (app 335300). We assume PS3 ≈ PC DS2 protocol and **correct against RPCS3 packet captures** as differences surface. Unknowns to discover from captures: PS3 **app_version** (differs from Steam's 17039619 — make the version gate configurable/lenient during bring-up), NP-ticket format, any opcode deltas.
- **Client redirection (the former blocker, now tractable):** we generate **our own RSA keypair** and must patch the DS2 client's embedded **server hostname** and **FROM's RSA public key** to point at our server and trust our key. On PC this is the loader/injector (`Source/Loader/Utils/PatchingUtils.cs` builds a ~520-byte ServerInfo blob: key at offset 0, hostname at offset 432 ≤85 bytes — a direct guide to the embedded blob format; `Source/Injector/Hooks/DarkSouls2/DS2_ReplaceServerAddressHook.cpp` shows the DS2 address mechanism). On PS3 the analog is an **RPCS3 `patch.yml` PPU patch** (and an EBOOT patch for the real console) that overwrites the same hostname + public-key bytes — requires locating those offsets in the PS3 EBOOT.
- RPCS3 also gives us easy traffic capture (Wireshark / logging) to verify the protocol and iterate quickly.

## Findings (from exploration)

### Architecture of the reference (for orientation, not porting)
- One server binary hosts 4 services: **LoginService** (TCP 50050), **AuthService** (TCP 50000, Steam ticket auth), **GameService** (UDP 50010), **WebUIService** (HTTP 50005). A `ServerManager` supports multi-shard.
- Game logic is organized as **managers** registered per game (`DS3_Game` / `DS2_Game`): Boot, PlayerData, BloodMessage, Bloodstain, Ghost, Sign, BreakIn (invasions), Visitor (covenant auto-summon), QuickMatch, Ranking, Logging, Misc; DS2 adds MirrorKnight, DS3 adds Mark/AntiCheat/bell-ringing.
- Persistence: SQLite, one schema for both games (Players, BloodMessages, Bloodstains, Ghosts, Rankings, Characters, Statistics, MatchingSamples, Bans, AntiCheat*). Bloodstains/ghosts default to memory-cache only.
- Config: single `config.json` (`Source/Server/Config/RuntimeConfig.h`); `GameType` key switches DS3/DS2. DS2 matchmaking is **soul-memory-tier** based vs DS3's soul-level+weapon-level.
- Build-time constants (`Source/Server/Config/BuildConfig.h`): DS2 SOTFS app version 17039619, Steam AppID 335300; DS3 114-116 / 374320.
- Client connects via the DS3OS **Loader/Injector** (Windows) which injects a DLL to redirect hostname/port and supply the server's RSA public key. Discovery optionally via a Node master server (in-memory list, heartbeat POST). We can reuse the existing loader by mimicking the server side.
- Reference Docker: 3-stage build, needs `steamclient.so` from steamcmd (Steam ticket validation), runs with `--net host`, bind-mounts `Saved/`.
- DS2-specific code in reference: `Source/Server.DarkSouls2/` (`DS2_Game`, `DS2_Frpg2ReliableUdpMessage*`, `DS2_CellAndAreaId.h`, `Protobuf/Generated/DS2_*`).

### Protocol layer (the part we take from the reference)
**Protobuf definitions** at `ref/ds3so/ds3os/Protobuf/` — proto2, LITE_RUNTIME, ~503 messages:
- `Shared/Shared_Frpg2RequestMessage.proto` — the login/auth handshake messages (short file; both games).
- `DarkSouls2/DS2_Frpg2RequestMessage.proto` (211 msgs) + `DS2_Frpg2PlayerData.proto` — DS2 in-game messages.
- `DarkSouls3/DS3_*.proto` — DS3 equivalents + telemetry (`DS3_FpdLogMessage.proto`).

**TCP framing (login/auth)**: `uint16` BE length prefix → 12-byte packet header (send counter, payload len) → 12-byte message header (`header_size=12`, `msg_type`, `msg_index` — msg_index little-endian, rest big-endian) → protobuf payload. Replies add a 16-byte response header. Login/auth msg_type space: Reply=0, KeyMaterial=1, GetServiceStatus=2, SteamTicket=3, RequestQueryLoginServerInfo=5, RequestHandshake=6. (Ref: `Source/Server/Server/Streams/Frpg2PacketStream.*`, `Frpg2Message.h`.)

**Handshake flow (what Milestone 1 must implement)**:
1. **Login TCP :50050** — whole stream RSA (client→server PKCS1-OAEP, server→client X9.31, server keypair). Client: `RequestQueryLoginServerInfo{steam_id, app_version}` → server: `{server_ip, port}` of auth server.
2. **Auth TCP :50000** — RSA `RequestHandshake{aes_cwc_key}` (client-chosen 16-byte key) → server sends unencrypted 27-byte blob (11 random + 16 zero) → switch to AES-CWC-128 → `GetServiceStatus` (version gate: DS2 SOTFS app_version 17039619) → `KeyMaterial`: client 8 bytes + server 8 random bytes = 16-byte game CWC key → `SteamTicket`: [16-byte key echo | steam session ticket] → server replies raw 184-byte `Frpg2GameServerInfo` struct (not protobuf: uint64 auth_token, ip[16], 112 pad, uint16 port, 11 tuning uint32s) and registers (auth_token→CWC key), 30s expiry. (Ref: `Source/Server/Server/AuthService/AuthClient.{h,cpp}`.)
3. **Game UDP :50010** — first 8 plaintext bytes of each datagram = auth token (outside the AEAD); server looks up the CWC key registered in step 2 and builds the reliable-UDP message stream. UDP AES-CWC layout: client→server `[8 auth_token][11 IV][16 tag][1 pkt_type][ciphertext]` (AAD = IV||auth_token||pkt_type, 20 B); server→client `[11 IV][16 tag][ciphertext]` (AAD = IV, 11 B). Above the crypto sits the 4-layer reliable-UDP stack (packet `magic 0x02F5`, SYN/RACK/DAT/HBT/FIN/ACK opcodes, 12-bit ack counters, `MAX_PACKETS_IN_FLIGHT=32`) → fragments (`MAX_FRAGMENT_LENGTH=900`, zlib deflate when payload ≥512) → messages (push=`msg_type 0x0320`/`msg_index 0xFFFFFFFF` with real type in protobuf field 1 `PushMessageId`; reply=`msg_type 0` w/ copied index; request=incrementing counter). DS2 opcode dispatch table: `Source/Server.DarkSouls2/Server/Streams/DS2_Frpg2ReliableUdpMessageTypes.inc`.

**Best cross-reference for building/testing:** `Source/Server/Client/Client.cpp` is ds3os's own client emulator — it walks the entire client side of the handshake and is the single best guide for a Go test harness to validate Milestone 1. Verified directly against source.

**DS2 constants (verified in `BuildConfig.h`):** `MIN_APP_VERSION = APP_VERSION = 17039619`, `STEAM_APPID = 335300`, `AUTH_ENABLED = true` (Steam ticket validation is on by default).

**Definition of "successful DS2 logon" (the boot sequence after UDP is Established, from `Client.cpp` + DS2 dispatch table):**
1. Reliable-UDP stream reaches `Established`.
2. `RequestWaitForUserLogin` (opcode `0x0386`) → server returns a `player_id`. **This is the minimum bar** — the client now considers itself authenticated to the game server.
3. `RequestGetAnnounceMessageList` (DS2 opcode `0x03EC`) → announcements (can be empty).
4. `RequestUpdateLoginPlayerCharacter` (`0x03B6`) → returns a server character id.
5. `RequestUpdatePlayerStatus` (`0x03B8`) → uploads the player status blob.
Reaching step 5 is a clean "player is online at the bonfire" state. Milestone 1 targets steps 1-5 for a real DS2 SOTFS client (or our Go client emulator as a stand-in).

## Approach

### Guiding principles
- **Clean-room:** only the reference's `.proto` files and the protocol facts above are reused. No C++ is ported; the CWC algorithm and X9.31 padding are re-implemented in pure Go from their specs (and validated against the reference's own golden vectors).
- **Pure Go, `CGO_ENABLED=0` everywhere** → tiny static distroless image. No OpenSSL, no Steamworks, no zlib-C. (The only C touched is an optional, build-tagged differential-fuzz *test* harness that never ships.)
- **Layered so games and protocol generations are pluggable:** a shared `frpg2` core (framing, crypto, reliable-UDP) serves DS2+DS3; a future `frpg` package is where Demon's Souls / DS1 slot in. A **game registry** + env toggle (`DSO_GAME`) selects the active game at boot. Each game binds {protocol family, message dispatch table, feature managers}.

### Go module layout (`github.com/sstreight/dso`, go 1.25)
```
cmd/            dsoserver (server) · dsoclient (emulator) · dsotool (keygen/rpcs3-patch/decode/healthcheck)
proto/          vendored UNMODIFIED reference .proto (shared + ds2 + ds3); buf managed-mode codegen
internal/
  config/       koanf: defaults→file→DSO_* env→flags; validation + startup banner
  logging/      slog; hexdump; per-connection loggers; unknown-message dumper
  crypto/
    cwc/        pure-Go AES-CWC-128 AEAD primitive (+ golden vectors)  ← top risk, build first
    x931/       X9.31 pad/unpad + raw RSA private/public ops (CRT)
    keys/       RSA-2048 PKCS#1 PEM generate/load/save
    frpgcipher/ Cipher iface: RSA(server/client), CWC-TCP, CWC-UDP(client/server)
  frpg/         packet/ · message/ (cipher-swappable TCP stream) · rudp/ (4-layer reliable UDP) · zlibx/
  server/       core/ (Service iface, lifecycle) · login/ · auth/ (state machine + 184B GameServerInfo) · game/ (UDP demux, token registry, Manager iface)
  games/        game.go (Game iface) · ds2/ (msgtypes, boot, playerdata, state) · ds3/ (compile-only stub)
  identity/     Validator iface: noop | psn | (steam later)
  store/        Store iface + memory impl (sqlite later via modernc.org/sqlite, cgo-free)
  client/       emulator as a library (so e2e tests drive it)
  proto/        GENERATED (committed): sharedpb/ ds2pb/ ds3pb/
test/e2e/       in-process server + emulator, per-stage asserts
tools/rpcs3/    patch.yml template + EBOOT-search README
```
Key design choices: **one UDP socket** with `map[[8]byte]*client` demux (replies via `WriteToUDPAddrPort` preserve source port behind NAT); reliable-UDP modeled as one composed `Session` (not the reference's 4-level inheritance); **`auth_token` handled as `[8]byte`** never uint64 (correctness on big-endian PS3); proto2 `required` fields forced-set on our responses and, on inbound parse failure, dumped + field-walked with `protowire`.

### Libraries
protobuf `google.golang.org/protobuf` + `buf` (managed mode injects `go_package`, protos stay pristine) · logging `log/slog` (+`tint` dev only) · config `koanf/v2` · SQLite later `modernc.org/sqlite` (cgo-free) · AES/zlib/subtle stdlib · tests `testing`+`go-cmp/protocmp`.

### The four hard problems (condensed; full detail in the findings above)
1. **AES-CWC-128 AEAD** (not in stdlib) — hand-port Gladman `cwc.c` integer path (~250 LOC): AES-CTR (counter at byte 12, pre-incremented) + 127-bit GF(2¹²⁷−1) polynomial hash over 12-byte blocks, AAD absorbed first zero-padded. **Validate against `Source/ThirdParty/aes_modes/testvals/cwc.1` (verified present, 15 vectors) before writing any networking.** `[2]uint64` accumulator via `math/bits`; constant-time tag compare.
2. **RSA X9.31 encryption** (not in stdlib) — ~80 LOC: pad `0x6B ‖ 0xBB* ‖ 0xBA ‖ msg ‖ 0xCC` (or `0x6A ‖ msg ‖ 0xCC`), raw `m^d mod n` via CRT (`rsa.PrivateKey.Precomputed`), then `s=min(s,n−s)`. Public op mirrors for tests/emulator. OAEP (inbound) is stdlib SHA-1. Keys RSA-2048 PKCS#1 PEM; `public_exponent` configurable (65537 default; e=3 fallback if the PS3 client requires it). Golden-file tests vs OpenSSL. **Used exactly once per session** (login response), so surface is tiny.
3. **Protobuf** — copy 3 protos verbatim (`Shared_Frpg2RequestMessage`, `DS2_Frpg2RequestMessage`, `DS2_Frpg2PlayerData`); buf managed mode; **commit generated code**; CI `make proto && git diff --exit-code`. Inbound parse failures dump raw bytes + `protowire` field-walk (our PS3 protocol-archaeology tool).
4. **zlib** — M1: **outbound compression OFF**, inbound ON (stdlib inflate ignores window size). Later outbound: hand-write `58 C3` header + raw `flate` level 7 + Adler-32, gated to 512–8192 bytes (8 KiB window).

### Auth (pluggable, PSN)
`identity.Validator`: **`noop` default** (accept any ticket, identity = the `steam_id`/PSN-id string already sent, log first ticket seen) → `psn` (parse NP-ticket TLV for stable online-id, no signature check) → `steam` later (Web API, no CGO). Insecure modes gated behind private-bind or `DSO_ALLOW_INSECURE_AUTH=1` + loud startup WARN.

### PS3 / RPCS3 workstream (parallel, off the Go critical path)
- **Hostname:** RPCS3 *Network → IP/Hosts switch* maps the login hostname → server LAN IP (no binary edit). Real PS3: LAN DNS override.
- **RSA public key (must patch):** generate our keypair; `dsotool rpcs3-patch` emits a `patch.yml` (keyed by RPCS3's `PPU-<hash>`) that overwrites FROM's **plaintext PEM** (verified: DS2 stores it as a searchable string, no TEA) with ours — equal-or-shorter overwrite. Real PS3: patch+re-sign EBOOT (deferred).
- **Spikes (day 1):** (a) locate hostname + PEM offsets in the decrypted PS3 EBOOT (`strings`/`grep -abo`, map to VAs via `readelf -l`); (b) confirm RPCS3 can mint an NP ticket (`RPCN`→`Simulated`) — if not, M1 gate degrades to "passes GetServiceStatus" and we go to real hardware sooner.
- **Bring-up leniency:** `enforce_app_version:false` (PS3 version unknown — log the real value), downgrade length checks to warnings, never disconnect on unhandled messages.

### M1 build order & checkpoints
| Phase | Deliverable | Checkpoint |
|---|---|---|
| 0 Scaffolding | go.mod, buf, logging, config | — |
| 1 Crypto | cwc, x931, keys, frpgcipher | **CP0: `go test ./internal/crypto/...` green incl. cwc.1 vectors — do not proceed until green** |
| 2 TCP framing | packet + message streams | round-trip + golden fixtures |
| 3 Core + login | Service lifecycle, LoginService | **CP1: emulator gets auth-server addr (proves OAEP-in/X931-out/framing)** |
| 4 Auth | state machine, 184B GameServerInfo, token registry, identity | **CP2 = M1 HARD GATE: RPCS3 client receives GameServerInfo** |
| 5 Reliable UDP | opcode/packet/fragment/message/session | CP3: loopback + emulator reaches Established |
| 6 Game + DS2 boot | Store, Game/Manager ifaces, ds2 boot+playerdata, ds3 stub | CP4 (stretch): `player_id` returned |
| 7 Entrypoints/emulator/tooling/docker | dsoserver/dsoclient/dsotool, e2e test, Dockerfile, patch.yml | CP5: containerised; CP6/7: RPCS3 through auth |

### Docker
Multi-stage `golang:1.25` → `distroless/static-debian12:nonroot`; `CGO_ENABLED=0 -trimpath`; `EXPOSE 50050/tcp 50000/tcp 50010/udp`; `VOLUME /data` (keys/config/db/dumps); exec-form `HEALTHCHECK` via `dsotool healthcheck` (TCP-dials login+auth). `advertise_address` is **explicit config, never auto-detected** (so bridge networking works); provide a `docker-compose.host.yml` (`network_mode: host`) for Linux console testing. **WSL2 caveat:** for RPCS3 on the same desktop, `go run ./cmd/dsoserver` natively is the least-friction path; Docker is for CI/deploy.

## Verification

Prove M1 end-to-end, offline first (CP0→CP4 are pure Go), then against a real client:

```bash
cd /mnt/f/ClaudeHole/dso
make proto && go vet ./... && go test ./... -race          # all green
go test ./internal/crypto/cwc -run TestVectors -v          # CP0: the critical gate

# keys (public PEM is what we patch into the client)
go run ./cmd/dsotool keygen --out ./data/keys --bits 2048

# server + emulator (two shells) — the automated M1 proof
DSO_SERVER_ADVERTISE_ADDRESS=<LAN_IP> DSO_LOGGING_LEVEL=debug go run ./cmd/dsoserver --config=data/config/server.yaml
go run ./cmd/dsoclient --server=<LAN_IP> --public-key=./data/keys/server.public.pem --stage=full -v
#   expect: [login] auth addr → [auth] GameServerInfo token=… → [game] established → player_id=1
go test ./test/e2e -run TestDS2Logon -v                    # what CI runs

# real DS2 under RPCS3 (the milestone gate)
go run ./cmd/dsotool rpcs3-patch --key ./data/keys/server.public.pem --ppu-hash PPU-xxxx --key-addr 0x00xxxxxx > ~/.config/rpcs3/patches/dso.yml
# RPCS3 Network: Connected + RPCN(or Simulated) + IP/Hosts switch login-host=<LAN_IP>; boot DS2, go online
# server log must show: login RequestQueryLoginServerInfo (RECORD app_version) → auth handshake → auth complete
# on any failure: go run ./cmd/dsotool decode data/dumps/auth/0x0003/0.bin
```
**M1 is complete when a real DS2 client under RPCS3 completes login + auth and receives a well-formed `Frpg2GameServerInfo`.** Reaching the UDP boot `player_id` is the stretch goal.

## Top risks
1. **AES-CWC port correctness** — sits under every layer; a bug is unattributable. Mitigation: build+validate `cwc` first against the vendored `cwc.1` vectors (verified present); do not start networking until CP0 is green.
2. **PS3 ≠ PC SOTFS protocol** (reference is PC vanilla-vs-SOTFS mismatch) — different app_version/opcodes/ticket possible. Mitigation: lenient version gate, dump+field-walk every unparsed message from Phase 0, never disconnect on unhandled, and note the M1 message set is 5 tiny protos all in the *shared* file (most likely identical across platforms).
3. **PS3 client redirection** (EBOOT offsets / patch.yml size / NP-ticket availability under RPCS3) — run the two spikes in parallel from day 1. Strong prior: DS2 stores hostname+PEM as plaintext (verified), so it's a straight overwrite; RPCS3 IP/Hosts switch removes the hostname patch entirely.

## Later scope (seams designed, not built in M1)
New game features = one `game.Manager` each (blood messages, bloodstains, ghosts, signs, invasions, quick match, ranking — one file per manager). DS3 = fill `internal/games/ds3/` (note: DS3 client redirection needs a TEA codec DS2 doesn't). Demon's Souls / DS1 = a new `frpg` (v1) protocol-family package behind the same game-registry seam. Persistence = `store.Store` on `modernc.org/sqlite`. Real auth, sharding, master server, WebUI = new `Validator` / `core.Service` implementations.
