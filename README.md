# dso

A modern, clean-room game server for FromSoftware's "Frpg2" online titles
(Dark Souls 2 and Dark Souls 3), written in Go and designed to run easily in a
container. A single binary hosts any supported game, selected by an environment
toggle (`DSO_GAME`).

The protocol/packet definitions are derived from community reverse-engineering
work; only wire-format knowledge is reused here — no third-party server code is
incorporated.

## Status

Early development. Milestone 1 is a successful **Dark Souls 2 (PS3)** logon:
a real client completing the login and authentication handshake against this
server, tested first under RPCS3.

Implemented so far:

- `internal/crypto/cwc` — AES-CWC-128 authenticated encryption (validated against
  the published CWC known-answer test vectors).

## Architecture

Two independent axes of pluggability:

- **Protocol family** — the transport/crypto/framing generation. `frpg2` covers
  Dark Souls 2/3; an older `frpg` family (Demon's Souls, Dark Souls 1) can slot in
  later behind the same seam.
- **Game** — version gate, message dispatch table, and feature set. Games register
  themselves and are chosen at boot from `DSO_GAME`.

## Building

Requires Go 1.26+.

```sh
go test ./...
```
