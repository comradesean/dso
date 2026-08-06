# Calibration and regulation file formats (DS2 PS3, BLUS41045)

Complete specification of the two files the client fetches over HTTP at boot
(`contents_NNNN.bin`, `regulation_NNNN.bin`) and of everything inside them.

**All five layers are now solved, including the 256-byte RSA header that was previously the
blocker.** The layout was recovered by differencing all twenty archived files and is confirmed
by exact byte-for-byte reconstruction of all twenty headers (see §3.5).

Evidence labels used throughout: **CONFIRMED (bytes)** = reproduced from the archived files;
**CONFIRMED (EBOOT 0x…)** = read out of the decrypted EBOOT; **INFERRED** = consistent with all
evidence but not directly witnessed; **SPECULATION** = plausible, untested.

Related: `docs/STATUS.md` (the working delivery loop), `tasks/calibration-reverse-engineering.md`,
memory `dso-ds2-calibration-capture`.

---

## 1. The two files

The client hardcodes one URL (EBOOT vaddr `0x17F9C00`) and makes **two** requests per boot:

```
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/contents_0101.bin
  -> a manifest, 640 bytes, which names ...
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/regulation_0101.bin
  -> the payload
```

Both files use **the same container** (§2). Ten calibrations were published; all are archived in
`data/calibrations/` (gitignored).

| version | date | manifest SizeOrg | regulation SizeOrg | regulation SizeEnc |
|---|---|---|---|---|
| 0101 | 2014-01-26 | 377 | 674721 | 674992 |
| 0104 | 2014-01-31 | 377 | 674765 | 675024 |
| 0107 | 2014-06-02 | 377 | 675615 | 675872 |
| 0108 | 2014-06-02 | 377 | 748152 | 748416 |
| 0109 | 2014-06-05 | 377 | 675615 | 675872 |
| 0110 | 2014-07-08 | 377 | 762607 | 762864 |
| 0111 | 2014-08-11 | 377 | 798033 | 798304 |
| 0112 | 2014-09-09 | 377 | 827340 | 827600 |
| 0113 | 2014-10-07 | 377 | 827360 | 827616 |
| 0114 | 2015-04-01 | 377 | 828921 | 829184 |

All ten manifests are exactly 377 plaintext bytes — the only variable-length text is the DIGEST
(fixed 40 hex chars), the version in the URL (fixed 4 digits) and the two sizes (all 6 digits).
**CONFIRMED (bytes)**

---

## 2. Container

```
offset 0x000   256 bytes   RSA-2048 header (signature). Carries the IV, the size and a MAC.
offset 0x100   N bytes     AES-128-CBC ciphertext, N = ceil16(SizeOrg)

SizeEnc = 256 + ceil16(SizeOrg)         <- the on-disc file size
```

Tail padding after the last plaintext byte is **uninitialised heap, not PKCS#7** — do not rely on
it and do not reproduce it. Our writer zero-pads; the client only ever reads `plain[:SizeOrg]`.
**CONFIRMED (bytes)**

### 2.1 Keys

```
AES-128 key : 739c12a4f1a252662850ebb02ddd3402   = MD5(k1)  ^ MD5(k2)  ^ MD5(k3)
HMAC-SHA1 key: 9b0e703f14bbda7f2b63efb7b71fa5192bb7727a = SHA1(k1) ^ SHA1(k2) ^ SHA1(k3)
```

k1/k2/k3 are three 32-char ASCII constants at EBOOT `0x01D50208` / `0x01D501E0` / `0x01D501B8`:

```
Pe8krXyIwOxgFgQfORsHtDyReIt4VZKI
kyvUsGcL2sQdh6s4ihbbfwoYaxRg_0Ap
tsmEtjATsjM9uQKrgZ1vg03ItJKEg9L5
```

**CONFIRMED (EBOOT 0x01D501B8..0x01D50227)**

### 2.2 The RSA key

The EBOOT contains **exactly two** RSA public keys, both PKCS#1 PEM, 426 bytes, exponent 3:

| file offset | vaddr | role |
|---|---|---|
| `0x17EB338` | `0x17FB338` | **login / auth** key (`MIIBCAKCAQEAxSeDuBTm3Ayt…`) |
| `0x188AB48` | `0x189AB48` | **calibration / patch** key (`MIIBCAKCAQEAluKyYootlsum…`) |

