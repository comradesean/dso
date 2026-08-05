# PARKED: authoring our own calibration payloads

**Status:** on hold. Serving FromSoftware's real calibrations works completely; *authoring* our
own does not, and is blocked on one unsolved format.

**Why it was parked:** the immediate goal — getting new event items into the Majula Mansion chest
— was not achieved even after successfully delivering calibration 0114, which is the only
published payload that changes the event-item table. The chest stayed empty. That result points
away from calibration data and toward a missing *server-side* trigger, so the effort is better
spent elsewhere. See "What the negative result means" below.

## What already works — do not redo this

Serving calibrations end to end is **done and verified**. A real client was moved 1.01 → 1.06 →
1.13 → 1.15 by serving our own payloads, and the copy it stored is byte-identical to ours.

- All ten published calibrations are archived in `data/calibrations/` (gitignored).
- `DSO_CALIBRATION_VERSION` answers the hardcoded `contents_0101.bin` request with any version.
- The full loop, storage format, and the three behaviours that make it look broken are documented
  in `docs/STATUS.md` under "Delivering a calibration, end to end".

## The container format — solved

```
offset 0x000   256 bytes   RSA-2048 header, e=3, verified with the key at EBOOT 0x189AB48
                           (v1.00) / 0x1910670 (v1.10). Carries the AES IV. NOT SOLVED.
offset 0x100   N bytes     AES-128-CBC, N 16-aligned.  SizeEnc = 256 + ceil16(SizeOrg)
```

- **AES key** `739c12a4f1a252662850ebb02ddd3402` = `MD5(k1)^MD5(k2)^MD5(k3)`
- **HMAC key** `9b0e703f14bbda7f2b63efb7b71fa5192bb7727a` = `SHA1(k1)^SHA1(k2)^SHA1(k3)`
- k1/k2/k3 are 32-char strings at EBOOT `0x01D50208` / `0x01D501E0` / `0x01D501B8`:
  `Pe8krXyIwOxgFgQfORsHtDyReIt4VZKI`, `kyvUsGcL2sQdh6s4ihbbfwoYaxRg_0Ap`,
  `tsmEtjATsjM9uQKrgZ1vg03ItJKEg9L5`
- Manifest plaintext is ASCII `KEY\t = \tVALUE`, parsed with `strtol`. Payload plaintext is a
  DCX; `zlib` inflate `pt[0x4c:SizeOrg]` gives the BND4.
- Integrity: `HMAC-SHA1(hmacKey, inflated_bnd)` must equal the manifest's `DIGEST`.

**IVs do not need the RSA header to read an existing file** — recover them from known plaintext,
since CBC gives `IV = D(C1) XOR P1`:

- manifests always begin `Patch.List.Count` (exactly 16 bytes)
- regulations always begin with the DCX magic `44 43 58 00 00 01 00 00 00 00 00 18 00 00 00 24`

That trick decrypted all twenty archived files and every HMAC matched its manifest digest.

## The one blocker

**The 256-byte RSA header format.** Writing a payload the client accepts requires producing a
valid header, which carries the IV. With `m = c^3 mod n` over two samples, 214 of 256 bytes are
identical and three fields vary:

```
m[150:154]   4 B    length or version
m[158:175]  17 B    likely the IV
m[190:211]  21 B    likely a digest
```

Ruled out already: raw fields, constant-XOR at every offset, RSA-OAEP-SHA1 (EM[0]=0x2b not 0x00,
DB[:20] != SHA1("")), PKCS#1 v1.5. One transform remains unidentified.

**Two things make this much more tractable than when it was first attempted:**

1. **Twenty samples now, not two.** Every archived file has a *known* IV (recovered as above), a
   known SizeOrg and a known DIGEST. That is twenty (header, IV, size, digest) tuples to solve
   the layout by differencing — the original analysis had two.
2. **We would not have to forge it.** The header is verified with the key at `0x189AB48` /
   `0x1910670`, and `tools/rpcs3/dso.yml` already contains a (disabled) patch replacing that key
   with ours. With our key in place we can *sign* headers with our own private key rather than
   reverse the padding. The remaining unknown is only the field layout, not the signature.

To make progress, find the code consuming the 256-byte header: locate the OpenSSL
`AES_set_decrypt_key` / `AES_cbc_encrypt` callers. The inverse S-box is at VA `0x17E9F30` (only
the inverse table is present — the client only decrypts), but nothing references it via a data
word, so it is materialised by `lis/ori` or through an unresolved anchor.

## What the negative result means

Calibration **0114 installed successfully** on a v1.10 client and the chest was still empty.

- `ItemLotParam2_SvrEvent.param` is byte-identical across 0101–0113; only 0114 changes it.
- 0114 adds lots 11250/11260/11270 (guaranteed Human Effigy) and 11280 (Titanite Chunk 90 /
  Slab 5 / Twinkling 3 / Petrified Dragon Bone 2), and upgrades 11180 to that same weighted drop.
- Those lots are now present in the client's regulation and still nothing appears.

**Therefore the lot table is necessary but not sufficient — something must select an active lot,
and that selection is not in the regulation.** The likely candidates are all server-side and are
in `tasks/remaining-features.md`:

- an event flag delivered or set by the server
- one of the unimplemented opcodes (`RequestNotifyRingBell` `0x03EE` is suggestively named)
- the `PlayerInfoUploadConfigPushMessage` push `0x038C`, which we never send

Chase those before returning to this file.

## Related

- `docs/STATUS.md` — the working delivery loop and save-slot format
- `tools/rpcs3/dso.yml` — the calibration-key patch, currently disabled
- Memory: `dso-ds2-calibration-capture`, `dso-ps3-eboot-toolchain`
