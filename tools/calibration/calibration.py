#!/usr/bin/env python3
"""Reference reader/writer for DS2 PS3 calibration containers.

Executable spec for docs/regulation-format.md. Verified against all ten archived
calibrations: unpacking reproduces every plaintext, and packing reproduces
FromSoftware's own SizeOrg / SizeEnc / DIGEST values byte for byte.

Reading needs only the public key (the client's, at EBOOT vaddr 0x189AB48).
Writing needs a private key, which means the client must be patched to trust ours.

    python3 calibration.py unpack contents_0114.bin out.txt
    python3 calibration.py unpack regulation_0114.bin out.bnd     # also inflates
    python3 calibration.py pack   out.bnd regulation_9999.bin --key priv.pem

Requires `cryptography`.
"""

import argparse
import hashlib
import hmac
import os
import struct
import sys
import zlib

from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes
from cryptography.hazmat.primitives.serialization import load_pem_private_key, load_pem_public_key

# MD5(k1)^MD5(k2)^MD5(k3) and SHA1(k1)^SHA1(k2)^SHA1(k3), the k's being three
# 32-char constants at EBOOT 0x01D50208 / 0x01D501E0 / 0x01D501B8.
AES_KEY = bytes.fromhex("739c12a4f1a252662850ebb02ddd3402")
HMAC_KEY = bytes.fromhex("9b0e703f14bbda7f2b63efb7b71fa5192bb7727a")

# The calibration verification key embedded in BLUS41045 at vaddr 0x189AB48.
# (The other embedded key, at 0x17FB338, is the login key -- not this one.)
CLIENT_PUBKEY_PEM = b"""-----BEGIN RSA PUBLIC KEY-----
MIIBCAKCAQEAluKyYootlsumw6gmDZuW6ZRaAywRwHjbQt6W2fNmYxYzzW5uHVdI
ZL7kRvt3oOO5LO/uvoaQMzMvm/3KBASoXVnCcTIHxEFSORyuV66A6qJMF8OG0D5Z
jgfvsjqdhFgT2LlKuKfzoy0baG5fHUV/tmMQe+why1R+gkXifMjsO0oDJPT6HQr2
dTwxAy2FhlRBMtAItHl1uZgKwmFOEHwCnFwOTt1n72Uyz9IMs09ffuRqNrOdqW09
y32KuTSFIkNLEpDtlwFh0/q8VKQywWEzIZ1GyCoZ44bJdv8svX79aNklzhbkJ5Kn
GbzDc3BoHdjrm1sws/ZAMZNexY6VTOpxhwIBAw==
-----END RSA PUBLIC KEY-----
"""


def header_plaintext(size_org: int, iv: bytes, digest: bytes) -> bytes:
    """The 256-byte struct M. The container stores sig where sig^e mod n == n - M."""
    m = bytearray(256)
    m[0] = 0x6B  # keeps M < n for any 2048-bit modulus
    m[1:142] = b"\xbb" * 141
    m[142] = 0xBA
    m[143:147] = b"ENCR"
    m[147:151] = bytes([1, 0, 1, 0])  # two u16 LE, both 1
    m[151:159] = size_org.to_bytes(8, "little")
    m[159:175] = iv
    # m[175:191] reserved zero
    m[191:211] = digest
    # m[211:255] reserved zero
    m[255] = 0xCC
    return bytes(m)


def parse_header(blob: bytes, n: int):
    m = (n - pow(int.from_bytes(blob[:256], "big"), 3, n)).to_bytes(256, "big")
    if m[143:147] != b"ENCR":
        raise ValueError("bad header magic %r -- wrong key?" % m[143:147])
    size_org = int.from_bytes(m[151:159], "little")
    return size_org, m[159:175], m[191:211]


def unpack(blob: bytes, n: int):
    size_org, iv, digest = parse_header(blob, n)
    dec = Cipher(algorithms.AES(AES_KEY), modes.CBC(iv)).decryptor()
    plain = (dec.update(blob[256:]) + dec.finalize())[:size_org]
    if not hmac.compare_digest(hmac.new(HMAC_KEY, plain, hashlib.sha1).digest(), digest):
        raise ValueError("header HMAC mismatch")
    return plain