**CONFIRMED (EBOOT).** The calibration key is the one that verifies these headers; it is
immediately followed (`0x188ACF2`+) by the UTF-16 `Patch.List.Count` / `Patch.List.File%d.*`
format strings, which is independent corroboration that this key belongs to the patch subsystem.

> Correction to earlier notes: `0x189AB48` was previously described as the login key. It is not —
> it is the calibration key, and `0x17FB338` is the login key. `cmd/dsotool/main.go` already
> defaults to the correct login address.

Modulus (n), 2048-bit, e = 3:

```
-----BEGIN RSA PUBLIC KEY-----
MIIBCAKCAQEAluKyYootlsumw6gmDZuW6ZRaAywRwHjbQt6W2fNmYxYzzW5uHVdI
ZL7kRvt3oOO5LO/uvoaQMzMvm/3KBASoXVnCcTIHxEFSORyuV66A6qJMF8OG0D5Z
jgfvsjqdhFgT2LlKuKfzoy0baG5fHUV/tmMQe+why1R+gkXifMjsO0oDJPT6HQr2
dTwxAy2FhlRBMtAItHl1uZgKwmFOEHwCnFwOTt1n72Uyz9IMs09ffuRqNrOdqW09
y32KuTSFIkNLEpDtlwFh0/q8VKQywWEzIZ1GyCoZ44bJdv8svX79aNklzhbkJ5Kn
GbzDc3BoHdjrm1sws/ZAMZNexY6VTOpxhwIBAw==
-----END RSA PUBLIC KEY-----
```

---

## 3. The 256-byte RSA header — SOLVED

### 3.1 Recovering the plaintext

```
c = int(header[0:256], big-endian)
M = n - (c^3 mod n)          # 256 bytes, big-endian
```

**The negation is the part that hid this for so long.** `c^3 mod n` on its own gives a
structured-but-unreadable value; the readable struct is its additive inverse mod n. Because
e = 3 is odd this is equivalent to saying the stored header is `n - sig` where
`sig^3 mod n = M`, i.e. `(n - c)^3 mod n = M`. Either formulation works. **CONFIRMED (bytes,
20/20 files)**

### 3.2 Layout of M

```
offset  size  value
------  ----  -----------------------------------------------------------------
0x000      1  0x6B                       fixed
0x001    141  0xBB ...                   fixed filler
0x08E      1  0xBA                       fixed end-of-filler marker
0x08F      4  "ENCR"                     magic  (45 4E 43 52)
0x093      4  01 00 01 00                version, two u16 LE = (1, 1)
0x097      8  SizeOrg                    uint64, LITTLE-endian
0x09F     16  IV                         AES-128-CBC IV, big-endian/raw
0x0AF     16  00 * 16                    reserved, always zero
0x0BF     20  HMAC-SHA1(hmacKey, plain[:SizeOrg])
0x0D3     44  00 * 44                    reserved, always zero
0x0FF      1  0xCC                       fixed terminator
```

Read as a struct starting at the magic, it is a clean 0x70-byte record:

```
+0x00  char[4]  magic   "ENCR"
+0x04  u16      1               (LE)
+0x06  u16      1               (LE)
+0x08  u64      SizeOrg         (LE)
+0x10  u8[16]   iv
+0x20  u8[16]   zero
+0x30  u8[20]   hmac_sha1
+0x44  u8[44]   zero
       (0x8F + 0x70 == 0xFF, then the 0xCC terminator)
```

**CONFIRMED (bytes)** for every field: sizes, IVs and HMACs from all 20 files reconstruct all
256 header bytes exactly.

Notes:

- The leading `0x6B` guarantees `M < n` for any 2048-bit modulus (top byte ≥ 0x80), so the
  header is valid under our own key too. **INFERRED** (that this is *why* 0x6B was chosen).
- The header's digest is **HMAC-SHA1 of the container plaintext**, i.e. of the DCX for a
  regulation and of the ASCII text for a manifest. It is **not** the same value as the
  manifest's `DIGEST` field, which is HMAC-SHA1 of the *inflated BND4*. Two different digests
  over two different byte strings. **CONFIRMED (bytes)**
