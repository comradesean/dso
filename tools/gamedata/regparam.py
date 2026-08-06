#!/usr/bin/env python3
"""Extract and patch PARAM files out of an unpacked regulation BND4.

The 0x038B regulation push replaces one whole resource file in the running
client, and for a .param the payload size must equal the loaded resource's size
EXACTLY or the client discards it in silence. So the only safe way to build a
payload is to take the real file out of the regulation the client is running and
edit bytes in place -- never to synthesise one.

Get the BND4 first:

    python3 tools/calibration/calibration.py unpack \\
        data/calibrations/regulation_0114.bin /tmp/reg0114.bnd

Then:

    # what's in there
    python3 tools/gamedata/regparam.py list /tmp/reg0114.bnd

    # pull one out
    python3 tools/gamedata/regparam.py extract /tmp/reg0114.bnd \\
        OnlineEventParam.param -o /tmp/OnlineEventParam.param

    # show its rows
    python3 tools/gamedata/regparam.py rows /tmp/OnlineEventParam.param

    # patch a row field in place, same size out
    python3 tools/gamedata/regparam.py patch /tmp/OnlineEventParam.param \\
        --row 0 --offset 2 --u16 1 -o /tmp/OnlineEventParam.patched.param

Formats are documented in docs/regulation-format.md sections 7 and 8.
"""

import argparse
import struct
import sys


def bnd4_entries(data):
    """Yield (name, dataOffset, size) for every entry in a BND4."""
    if data[:4] != b"BND4":
        raise SystemExit("not a BND4 (missing magic) -- unpack the regulation first")
    count = struct.unpack(">I", data[0x0C:0x10])[0]
    table = struct.unpack(">q", data[0x10:0x18])[0]
    for i in range(count):
        e = table + i * 24
        size = struct.unpack(">q", data[e + 8 : e + 16])[0]
        data_off, name_off = struct.unpack(">II", data[e + 16 : e + 24])
        name = data[name_off : data.index(b"\x00", name_off)].decode("ascii")
        yield name, data_off, size


def param_info(data):
    """Return (paramType, rowCount, rowSize, rows) for a PARAM file.

    Row size is not stored; it is derived from the gap between the first row's
    data and the string block, then validated -- a mismatch means the file is
    not what we think it is.
    """
    strings_off = struct.unpack(">I", data[0x00:0x04])[0]
    row_count = struct.unpack(">H", data[0x0A:0x0C])[0]
    param_type = data[0x0C:0x2C].rstrip(b" \x00").decode("ascii")
    first_row = struct.unpack(">I", data[0x30:0x34])[0]

    if row_count == 0:
        return param_type, 0, 0, []

    row_size, rem = divmod(strings_off - first_row, row_count)
    if rem or first_row + row_count * row_size != strings_off:
        raise SystemExit(
            f"row size does not divide evenly ({param_type}): "
            f"strings={strings_off:#x} first={first_row:#x} count={row_count}"
        )

    rows = []
    for i in range(row_count):
        e = 0x40 + i * 12
        row_id, off, _name_off = struct.unpack(">III", data[e : e + 12])
        rows.append((row_id, off))
    return param_type, row_count, row_size, rows


def cmd_list(args):
    data = open(args.bnd, "rb").read()
    for name, off, size in bnd4_entries(data):
        if args.filter and args.filter.lower() not in name.lower():
            continue
        print(f"{name:<48} size={size:<8} off={off:#x}")


def cmd_extract(args):
    data = open(args.bnd, "rb").read()
    for name, off, size in bnd4_entries(data):
        if name == args.name or name.endswith("/" + args.name):
            out = args.out or args.name
            open(out, "wb").write(data[off : off + size])
            print(f"wrote {size} bytes to {out}")
            return
    raise SystemExit(f"{args.name} not found in {args.bnd}")


def cmd_rows(args):
    data = open(args.param, "rb").read()
    param_type, count, row_size, rows = param_info(data)
    print(f"{param_type}: {count} rows of {row_size} bytes, file {len(data)} bytes")
    for row_id, off in rows:
        if args.row is not None and row_id != args.row:
            continue
        print(f"  id={row_id:<10} @{off:#x}  {data[off:off + row_size].hex()}")


def cmd_patch(args):
    data = bytearray(open(args.param, "rb").read())
    original_len = len(data)
    _, _, row_size, rows = param_info(bytes(data))

    for row_id, off in rows:
        if row_id != args.row:
            continue
        if args.offset + (2 if args.u16 is not None else 4) > row_size:
            raise SystemExit(f"offset {args.offset} past end of {row_size}-byte row")
        target = off + args.offset
        before = bytes(data[off : off + row_size])
        if args.u16 is not None:
            struct.pack_into(">H", data, target, args.u16)
        else:
            struct.pack_into(">I", data, target, args.u32)
        after = bytes(data[off : off + row_size])

        # The push is discarded silently if the size changes, so make the
        # invariant loud here instead.
        assert len(data) == original_len, "patch changed the file size"

        out = args.out or args.param
        open(out, "wb").write(data)
        print(f"row {row_id} +{args.offset}")
        print(f"  before {before.hex()}")
        print(f"  after  {after.hex()}")
        print(f"wrote {len(data)} bytes to {out} (size unchanged)")
        return

    raise SystemExit(f"row {args.row} not found")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)

    p = sub.add_parser("list", help="list BND4 entries")
    p.add_argument("bnd")
    p.add_argument("--filter", help="substring match on the name")
    p.set_defaults(func=cmd_list)

    p = sub.add_parser("extract", help="extract one entry")
    p.add_argument("bnd")
    p.add_argument("name")
    p.add_argument("-o", "--out")
    p.set_defaults(func=cmd_extract)

    p = sub.add_parser("rows", help="show PARAM rows")
    p.add_argument("param")
    p.add_argument("--row", type=int, help="only this row id")
    p.set_defaults(func=cmd_rows)

    p = sub.add_parser("patch", help="patch a field in a row, size preserved")
    p.add_argument("param")
    p.add_argument("--row", type=int, required=True)
    p.add_argument("--offset", type=int, required=True, help="byte offset within the row")
    g = p.add_mutually_exclusive_group(required=True)
    g.add_argument("--u16", type=int)
    g.add_argument("--u32", type=int)
    p.add_argument("-o", "--out")
    p.set_defaults(func=cmd_patch)

    args = ap.parse_args()
    args.func(args)


if __name__ == "__main__":
    sys.exit(main())
