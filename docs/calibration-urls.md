# Calibration payload URLs — reference

**This file is a link reference, nothing more.** Every calibration FromSoftware published for
Dark Souls II on PS3 (BLUS41045), with the address it was served from, its size and its SHA-1.
How the files are *built* and *decrypted* is in [`regulation-format.md`](regulation-format.md);
how they are *delivered to a client* is in [`STATUS.md`](STATUS.md).

All twenty are already mirrored in the repo under `calibrations/`, so nothing here needs
downloading to work on the project. The URLs are recorded so the mirror can be re-derived, and
checked, from the original source.

## The host

```
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/
```

The same host `DSO_DNS` redirects. **A retail client only ever requests one of these URLs** —
`contents_0101.bin`, hardcoded at EBOOT vaddr `0x17F9C00`, and the only FQDN in the binary
besides `openssl.org`. Everything else here is an archive address: the client learns the
regulation filename from whichever manifest it is handed, which is what lets
`DSO_CALIBRATION_VERSION` answer the frozen 0101 request with a later version's manifest.

**Still live.** A HEAD on `contents_0101.bin` on 2026-08-05 returned `Last-Modified: Sun, 26 Jan
2014` and an ETag matching our copy byte for byte — the bucket outlived the game servers. Only
that one URL was re-checked; the other nineteen are recorded from when the mirror was taken.

## The payloads

`contents_NNNN.bin` is the manifest — always exactly 640 bytes, encrypted, naming the regulation
file to fetch. `regulation_NNNN.bin` is the payload it points at.

Dates are the server's `Last-Modified`, preserved as the mtime on the mirrored files.

| Version | Published | `contents_NNNN.bin` SHA-1 | `regulation_NNNN.bin` SHA-1 | Bytes |
|---|---|---|---|---|
| 0101 | 2014-01-26 | `dc2c58c52c3e7ccc2ba2b2b391ebb1028a5e667b` | `88112f633d750a38c59311c1386b1cdcddab70f3` | 674,992 |
| 0104 | 2014-01-31 | `436e1358a5ddcb6ae31c971b74f10f4d0f9604d4` | `64fa33ed12ffa89c8268707870798d9c6b7f221d` | 675,024 |
| 0107 | 2014-06-02 | `1df614b8ecdb271a77c050ac1068c1e444db7ee1` | `0b86b36b2417d4291be28d043b9d80a448ea3cc6` | 675,872 |
| 0108 | 2014-06-02 | `0668b8a6cd7ce08907ed77308efdb4a22ff99c0a` | `015467c574fda867128f105b5c17cc56ba756d86` | 748,416 |
| 0109 | 2014-06-05 | `18b91dafacad5ac884f655355fec1f5c3a69d3aa` | `ff66f992e80bce5d2ba2ffefc6f27d211aa921f7` | 675,872 |
| 0110 | 2014-07-08 | `2ecd9a03c1a6da14d6b09e8bc33f75ffc9cdf89b` | `12dca432021668280806acf1a3b1607941e9d28a` | 762,864 |
| 0111 | 2014-08-11 | `b690c2357570de7ac67b957ea5083f8a79e3f138` | `409354d73be62044dd6e9399c5691f2a14bacd43` | 798,304 |
| 0112 | 2014-09-09 | `388b2594e222d17b7cbd9d172668d9c368d4a967` | `a5ae33d74d0114c1a98cb9991a17d3025d908434` | 827,600 |
| 0113 | 2014-10-07 | `d0f7abe6658d49bc4d9d1567e52bc47601f5818d` | `a83f46f6b185aeaf64d87e99ea487c8f59b3cdfd` | 827,616 |
| 0114 | 2015-04-01 | `e436c31c1c45352061e02e31ca6efcc642bc552c` | `68850f3c68bc9d28fae99ce5d692d396f1ed1b09` | 829,184 |

Every `contents_NNNN.bin` is 640 bytes; the size column is the regulation.