- The `SizeOrg` in the header always equals the `SizeOrg` in the manifest for the file it
  describes; for a manifest it is the manifest's own plaintext length (377). **CONFIRMED**
- `"ENCR"`, `0xBB`, `0x6B` and `0xCC` do **not** appear as immediates or rodata anywhere in the
  EBOOT (searched for the 4-byte word, for `lis 0x454E`, and for `0xBBBBBBBB`). The client most
  likely reads the IV and size at fixed offsets without validating magic or filler.
  **INFERRED** — so a header that gets those two fields right will probably be accepted even if
  the pad is wrong, but reproduce the template anyway.

### 3.3 Signing your own

```
sig = (n - M)^d mod n        # d = private exponent of whichever key the client trusts
header = sig.to_bytes(256, 'big')
```

Verified round-trip with a freshly generated e=3 2048-bit key: pack → unpack reproduces the
plaintext, the HMAC checks, and the inflated BND4 is byte-identical to the input.
**CONFIRMED (bytes)**

Because the client verifies with FromSoftware's public key, authoring requires **replacing that
key**. `tools/rpcs3/dso.yml` already carries a (currently disabled) patch that overwrites the
calibration key at `0x189AB48`. Generate the replacement with
`dsotool keygen --exponent 3` — an e=3 2048-bit PKCS#1 PEM is exactly 426 bytes, the same length
as the original, so it drops in without shifting anything.

### 3.4 Reading without solving anything (still valid)

If you only want to *read* an archived file, the IV can be recovered from known plaintext,
since CBC gives `IV = D(C1) XOR P1`:

- manifests always begin `Patch.List.Count` (exactly 16 bytes)
- regulations always begin the DCX magic `44 43 58 00 00 01 00 00 00 00 00 18 00 00 00 24`

Both routes agree on all twenty IVs.

### 3.5 How it was solved (so it can be re-derived)

Model discovery, not guesswork: differencing the twenty `c^3 mod n` values left three varying
byte runs. Brute-forcing `int(m[a:b]) ± value << shift == constant` over candidate values found
three exact fits across all twenty samples simultaneously:

```
int(m[144:158]) + byteswap32(SizeOrg) << 24                          == 0xe48cb6b37974b9980ac2614e107c
int(m[157:178]) + int(IV)             << 24                          == 0x7c029c5c0e4edd67ef6532cfd20cb34f5f7ee46a36
int(m[189:212]) + int(HMAC-SHA1(plain[:SizeOrg])) << 8               == 0x22434b1290ed970161d3fabc54a432c16133219d46c82a
```

Solving for the single global constant K gave `K = m + fields`, identical for all twenty, and
`n - K` turned out to be the 0x6B/0xBB/0xBA/`ENCR`/0xCC pad — which is what revealed that the
whole thing is one big-integer negation, not three independent fields. (The "little-endian
SizeOrg / big-endian IV" asymmetry above is real and is a property of the struct, not of the
negation.)

Reference implementation: `tools/calibration/calibration.py`.

---

## 4. AES layer

`AES-128-CBC`, key from §2.1, IV from the header, ciphertext at `0x100`, length
`ceil16(SizeOrg)`. Decrypt, then take `plain[:SizeOrg]`.

Integrity: `HMAC-SHA1(hmacKey, plain[:SizeOrg])` must equal the header's digest field.

---

## 5. Manifest plaintext

Plain ASCII, keys and values separated by the literal three-character sequence
`\t` `space` `=` `space` `\t`, one pair per line, `\n` terminated. Numeric values are parsed with
`strtol`, so there are no structs and none of the usual PS3 alignment traps apply.

```
Patch.List.Count         = 1
Patch.List.File0.DIGEST  = F5AEC728004C145A72315B809334DDBE195A3204
Patch.List.File0.Dir     = system:/
Patch.List.File0.Name    = regulation.bnd
Patch.List.File0.Path    = http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/regulation_0114.bin
Patch.List.File0.SizeEnc = 829184
Patch.List.File0.SizeOrg = 828921
Patch.List.File0.Version = 1
```

`DIGEST` is `HMAC-SHA1(hmacKey, inflated_BND4)` — over the *decompressed* BND4, not the DCX.
Every archived pair verifies. **CONFIRMED (bytes)**

`Path` is followed literally, so a payload can be served from any host; only the manifest URL
itself is hardcoded in the EBOOT.

