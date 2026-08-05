# Kaitai Struct definitions for the Frpg2 wire formats — PC-reference derived

Machine-readable specs for the binary framing Dark Souls 2/3 clients speak to their
servers. Written 2026-08-05.

## Provenance — read this first

Everything in this directory is derived from the **DS3OS** C++ reference implementation
(`ref/ds3so`, gitignored) plus our own Go implementation — both of which target
**PC / Scholar of the First Sin**. Hence `pc/`.

Our actual target is **PS3 / original Dark Souls 2 (BLUS41045)**, which differs on two
axes at once: platform *and* edition. A sibling `dev/proto/ps3/` should be populated from
**decompilation of the PS3 EBOOT**, and where the two disagree, `ps3/` wins.

The login and auth specs here happen to be independently **byte-verified against a real
PS3 client**, so they are trustworthy for both platforms. The game-service specs are not
verified anywhere — see the verification table below.

## Layout

```
dev/proto/pc/
  incoming/    client -> server   (what the server parses)
  outgoing/    server -> client   (what the server emits)
```

Each direction has one file per protocol:

| Protocol | Transport | Port (PS3 / PC) | Files |
|---|---|---|---|
| Login | TCP, RSA | 50011 / 50050 | `frpg2_login_in.ksy`, `frpg2_login_out.ksy` |
| Auth | TCP, RSA then AES-CWC | 50000 | `frpg2_auth_in.ksy`, `frpg2_auth_out.ksy` |
| Game | UDP, AES-CWC + reliable-UDP | 50010 | `frpg2_game_in.ksy`, `frpg2_game_out.ksy` |

The split is by direction because the two directions are genuinely **not symmetric**:

- Login/auth use different RSA paddings per direction (OAEP inbound, X9.31 outbound).
- The auth reply to `request_handshake` is sent **in the clear** while the request is encrypted.
- The game UDP datagram carries an 8-byte auth token and a packet-type byte **only**
  client->server; the server->client datagram has neither.
- The reliable-UDP connection prefix (35 bytes) is only ever sent by the client.

## What these cover, and what they don't

These describe **framing**: length prefixes, headers, cipher envelopes, the reliable-UDP
packet/fragment/message layers, and the handful of raw fixed-size structs (the handshake
blob, key material, and `Frpg2GameServerInfo`).

They deliberately stop at the **protobuf** boundary. Message bodies above the framing are
protobuf, which Kaitai does not parse; use the `.proto` definitions and
`docs/protocol-map.md` for those. Where a body is protobuf the spec exposes it as an opaque
byte slice and names the expected message in a `doc`.

Encrypted regions are likewise opaque — Kaitai cannot decrypt. Each spec marks the exact
byte ranges that are ciphertext and documents the cipher, so a decode is a two-pass job:
parse the envelope here, decrypt, then re-parse the plaintext with the inner type.

## Verification status

| Spec | Status |
|---|---|
| `frpg2_login_in` / `frpg2_login_out` | **Byte-verified** against a real PS3 client, 2026-08-04 |
| `frpg2_auth_in` / `frpg2_auth_out` | **Byte-verified** — full four-stage handshake, 2026-08-05 |
| `game_server_info` (in `frpg2_auth_out`) | **Known wrong on PS3.** The client reads it and refuses to open the UDP session. Decomp underway. |
| `frpg2_game_in` / `frpg2_game_out` | **Unverified anywhere.** No game-service byte has ever been seen from a PS3 client. |

The gap matters: the auth *framing* is proven, but the 184-byte `Frpg2GameServerInfo`
struct carried inside the final auth reply is reference-derived and is currently being
rejected. That single struct is why the game-service specs have never been exercised.
Anything below the auth layer should be read as a hypothesis to test, not as fact.

Byte-order note worth flagging, since it is easy to get wrong: nearly everything is
big-endian, but `msg_index` is **little-endian** in both the TCP and UDP message headers, the
reliable-UDP fragment `packet_counter` is little-endian, and the UDP message response header
is little-endian where the TCP one is big-endian.

## Using them

```sh
ksv <capture.bin> dev/proto/pc/incoming/frpg2_auth_in.ksy      # visualise
kaitai-struct-compiler -t python dev/proto/pc/incoming/*.ksy   # generate a parser
```
