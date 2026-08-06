#!/usr/bin/env python3
"""Read and extract from the PS3 GameData archive (BHF4/BDF4).

The DS2 PS3 data archive is NOT encrypted, unlike the SOTFS PC one. It is a
plain big-endian BXF4 pair — GameData.bhd holds the index and filenames,
GameData.bdt holds the bytes — so it can be read directly without WitchyBND or
any Souls modding toolchain. This was assumed to be impossible earlier in the
project ("the entries are compressed, a scan would not settle it either way"),
which was simply wrong: nothing had checked the header.

Header, all big-endian:

    0x00  "BHF4"
    0x0C  u32  file count
    0x10  u64  header size (0x40)
    0x18  char[8] version stamp
    0x20  u64  file-entry size (0x1C)
    0x40  entries begin

Each 28-byte entry:

    +0x00  u32  flags (0x02 observed throughout)
    +0x04  u32  padding, always 0xFFFFFFFF
    +0x08  u64  size
    +0x10  u64  offset into the .bdt
    +0x18  u32  offset of the NUL-terminated name, within the .bhd

Entries are stored uncompressed at those offsets; individual files may still be
DCX-wrapped, which shows as a "DCX\\0" magic in the extracted bytes.

Usage:
    bhf4.py list [substring]        names, optionally filtered
    bhf4.py extract <substring> <outdir>
    bhf4.py grep32 <int> [substring] search files for a big-endian u32

The grep32 mode is what identified the bell trigger: EzState command 130631
appears in exactly two of the game's 475 .esd scripts, event_m10_16_00_00 and
event_m10_19_00_00 — Belfry Luna and Belfry Sol — and nowhere else, which is
what proved opcode 0x03EE is genuinely the bell and is reachable in retail.
"""

import os
import struct
import sys

USRDIR = os.environ.get(
    "DSO_PS3_USRDIR",
    "/mnt/d/rpcs3-v0.0.41-19598-357b7d44_win64_msvc/games/"
    "DARK SOULS Ⅱ [BLUS41045]/PS3_GAME/USRDIR",
)


def entries(usrdir=USRDIR):
    """Yield (name, offset, size) for every file in the archive index."""
    bhd = open(os.path.join(usrdir, "GameData.bhd"), "rb").read()
    if bhd[:4] != b"BHF4":
        raise SystemExit(f"not a BHF4 index: {bhd[:4]!r}")
    count = struct.unpack_from(">I", bhd, 0x0C)[0]
    for i in range(count):
        base = 0x40 + i * 28
        size, off = struct.unpack_from(">QQ", bhd, base + 8)
        noff = struct.unpack_from(">I", bhd, base + 24)[0]
        name = bhd[noff : bhd.index(b"\x00", noff)].decode("latin1")
        yield name, off, size


def read(bdt, off, size):
    bdt.seek(off)
    return bdt.read(size)


def main(argv):
    if len(argv) < 2:
        raise SystemExit(__doc__)
    mode = argv[1]
    idx = list(entries())
    bdt_path = os.path.join(USRDIR, "GameData.bdt")

    if mode == "list":
        needle = argv[2].lower() if len(argv) > 2 else ""
        for name, _, size in idx:
            if needle in name.lower():
                print(f"{size:>10}  {name}")
        return

    if mode == "extract":
        needle, outdir = argv[2].lower(), argv[3]
        os.makedirs(outdir, exist_ok=True)
        with open(bdt_path, "rb") as bdt:
            for name, off, size in idx:
                if needle not in name.lower():
                    continue
                data = read(bdt, off, size)
                dest = os.path.join(outdir, os.path.basename(name.replace("\\", "/")))
                open(dest, "wb").write(data)
                print(f"{dest}  {size} bytes  magic={data[:4]!r}")
        return

    if mode == "grep32":
        pat = struct.pack(">I", int(argv[2], 0))
        needle = argv[3].lower() if len(argv) > 3 else ""
        scanned = 0
        with open(bdt_path, "rb") as bdt:
            for name, off, size in idx:
                if needle and needle not in name.lower():
                    continue
                scanned += 1
                n = read(bdt, off, size).count(pat)
                if n:
                    print(f"{name}  x{n}")
        print(f"-- scanned {scanned} files", file=sys.stderr)
        return

    raise SystemExit(__doc__)


if __name__ == "__main__":
    main(sys.argv)