---

## 6. DCX (the regulation plaintext)

The decrypted regulation is a DCX container. Header is exactly 0x4C bytes, big-endian:

```
+0x00  char[4] "DCX\0"
+0x04  u32     0x00010000
+0x08  u32     0x00000018     offset of DCS
+0x0C  u32     0x00000024
+0x10  u32     0x00000024
+0x14  u32     0x0000002C
+0x18  char[4] "DCS\0"
+0x1C  u32     uncompressedSize
+0x20  u32     compressedSize
+0x24  char[4] "DCP\0"
+0x28  char[4] "DFLT"
+0x2C  u32     0x00000020
+0x30  u8      0x09           zlib level
+0x31  u8[15]  0
+0x40  u32     0x00010100
+0x44  char[4] "DCA\0"
+0x48  u32     0x00000008
+0x4C  ...     raw zlib stream (78 DA ...)
```

Therefore **`SizeOrg == 0x4C + compressedSize`** — verified for all ten regulations.

Inflating `plain[0x4C:SizeOrg]` gives the BND4. Re-compressing the 0114 BND4 with plain
`zlib.compress(data, 9)` reproduces FromSoftware's exact `compressedSize` and hence their exact
`SizeOrg`/`SizeEnc` — so their tool used stock zlib level 9. **CONFIRMED (bytes)**

---

## 7. BND4

Big-endian throughout.

```
+0x00  char[4] "BND4"
+0x04  u8[4]   01 01 00 00
+0x08  u8[4]   00 01 00 00
+0x0C  u32     fileCount
+0x10  u64     0x40            offset of the entry table
+0x18  char[8] "01001500"      version string
+0x20  u64     0x18            entry size
+0x28  u64     data start (approx; not needed)
+0x30  u8      0               unicode flag (names are ASCII)
+0x31  u8      0x0C            format
+0x32  ...     0
```

Entry table at `0x40`, `fileCount` entries of 24 bytes:

```
+0x00  u32     0x02000000      flags
+0x04  i32     -1
+0x08  u64     size
+0x10  u32     dataOffset      absolute, from the start of the BND4
+0x14  u32     nameOffset      absolute; NUL-terminated ASCII
```

Entry counts: 252 (0101/0104), 248 (0107/0109), 253 (0108), 249 (0110–0113), 221 (0114).
The archive holds `*.param`, `*.emevd` and a handful of `regulation<Lang>.fmg`.

---

## 8. PARAM

Big-endian.

```
+0x00  u32       stringsOffset       (also: end of the row-data block)
+0x04  u16       0
+0x06  u16       flag                (0 or 1; meaning unknown)
+0x08  u16       paramdefDataVersion (1, 2 or 4)
+0x0A  u16       rowCount
+0x0C  char[0x20] paramType, space-padded  e.g. "ITEM_LOT_PARAM2"
+0x2C  u8[4]     FF 03 01 FF
+0x30  u32       firstRowDataOffset  == 0x40 + rowCount*12
+0x34  u8[12]    0
+0x40            row index: rowCount entries of 12 bytes
```

Row index entry:

```
+0x00  u32  id
+0x04  u32  dataOffset
+0x08  u32  nameOffset   (0 in every regulation param)
```

**Row size is not stored.** Derive it and validate:

```
rowSize = (stringsOffset - firstRowDataOffset) / rowCount
assert firstRowDataOffset + rowCount*rowSize == stringsOffset
```

This holds for every param in every archived regulation. **CONFIRMED (bytes)**

Observed row sizes: `EVENT_FLAG_LIST_PARAM` 4, `EVENT_COMMON_PARAM` 4, `ONLINE_EVENT_PARAM` 16,
`RESULT_EVENT_PARAM` 24, `ITEM_LOT_PARAM2` 124.

### 8.1 ITEM_LOT_PARAM2 (124 bytes)

Used by `ItemLotParam2_Chr.param`, `ItemLotParam2_Other.param` and
`ItemLotParam2_SvrEvent.param`.