def pack(plain: bytes, priv) -> bytes:
    n = priv.public_key().public_numbers().n
    d = priv.private_numbers().d
    iv = os.urandom(16)
    padded = plain + b"\0" * (-len(plain) % 16)
    enc = Cipher(algorithms.AES(AES_KEY), modes.CBC(iv)).encryptor()
    ct = enc.update(padded) + enc.finalize()
    m = header_plaintext(len(plain), iv, hmac.new(HMAC_KEY, plain, hashlib.sha1).digest())
    sig = pow(n - int.from_bytes(m, "big"), d, n)
    return sig.to_bytes(256, "big") + ct


def dcx_wrap(raw: bytes) -> bytes:
    """DCX/DCS/DCP/DFLT/DCA header, 0x4C bytes, then a raw zlib stream."""
    comp = zlib.compress(raw, 9)
    h = b"DCX\0" + struct.pack(">IIIII", 0x00010000, 0x18, 0x24, 0x24, 0x2C)
    h += b"DCS\0" + struct.pack(">II", len(raw), len(comp))
    h += b"DCP\0" + b"DFLT" + struct.pack(">I", 0x20) + bytes([9, 0, 0, 0]) + bytes(12)
    h += struct.pack(">I", 0x00010100) + b"DCA\0" + struct.pack(">I", 8)
    assert len(h) == 0x4C
    return h + comp


def dcx_unwrap(dcx: bytes) -> bytes:
    if dcx[:4] != b"DCX\0":
        raise ValueError("not a DCX")
    return zlib.decompress(dcx[0x4C:])


def manifest_text(name, url, bnd, size_org, size_enc) -> bytes:
    """DIGEST is HMAC-SHA1 of the INFLATED bnd, not of the DCX."""
    digest = hmac.new(HMAC_KEY, bnd, hashlib.sha1).hexdigest().upper()
    fields = [
        ("Patch.List.Count", "1"),
        ("Patch.List.File0.DIGEST", digest),
        ("Patch.List.File0.Dir", "system:/"),
        ("Patch.List.File0.Name", name),
        ("Patch.List.File0.Path", url),
        ("Patch.List.File0.SizeEnc", str(size_enc)),
        ("Patch.List.File0.SizeOrg", str(size_org)),
        ("Patch.List.File0.Version", "1"),
    ]
    return "".join("%s\t = \t%s\n" % kv for kv in fields).encode()


def main():
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd", required=True)

    u = sub.add_parser("unpack")
    u.add_argument("infile")
    u.add_argument("outfile")
    u.add_argument("--pubkey", help="PEM; defaults to the client's embedded key")

    p = sub.add_parser("pack")
    p.add_argument("bnd", help="an inflated BND4")
    p.add_argument("outfile")
    p.add_argument("--key", required=True, help="RSA private key PEM (exponent 3)")
    p.add_argument("--manifest", help="also write a manifest here")
    p.add_argument("--url", default="http://127.0.0.1/regulation_9999.bin")

    a = ap.parse_args()

    if a.cmd == "unpack":
        pem = open(a.pubkey, "rb").read() if a.pubkey else CLIENT_PUBKEY_PEM
        n = load_pem_public_key(pem).public_numbers().n
        plain = unpack(open(a.infile, "rb").read(), n)
        if plain[:4] == b"DCX\0":
            inner = dcx_unwrap(plain)
            print("DCX -> %d bytes, magic %r, DIGEST %s"
                  % (len(inner), inner[:4],
                     hmac.new(HMAC_KEY, inner, hashlib.sha1).hexdigest().upper()))
            plain = inner
        open(a.outfile, "wb").write(plain)
        print("wrote %d bytes to %s" % (len(plain), a.outfile))
        return

    priv = load_pem_private_key(open(a.key, "rb").read(), password=None)
    if priv.public_key().public_numbers().e != 3:
        sys.exit("key must have exponent 3")
    bnd = open(a.bnd, "rb").read()
    dcx = dcx_wrap(bnd)
    blob = pack(dcx, priv)
    open(a.outfile, "wb").write(blob)
    print("wrote %s: SizeOrg=%d SizeEnc=%d" % (a.outfile, len(dcx), len(blob)))
    if a.manifest:
        text = manifest_text("regulation.bnd", a.url, bnd, len(dcx), len(blob))
        open(a.manifest, "wb").write(pack(text, priv))
        print("wrote %s: SizeOrg=%d" % (a.manifest, len(text)))


if __name__ == "__main__":
    main()
