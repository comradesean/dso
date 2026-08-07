# Reading FromSoftware's live DS2 servers

**Why this exists.** Several questions cannot be answered from the PS3 client or from our own server,
because they are about what *FromSoftware's* server did: did it filter belfry-bell pushes by location
(`tasks/bell-broadcast.md`), did it ever send `0x038B` (`tasks/regulation-push-038b.md`), what did a
real weekly event rotation look like. DS2 SOTFS's PC servers are still live, so the traffic exists.

**Status: a passive capture alone can never work. With a client-side key it will.** The key's
location is found and the verification recipe is settled; nothing has been run against a live game
yet.

---

## Why passive capture alone fails (CONFIRMED, 2026-08-06)

680 MB captured against the live servers. Two independent blockers.

**The key is never on the wire in recoverable form.** `RequestHandshake` carries the 16-byte CWC key
**client->server under RSA-OAEP** to FromSoftware's public key (2048-bit, e=3, embedded in the exe).
Only their private key reverses it, and `e=3` buys nothing because OAEP is randomised and fills the
modulus. Both directions then switch to CWC *before* the game key (`client8 ‖ server8`) is derived,
so it never appears in the clear.

The one asymmetric direction, server->client login using X9.31, **is** reversible with the embedded
public key — but it carries only the auth server's address. No key material.

**That capture also missed the handshake entirely.** A 369-second gap between capture files swallowed
login, auth and the RUDP SYN. Confirmed two ways: zero Frpg2 packet-header signatures across 441,902
TCP segments with payload, and the first captured game datagram is mid-session (`packet_type = 0x00`,
no connection prefix). **Start the capture before launching the game.**

### What a capture does give you

The envelope is cleartext and must be — the receiver needs the IV and tag before it can do anything:

```
client->server   token(8) ‖ iv(11) ‖ tag(16) ‖ ptype(1) ‖ ciphertext
server->client   iv(11) ‖ tag(16) ‖ ciphertext
```

Verified: all 4,773 client->server datagrams in that capture carry the same cleartext auth token, and
the framing matches `dev/proto/pc/**/frpg2_game_*.ksy` exactly. Counts, sizes, timing and direction
are readable. Contents are not: 2.4 MB at 7.9999 bits/byte, no `F5 02` magic anywhere.

**The PC game service is on UDP `:50000`** — not `:50010` (PS3) or `:50031` (the TCP control
channels).

---

## The key, and how to get it (CONFIRMED from disassembly)

`DarkSoulsII.exe`, 28,200,992 bytes, PE x86-64, **ImageBase `0x140000000`**. SteamStub-wrapped, but
`.text` is plaintext on disk, and the build matches the one ds3os targeted.

**Every CWC key the client installs passes through one function:**

| | |
|---|---|
| `cwc_init_and_key` | VA `0x140260D90` · RVA `0x260D90` · file `0x260190` |
| entry contract (MSVC x64) | `RCX` = key pointer, `RDX` = key length (16), `R8` = `cwc_ctx*` |
| what to read | 16 bytes at `RCX` |

Identified as Brian Gladman's `cwc_init_and_key` from `aes_modes/cwc.c` — the same code our
`internal/crypto/cwc/` descends from — line for line:

```
140260da7: 8d42f0            lea  eax,[rdx-0x10]        ; key_len-16
140260db2: a9e7ffffff        test eax,0xffffffe7        ; accept only 16/24/32
140260dbd: 83fa28            cmp  edx,0x28              ; reject 40
140260dc8: 41b860010000      mov  r8d,0x160             ; memset(ctx,0,352) = sizeof cwc_ctx
140260ddb: 4c8d4630          lea  r8,[rsi+0x30]         ; ctx->enc_ctx
140260de4: call 0x1406b9d10                            ; aes_encrypt_key
140260dfe: c64424 20 c0      mov  byte[rsp+0x20],0xC0   ; zv[0]=0xC0
140260e08: call 0x1406babd0                            ; aes_encrypt
140260e0d: 806424207f        and  byte[rsp+0x20],0x7F   ; zv[0]&=0x7F
```

`hdr_cnt`/`txt_ccnt`/`txt_acnt` land at `ctx+0x154/+0x158/+0x15C`, matching the `+340/344/348` the
ds3os cheat tables read. Neighbours: `aes_encrypt_key` `0x1406B9D10`, `aes_encrypt` `0x1406BABD0`.

### Runtime signature (survives ASLR)

`test eax,0xFFFFFFE7` = `A9 E7 FF FF FF` occurs **exactly once in the whole 28 MB image**, at
function entry `+0x22`.

```
scan for   A9 E7 FF FF FF
subtract   0x22           -> function entry
breakpoint there, read 16 bytes at [RCX]
```

Tolerant mask, also unique: `8D 42 F0 ?? ?? ?? ?? ?? ?? ?? ?? A9 E7 FF FF FF 0F 85 ?? ?? ?? ?? 83 FA 28`

### Callers (CONFIRMED)