```
+0x00  u8[4]    header. [0] = 0x03 or 0x00, [1] = 0x01 or 0x03, [2..3] = 0
+0x04  u8[10]   quantity per slot         (1 for event lots; 10/20/30 for the soul lots)
+0x0E  u8[10]   unknown, zero in these tables
+0x18  u8[10]   unknown, zero in these tables
+0x22  u8[10]   per-slot enable flag; mirrors the non-zero entries of +0x04
+0x2C  u32[10]  lotItemId; the sentinel for "unused slot" is 10, not 0
+0x54  f32[10]  relative weight
```

Cross-checked against known ground truth: lot 11280 is
`60980000 w90 / 60990000 w5 / 61000000 w3 / 61030000 w2` = Titanite Chunk 90 / Slab 5 /
Twinkling 3 / Petrified Dragon Bone 2, and lots 11250–11270 are a single guaranteed
`60151000` = Human Effigy. Both match the independently documented 0114 payload exactly.
**CONFIRMED (bytes)**

Caveat: the three `9000000x` soul lots have three item slots but four non-zero weights
(`100 / 0.25 / 0.25 / 0.25`), which does not fit the 1:1 reading. Either slot 0 is a
"nothing" slot in some mode selected by header byte [1] (0x03 there vs 0x01 elsewhere), or one
of the two arrays is offset by one for that mode. Unresolved; irrelevant to the 11xxx lots,
which are unambiguous. **Open.**

### 8.2 RESULT_EVENT_PARAM (24 bytes)

```
+0x00  u32   packed flag bytes (unknown)
+0x04  u32   group id  (7 digits, 122xxxx)
+0x08  u32   ItemLotParam2_Other  lot id     (10xxx, 0 = none)
+0x0C  u32   ItemLotParam2_SvrEvent lot id   (11xxx, 0 = none)
+0x10  u32   0, or 0x01000000 from 0107 onward on four rows
+0x14  u32   0
```

**This is the table that binds an item lot to an event.** 82 rows in every version.

### 8.3 EVENT_FLAG_LIST_PARAM (4 bytes)

A pure indirection table: `row id -> event flag id (u32)`. E.g. `3500 -> 103500`,
`10001150 -> 101100`, `5035000 -> 535000010`. Row ids in the `10xxxxxx` range look like a
compiled-in enum; row ids in the `50xxxxx` range are `<map><index>`.

### 8.4 EVENT_COMMON_PARAM (4 bytes)

A scalar constants table, `row id -> u32`. 15 rows through 0112; 0113 added row 15 = 1;
0114 added row 16 = 30000.

### 8.5 ONLINE_EVENT_PARAM (16 bytes)

**One row, id 0, all sixteen bytes zero, in every single calibration.** It is a placeholder and
was never used. Rule it out. **CONFIRMED (bytes)**

---

## 9. Authoring a calibration — verified recipe

```python
# payload
dcx   = DCX_HEADER(len(bnd), len(zlib.compress(bnd, 9))) + zlib.compress(bnd, 9)
iv    = os.urandom(16)
ct    = AES_CBC_encrypt(AESKEY, iv, dcx + b'\0' * (-len(dcx) % 16))
M     = header_template(SizeOrg=len(dcx), iv=iv,
                        digest=HMAC_SHA1(HKEY, dcx))
sig   = pow(n - int(M), d, n)
open('regulation_XXXX.bin','wb').write(sig.to_bytes(256,'big') + ct)

# manifest: same container, plaintext is the ASCII text of §5, with
#   DIGEST  = HMAC_SHA1(HKEY, bnd).hex().upper()      <- the INFLATED bnd
#   SizeOrg = len(dcx)
#   SizeEnc = 256 + ceil16(len(dcx))
```

Working implementation: **`tools/calibration/calibration.py`**.

```
# read, with no key material of ours at all
python3 tools/calibration/calibration.py unpack data/calibrations/regulation_0114.bin out.bnd
python3 tools/calibration/calibration.py unpack data/calibrations/contents_0114.bin out.txt

# author
python3 tools/calibration/calibration.py pack out.bnd regulation_9999.bin \
        --key priv.pem --manifest contents_9999.bin --url http://host/regulation_9999.bin
```

Verified: repacking the 0114 BND4 yields `SizeOrg=828921 SizeEnc=829184` — FromSoftware's own
values for that file — and a full pack→unpack round trip returns the BND4 byte for byte.

Remaining requirement: the client must trust our key (RPCS3 patch at `0x189AB48`), or we must
have FromSoftware's private key, which we do not.

