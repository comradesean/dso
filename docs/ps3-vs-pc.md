# DS2 PS3 vs. the ds3os reference (PC / SOTFS)

The reference in `ref/ds3so` targets **Dark Souls 2: Scholar of the First Sin on PC
(Steam app 335300)**. Our M1 target is **BLUS41045 — original Dark Souls 2 on PS3**. Two axes of
difference at once: platform (PS3/PSN vs PC/Steam) and edition (vanilla vs SOTFS).

This file records what we have actually observed, separated by evidence strength, so that
supporting both later is a matter of filling in a table rather than re-deriving everything.

Evidence levels used below:
- **Confirmed** — observed against a real BLUS41045 client under RPCS3, with bytes on record.
- **Reference** — documented by ds3os for PC; not yet checked on PS3.
- **Assumed** — inferred, not verified. Treat as a hypothesis.

---

## Confirmed SAME (do not "fix" these for PS3)

These were verified identical, and it matters: chasing imagined platform differences here cost
real time.

| Thing | Evidence |
|---|---|
| **AES-CWC-128 tag algorithm** | Building ds3os's `aes_modes/cwc.c` (PC-derived) and feeding it a captured PS3 exchange reproduces the console's tag **byte-for-byte**. The tag bug we hit was our own byte-order bug, *not* a platform difference. See `internal/crypto/cwc` and `TestConsoleCapture`. |
| **AES-CTR keystream** | Counter block `0x80 \|\| IV(11) \|\| be32 counter`, first data block uses counter 1. Ciphertext matched the console exactly even while our tag was wrong. |
| **TCP cipher framing** | `IV(11) \|\| tag(16) \|\| ciphertext`, with the IV also used as the authenticated header. Identical to `CWCCipher::Decrypt`. |
| **Handshake key-exchange blob** | 27 bytes = 11 random + 16 zero. ds3os calls this "not sure what's going on with this payload"; it is simply `IV length (11) + tag length (16)`. |
| **Login RSA public key** | The PS3 login key at `0x17FB338` is **byte-identical** to the key ds3os searches for and patches on PC (`DS2_ReplaceServerAddressHook.cpp`). Same key ships on both platforms. |
| **Auth stage sequence** | RequestHandshake → GetServiceStatus → KeyMaterial → ticket. Matches `AuthClient.cpp`'s state machine. |

---

## Confirmed DIFFERENT

| Thing | PC / SOTFS (reference) | PS3 / BLUS41045 (observed) |
|---|---|---|
| **`app_version`** | 17039619 (`0x1040103`) | **16912640 (`0x1020000`)** |
| **Identity** | Steam ID | **PSN online ID.** The proto field named `steam_id` carries it verbatim (observed: `comradesean`). The field is generic; only the naming is Steam-flavored. |
| **Ticket validation** | Steam `BeginAuthSession` | **PSN NP ticket.** Needs a different validator; the plan already scopes this as pluggable. |
| **Embedded RSA keys** | ds3os searches for one key string | **Two** distinct 2048-bit e=3 PEMs, 426 bytes each: `0x17FB338` (login, = PC's key) and `0x189AB48` (not the login key; patching it breaks the flow). |
| **Client redirection** | DLL injection; `memcpy` over the key found in process memory | **RPCS3 `patch.yml`** (`utf8` entries at both vaddrs) or an EBOOT patch on real hardware. See `tools/rpcs3/dso.yml`. |
| **Login port** | 50050 | **50011** |
| **Login hostname** | Steam-side host | `frpg2-ps3-ope.fromsoftware.jp` |
| **HTTP preflight** | not present in the reference | PS3 GETs `contents_0101.bin` from `frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com` before going online. Origin was still live as of 2026-08-04; a copy is in `data/`. Observed **non-blocking** — the client proceeded to login in a run where it never issued the fetch. |

---

## Open / unverified

- **Opcode and message-set deltas** between vanilla DS2 and SOTFS. The M1 message set is five
  small protos in the *shared* file, so most likely identical, but nothing past auth has been
  exercised on PS3.
- **UDP ciphers** (`CWCClientUDPCipher` / `CWCServerUDPCipher`). They share the CWC hash, so the
  tag fix should carry over, but no PS3 client has exercised them yet — verify rather than
  assume when reaching the game service.
- **`sceNpMatching2` / P2P** (summoning, invasions). Stubbed in RPCS3
  (`sceNpMatching2RegisterLobbyEventCallback` is a `todo()`), so emulator testing will not
  cover this even once the server supports it. Real hardware only.
- **NP ticket format** — not yet captured; stage 4 has never been reached.

---

## Design implication

The platform split is narrower than expected. Crypto, framing, and the login key are shared;
what actually differs is **identity, version, and how you get the client to talk to you**. So
supporting PC alongside PS3 is mostly a matter of a platform-scoped config bundle
(ports, hostnames, `app_version`, ticket validator) rather than a second protocol family — quite
unlike the Frpg (v1) split for Demon's Souls / DS1, which does need its own transport and crypto.
