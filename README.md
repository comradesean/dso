# dso

A modern, clean-room game server for FromSoftware's "Frpg2" online titles
(Dark Souls 2 and Dark Souls 3), written in Go and designed to run easily in a
container. A single binary hosts any supported game, selected by an environment
toggle (`DSO_GAME`).

The protocol/packet definitions are derived from community reverse-engineering
work; only wire-format knowledge is reused here — no third-party server code is
incorporated.

## Status

**Two real Dark Souls 2 (PS3) clients play together against this server.** Login,
the four-stage auth handshake and the reliable-UDP session all work against
retail hardware under RPCS3, and every named multiplayer mode has an
implementation: blood messages, bloodstains, ghosts, summon signs, invasions,
Mirror Knight, covenant auto-summons and duelling arenas have all been completed
live between two players.

Beyond the protocol, the server can serve its own **calibration payloads** (the
game's regulation download) and push a **whole replacement resource into a
running client** over `0x038B` — no restart. That last one drives two things
FromSoftware ran from their own servers and nobody has been able to since 2014:

- the **weekly Majula event chest**, armed and paying out on a rotation
- the **Majula obelisk**, the stone that reads *"The letters are worn beyond
  recognition."* — it displays whatever we send it again. See
  [`docs/worn-writing.md`](docs/worn-writing.md).

`docs/STATUS.md` is the honest account of what is proven versus assumed, and what
each mistake cost. `docs/features.md` maps every opcode to what a player actually
does in game and is the best starting point for the protocol.

## Architecture

Two independent axes of pluggability:

- **Protocol family** — the transport/crypto/framing generation. `frpg2` covers
  Dark Souls 2/3; an older `frpg` family (Demon's Souls, Dark Souls 1) can slot in
  later behind the same seam.
- **Game** — version gate, message dispatch table, and feature set. Games register
  themselves and are chosen at boot from `DSO_GAME`.

## Building and running

Requires Go 1.26+.

```sh
go test ./...
go build -o dsoserver ./cmd/dsoserver
```

Copy `dso.env.example` to `dso.env` and edit it — every knob is documented there,
including why several of them exist. The one that must be right is
`DSO_SERVER_ADVERTISE_ADDRESS`: it is written into the login reply, so a wrong
value produces a client that authenticates perfectly and then never sends.

```sh
./dsoserver
```

On Windows, run it from WSL rather than natively — Windows cannot bind TCP on the
LAN address, and WSL shares the same IP.