---

## 10. What actually changed across the ten calibrations

Params in the "event" family, by SHA-1 of the param blob:

| param | 0101 | 0104 | 0107 | 0108 | 0109 | 0110 | 0111 | 0112 | 0113 | 0114 |
|---|---|---|---|---|---|---|---|---|---|---|
| `ItemLotParam2_SvrEvent` | A | A | A | A | A | A | A | A | A | **B** |
| `ItemLotParam2_Other` | A | A | A | **B** | B | **C** | **D** | **E** | E | **F** |
| `ResultEventParam` | A | A | **B** | B | B | B | B | B | B | **C** |
| `EventFlagListParam` | A | A | A | **B** | **A** | **C** | **D** | **E** | **F** | **G** |
| `EventCommonParam` | A | A | A | A | A | A | A | A | **B** | **C** |
| `OnlineEventParam` | A | A | A | A | A | A | A | A | A | A |
| `NpcEventParam` | A | A | A | A | A | A | A | A | A | A |

**`EventFlagListParam` growth tracks the three DLC releases, not any event rotation.**
**CONFIRMED (bytes) + INFERRED (mapping to DLC)**

- 0108 (2 Jun 2014) added exactly 20 rows, fourteen of them in the `5035xxx` block.
- 0109 (5 Jun 2014) **removed all 20 again**, reverting byte-for-byte to the 0101 file.
- 0110 (8 Jul 2014) re-added them plus 22 more, all `5035xxx`. DLC1 *Crown of the Sunken King*
  shipped 22 Jul 2014 — map **m50_35**.
- 0111 (11 Aug 2014) added 32 rows, all `5036xxx`. DLC2 *Crown of the Old Iron King* shipped
  26 Aug 2014 — map **m50_36**.
- 0112 (9 Sep 2014) added 33 rows, mostly `5037xxx`. DLC3 *Crown of the Ivory King* shipped
  30 Sep 2014 — map **m50_37**.

The added values are nine-digit flags in the matching `536…`/`537…` space. So the six
consecutive `EventFlagListParam` changes over 0108–0113 are **DLC map flag registration** plus a
three-day accidental early publish that was rolled back. They are not the event surface.

---

## 11. The event-chest chain

The Majula mansion chest is a `MapObjSvrEventTreasureBoxComponent` — the class name is in the
EBOOT at vaddr `0x17F6CC8` (ASCII) / `0x17F6C8C` (UTF-16BE), alongside `MapObjTreasureBoxComponent`
and the other map-object component names. **CONFIRMED (EBOOT 0x17F6CC8)**

The full chain, all of it inside the regulation:

```
RESULT_EVENT_PARAM row  --+0x08-->  ItemLotParam2_Other   lot 10xxx
                        \--+0x0C-->  ItemLotParam2_SvrEvent lot 11xxx
                                        \--> ITEM_LOT_PARAM2 row -> item ids + weights
```

Nothing else in the regulation references the 11xxx lot ids: an exhaustive 4-byte-aligned scan
of the whole inflated BND4 for `11000`, `11180`, `11240`, `11250`, `11280` finds them only in
`ItemLotParam2_SvrEvent` itself and in `ResultEventParam`. **CONFIRMED (bytes)**

### 11.1 The baseline table is not a rotation

`ItemLotParam2_SvrEvent` holds 28 rows in 0101–0113 and 32 in 0114:

- `11000`–`11240` in steps of 10, **except 11040 which does not exist** — 25 lots. Each is bound
  by `ResultEventParam` to one (occasionally two or three) result-event rows. Three of them
  (`11090`, `11100`, `11130`) are empty in every version.