1. `0x140CDB7C9` — inside `Nauru::Security::CWCObject`'s keying method (`0x140CDB760`). RTTI string
   `.?AVCWCObject@Security@Nauru@@` at VA `0x1415E6100`; vtable `0x14120F878`. Its
   Encrypt/Decrypt/AuthHeader/ComputeTag wrappers call `0x140260D10 / 0x140260D50 / 0x1402606F0 /
   0x140260880`.
2. `0x14026045C` and 3. `0x1402604CF` — Frpg2 wire-cipher wrappers, both hardcoding key length 16,
   ctx at `struct+8`. Which is auth-TCP and which is game-UDP is **INFERRED**, not traced.

**Secondary anchor (INFERRED but strong):** the wrapper at `0x14026045C` stores the 16-byte key
**XOR `0xAC`** at `struct+0x168` right after keying (loop at `0x140260480`). Useful if you have the
cipher object but not a breakpoint. The choke point is simpler and universal — prefer it.

Two keys flow through: the auth-stream key first, then the **game-service** key (`client8 ‖ server8`)
when the UDP connection comes up. Log every `(RCX[0:16], R8)` pair and pick by verification.

---

## Verifying a candidate key

Do not trust an unverified key. `internal/crypto/frpgcipher/cwc_udp.go` already encodes the framing:

| direction | layout | AAD |
|---|---|---|
| server->client | `IV(11) ‖ tag(16) ‖ ct` | IV |
| client->server | `token(8) ‖ IV(11) ‖ tag(16) ‖ ptype(1) ‖ ct` | IV ‖ token ‖ ptype |

Feed key + one captured datagram to `cwc.Context.DecryptMessage`. If the tag verifies, the key is
real and you have plaintext.

---

## Procedure

1. **Start `dumpcap` with a ring buffer BEFORE launching the game.** The handshake is what gets lost
   otherwise, and it is not recoverable afterwards.
2. Launch the game, then run `keydump.exe` **as Administrator** (it needs `DebugActiveProcess`).
   Start it before going online — the keys are installed during the login handshake.
3. Go online. Keys print as they appear; expect at least two (auth stream, then game service).
4. Play. Ring bells, sit somewhere far from a belfry, let it run.
5. Pull one game datagram's **UDP payload** out of the capture and run `verifykey` against the
   dumped keys. The one that verifies is the game key.
6. Decode the capture with that key.

## Tooling — built, tested, in the repo

**`cmd/keydump`** (Windows only; cross-compile with
`GOOS=windows GOARCH=amd64 go build -o keydump.exe ./cmd/keydump`)

```
keydump.exe                  attach to DarkSoulsII.exe, print keys as installed
keydump.exe -pid 1234        attach to a specific process
keydump.exe -out keys.txt    also append to a file
```

Finds the function by **signature, not address**, so ASLR and a different build do not matter. It
reads the PE header for `SizeOfImage` rather than guessing a scan range, and **warns if the
signature matches more than once** — the whole approach rests on that uniqueness, so a second match
is something to know rather than to silently take the first of. It writes exactly one byte (the
INT3) and restores it on every hit; nothing else in the process is touched.

**`cmd/verifykey`**

```
verifykey <keyhex> <datagramhex> [s2c|c2s]
verifykey -keys keys.txt -datagram <hex> [s2c|c2s]
```

The second form takes keydump's own output file and reports which key works. Prints the plaintext
and flags whether the RUDP magic `F5 02` is present as an independent confirmation.

Two bugs were found and fixed while testing this against known-good sealed datagrams, both of which
would have been debugged live with a correct key in hand:

- The ciphers are named by **role**, not direction — `ServerUDPCipher.Open` reads *client->server*
  datagrams. Mixing them up gives a tag failure indistinguishable from a wrong key.
- `Open` returns `(plaintext, connectionPrefix, error)`. That bool is **not** success; it marks the
  SYN datagram. Reading it as "ok" made a correct key report "tag failed" while returning correct
  plaintext.

`cmd/verifykey/main_test.go` pins both.

**Capture-side helpers** (scratchpad, not in the repo):
`scratchpad/pccap/pcapng.py` — dependency-free pcapng + Ethernet/IPv4/TCP/UDP dissector;
`scratchpad/pccap/decode_game.py` — finds Frpg2 flows, decodes the envelope, takes `--key`.

## Also recovered

- RSA public key: file `0x10D3940`, VA `0x1400D4B40`, 2048-bit, `e=3`
- Login host: `frpg2-steam64-ope-login.fromsoftware-game.net` (UTF-16) at file `0x10D38B0`

Neither is obfuscated; ds3os's DS2 injector finds both by literal string search.

## Not done

Nothing here has been run against a live game. The breakpoint contract is derived statically — a
standard MSVC x64 prologue with `RCX/RDX/R8` untouched until the saves at `0x140260DAF`, so `[RCX]`
at entry is unambiguous, but it is unobserved.

**Captures contain other people's traffic.** The 680 MB set holds ~145 resolved DNS names and a lot
of unrelated personal HTTPS. Do not publish raw captures.