**The numbering has gaps.** No `0102`, `0103`, `0105` or `0106` was ever found at this host. They
may never have shipped, or may have been pulled — either way, do not go hunting for them.

## Things worth knowing before picking a version

- **`0104` reports the same version as `0101`** (stamp `00010100`), so applying it changes nothing
  visible even though it alters seven params. A perfectly working delivery looks like a no-op.
- **`0114` is the ceiling on a launch-disc install.** It downloads in full and then crashes RPCS3
  on `BLUS41045_v01.00` — an April 2015 payload in a different stamp format, expecting a matching
  title update. Nothing is written when it fails, so the install survives. Walk forward from the
  oldest and stop at the last one that boots.
- **`0108` is the odd one.** It is 72 KB larger than `0107` published the same day, and `0109`
  three days later is back to `0107`'s size — though not `0107`'s contents. Unexplained; recorded
  in case it matters later.
- Known version stamps, read from BND4+24 after decryption: disc `00010000`, 0101/0104
  `00010100`, 0107 `00010600`, 0110 `00010810`, 0114 `01001500`. The rest have not been read.

## The URLs

<details>
<summary>Manifests — <code>contents_NNNN.bin</code></summary>

```
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/contents_0101.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/contents_0104.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/contents_0107.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/contents_0108.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/contents_0109.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/contents_0110.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/contents_0111.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/contents_0112.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/contents_0113.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/contents_0114.bin
```

</details>

<details>
<summary>Payloads — <code>regulation_NNNN.bin</code></summary>

```
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/regulation_0101.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/regulation_0104.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/regulation_0107.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/regulation_0108.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/regulation_0109.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/regulation_0110.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/regulation_0111.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/regulation_0112.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/regulation_0113.bin
http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/regulation_0114.bin
```

</details>

## Verifying the mirror

```sh
cd calibrations && sha1sum -c <<'EOF'
dc2c58c52c3e7ccc2ba2b2b391ebb1028a5e667b  contents_0101.bin
436e1358a5ddcb6ae31c971b74f10f4d0f9604d4  contents_0104.bin
1df614b8ecdb271a77c050ac1068c1e444db7ee1  contents_0107.bin
0668b8a6cd7ce08907ed77308efdb4a22ff99c0a  contents_0108.bin
18b91dafacad5ac884f655355fec1f5c3a69d3aa  contents_0109.bin
2ecd9a03c1a6da14d6b09e8bc33f75ffc9cdf89b  contents_0110.bin
b690c2357570de7ac67b957ea5083f8a79e3f138  contents_0111.bin
388b2594e222d17b7cbd9d172668d9c368d4a967  contents_0112.bin
d0f7abe6658d49bc4d9d1567e52bc47601f5818d  contents_0113.bin
e436c31c1c45352061e02e31ca6efcc642bc552c  contents_0114.bin
88112f633d750a38c59311c1386b1cdcddab70f3  regulation_0101.bin
64fa33ed12ffa89c8268707870798d9c6b7f221d  regulation_0104.bin
0b86b36b2417d4291be28d043b9d80a448ea3cc6  regulation_0107.bin
015467c574fda867128f105b5c17cc56ba756d86  regulation_0108.bin
ff66f992e80bce5d2ba2ffefc6f27d211aa921f7  regulation_0109.bin
12dca432021668280806acf1a3b1607941e9d28a  regulation_0110.bin
409354d73be62044dd6e9399c5691f2a14bacd43  regulation_0111.bin
a5ae33d74d0114c1a98cb9991a17d3025d908434  regulation_0112.bin
a83f46f6b185aeaf64d87e99ea487c8f59b3cdfd  regulation_0113.bin
68850f3c68bc9d28fae99ce5d692d396f1ed1b09  regulation_0114.bin
EOF
```

The copies the server actually serves live in `data/calibrations/`
(`DSO_BOOTSTRAP_CALIBRATION_DIR`), which is gitignored.