- `10045500` — one guaranteed item `60420000` (**Torch ×1**), unchanged in all ten calibrations.
  **NOW CONFIRMED as the mansion event chest**, not merely inferred (2026-08-06):

  Majula is `m10_04_00_00`, and the mansion holds **two chest objects at identical coordinates**,
  both model `o04_0230`, entries #563/#564 of `map\m10_04_00_00\m10_04_00_00.msb`:

  | object | MSB name | selector byte | item lot |
  |---|---|---|---|
  | `10045500` | `o04_0230_0000` | `0x1C`, shared with 167 objects | `10045600` → Soul Vessel ×1 |
  | **`10045510`** | `o04_0230_0001` | **`0x24`, unique across all 3641 map objects** | **`10045500`** |

  So the ordinary chest is `10045500` (Soul Vessel, matching the wiki's "by default the chest
  always contains 1 Soul Vessel") and the EVENT chest is `10045510`, whose lot `10045500` is the
  only map-object row present in `ItemLotParam2_SvrEvent`. `MapObjectInstanceParam.+0x24` is the
  item lot id, validated by 889 of 924 non-zero values across all maps resolving to real
  `ItemLotParam2_Other` rows.

  The uniqueness of selector `0x24` is an empirical fact; that it *means* "server event chest"
  is inference.
- `90000000` / `90000001` — soul lots (item 1000000 / 2610000, x10/x20/x30, weight
  100 / 0.25 / 0.25).

So the 28 baseline lots are a **static per-result-event reward table**, one lot per event, not a
28-week rotation. Nothing in it changes between January 2014 and April 2015. Since the
documented in-game rotation was weekly and only ten calibrations were ever published, **the
rotation cannot have been driven by calibration content.** **INFERRED (strong)**

### 11.2 What 0114 actually did

0114 is not a one-file change. It changed **four** files in the chain, and it wired them
together:

| lot | contents | bound to ResultEvent row | that row before 0114 |
|---|---|---|---|
| 11250 | Human Effigy ×1 | 1200 | lot fields were **0** |
| 11260 | Human Effigy ×1 | 2400 | lot fields were **0** |
| 11270 | Human Effigy ×1 | 1100 | lot fields were **0** |
| 11280 | Chunk 90 / Slab 5 / Twinkling 3 / Dragon Bone 2 | 1300 | lot fields were **0** |
| 11180 | upgraded from Chunk ×1 to the same weighted drop | 411 | already bound |
| 11030, 11190, 11200, 11220 | gained a second item | 2100, 212, 412, 2015+2115 | already bound |

`ItemLotParam2_Other` received the identical four new lots (`10250`–`10280`) with identical
contents, and `ResultEventParam` names both halves of each pair. The two tables are kept in
perfect lockstep — every 11xxx lot has a byte-identical 10xxx twin.

**This closes the gap the earlier analysis left open in the wrong direction.** The regulation
0114 wiring is complete: lot table, twin table, and the selector that binds lots to events all
shipped together.

### CORRECTION (2026-08-06): none of this is about the chest

The conclusion above — "the reason the chest stayed empty is that the result event never fires" —
is **wrong**, and so is the premise that the chest is `ResultEventParam`-gated at all.

All 82 `ResultEventParam` rows were read. **Not one references lot `10045500` or `10045600`.**
The `11xxx` lots that table binds are post-session and covenant rewards — Awestone, Sunlight
Medal, Token of Fidelity and Spite, Smooth & Silky Stone, Human Effigy, Bonfire Ascetic, Pharros'
Lockstone, Dragon Scale, cracked eye orbs. That is the results screen after an online session,
not a chest in Majula.

The claim that "every 11xxx lot has a byte-identical 10xxx twin" is also imprecise: `11000`–`11280`
exist **only** in `_SvrEvent`, and `10045500` is the only id present in both tables.

**No scripting is involved either.** The three ids appear in exactly two files across the whole
v1.10 archive set — the m10_04 MSB and `MapObjectInstanceParam_m10_04_00_00.param` — and in none
of the 584 EzState scripts, and nowhere as immediates in the executable. The chest is native
component behaviour reading param data.

### 11.3 What we could not determine

What causes a `RESULT_EVENT_PARAM` row to fire. The row ids (0, 100, 116, 117, 199, 212,
300–304, 316, 317, 399, 406–417, 499, 501–504, 599, 601–605, 699, 701–705, 799, 899, 999,
1099–1100, 1199–1200, 1205, 1299–1300, 1311, 1399–1400, 1500, 1600, 1699–1700, 1715, 1800, 1814,
1899, 1916–1917, 2015, 2100, 2115, 2200, 2400, 2405, 2499, 2508, 2510, 2599, 2616–2617,
2716–2717, 2816–2817) look like `area * 100 + slot`, and the `+0x04` group ids repeat across
areas — but that is pattern-matching, not evidence. **SPECULATION.**

To resolve it we would need to follow the code: the param-name pointer pool at vaddr
`0x1C85BC4` (`ItemLotParam2_Other`) / `0x1C85BC8` (`ItemLotParam2_SvrEvent`) is the param
registry, indexed by an enum; whoever indexes it leads to the lot reader, and from there to the
`MapObjSvrEventTreasureBoxComponent` caller. That is a multi-hour disassembly job and was not
attempted here.

---

## 12. What the server can toggle — current state of the search

**Ruled out by this work:**

- `EventFlagListParam.param` — the six consecutive changes over 0108–0113 are DLC map flag
  registration (§10), not an event surface. It was the top-ranked candidate; it is now bottom.
- `OnlineEventParam.param` — one row, sixteen zero bytes, never changed in any calibration (§8.5).
- `NpcEventParam.param` — byte-identical in all ten calibrations.
- *(already ruled out elsewhere)* `RequestNotifyRingBell` `0x03EE`.

**Still standing, and now the only serious candidate:**

- **`0x038B RegulationFileUpdatePushMessage`.** Handler at `0x158B150`, `ParseFromArray` at
  `0x1655F68`. The handler is large (~0x900 bytes, running past `0x158BE50`), allocates and
  builds `std::string`s in a loop over the repeated field, and calls a 10-argument routine at
  `0x17DE1E4` with an immediate `2000`. That is consistent with it doing real work rather than
  being a stub, but it is **not proof that it applies a regulation** — that remains unproven, as
  before. Its `RegulationFileDiffData` carries `start_at`/`end_at`, which matches the shape of
  dated event windows. **CONFIRMED (EBOOT 0x158B150) that it parses and loops; the effect is
  still open.**
- `0x038C PlayerInfoUploadConfigPushMessage` (handler `0x1588218`) — never sent by us.

**New angle worth trying first, because it is cheap:** now that we can author signed
calibrations, we can test the hypothesis directly. Publish a calibration that binds a lot to a
result-event row and *also* changes something trivially observable, and bisect what makes the
chest fill. In particular, 0114 bound its new lots to four rows (1100, 1200, 1300, 2400) that
had never had a lot before — try instead **binding a new lot to a row that is already known to
fire**, e.g. row 2200 (lot 11000) or row 1400 (lot 11010). If those fill the chest and 0114's
rows do not, the gate is per-row and the answer is in the map/event data, not the server.

---

## 13. The community tool in `ref/`

`ref/Dark Souls II Parameter Editor/Offzip and Packzip/` contains a VB.NET save editor
(`Dark Souls II Parameter Editor.exe`), a separate `Dark Souls II Checksum Fixer.exe`, and
Luigi Auriemma's `offzip`/`packzip`. Its two batch files confirm the save-slot format
independently:

```
offzip.exe -a -1 USER_DATA15 "New Folder" 0     # 8-byte header, then raw zlib
packzip.exe -o 0x8 00000008.dat USER_DATA15
```

**The Checksum Fixer is a generic tool, not DS2 knowledge.** Its string table is a menu of
~40 algorithms (`CRC_16_*`, `CRC_32*`, `CRC_64_ECMA`, `ADLER_8/16/32`, `CHECKSUM_8..64`,
`AP_Hash`, `BKDR_Hash`, `DJB_HASH`, `ELF_HASH`, …) plus `BeginOffset` / `EndOffset` /
`CurrentOffset` controls, built on the open-source `PackageIO` endian-IO library. It is a
"pick your algorithm and byte range" utility; it encodes no DS2-specific checksum. The
implication that the community route was *editing the downloaded calibration inside the save
slot* rather than authoring a signed one is consistent with these tools — but that route is now
**obsolete**, because §3 lets us author a properly signed calibration instead. Do not spend more
time on it.

---

## 14. Summary of what is now possible

- Read any archived calibration without tricks (§3.1).
- Verify one end to end: RSA → IV/size/HMAC → AES → DCX → zlib → BND4 → params.
- **Author a new one**, byte-exact in structure, given a key the client trusts (§9).
- Edit any param and re-pack: change item lots, event flags, constants, FMG text.

The last remaining blocker for the event chest is not the file format. It is knowing what makes
a `RESULT_EVENT_PARAM` row fire.
