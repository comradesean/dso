#!/usr/bin/env python3
"""Extract UDP payloads from a pcapng capture.

Written because there is no tshark or scapy on this box, and because the Frpg2
game service is plain UDP: once you have the payload bytes you can hand them
straight to `go run ./cmd/verifykey` or to a decoder.

pcapng is simple enough to read directly — a stream of length-prefixed blocks,
of which only the Enhanced Packet Block (type 6) carries frames. Endianness is
declared by the Section Header Block's byte-order magic.

    # what UDP flows are in here
    python3 tools/pcap/udpdump.py capture.pcapng --summary

    # payload hex for the Frpg2 game service, first 5 datagrams each way
    python3 tools/pcap/udpdump.py capture.pcapng --port 50000 --limit 5

    # one datagram, hex only, ready to paste into verifykey
    python3 tools/pcap/udpdump.py capture.pcapng --port 50000 --dir s2c --limit 1 --raw

Direction is inferred from --local (defaults to any RFC1918 address): a datagram
whose source is local is client->server (c2s), otherwise server->client (s2c).
"""

import argparse
import ipaddress
import struct
import sys
from collections import Counter

SHB = 0x0A0D0D0A
IDB = 0x00000001
EPB = 0x00000006

LINKTYPE_ETHERNET = 1
LINKTYPE_RAW = 101
LINKTYPE_LINUX_SLL = 113


def read_blocks(path):
    """Yield (block_type, body) for each pcapng block."""
    with open(path, "rb") as fh:
        endian = "<"
        while True:
            head = fh.read(8)
            if len(head) < 8:
                return
            btype = struct.unpack(endian + "I", head[0:4])[0]

            if btype == SHB:
                # Byte-order magic decides endianness for everything after.
                # The magic is the u32 0x1A2B3C4D written in the file's own byte
                # order, so the BYTES 4d 3c 2b 1a mean little-endian and
                # 1a 2b 3c 4d mean big-endian. Getting this backwards parses every
                # subsequent block length as garbage and silently reads one block.
                magic = fh.read(4)
                if magic == b"\x4d\x3c\x2b\x1a":
                    endian = "<"
                elif magic == b"\x1a\x2b\x3c\x4d":
                    endian = ">"
                else:
                    raise SystemExit(f"{path}: not a pcapng (bad byte-order magic)")
                total = struct.unpack(endian + "I", head[4:8])[0]
                rest = fh.read(total - 12)
                yield btype, magic + rest
                continue

            total = struct.unpack(endian + "I", head[4:8])[0]
            if total < 12:
                return  # truncated or corrupt; stop rather than loop forever
            # A block is type(4) + total_length(4) + body + total_length(4). That
            # TRAILING length must be consumed too, or every following block
            # starts four bytes early and the whole file reads as garbage after
            # the first one.
            body = fh.read(total - 12)
            if len(body) < total - 12:
                return  # file still being written
            if len(fh.read(4)) < 4:
                return
            yield btype, body


def idb_timestamp_divisor(body):
    """Timestamp units per second for an interface, from the IDB's if_tsresol.

    This is NOT always microseconds. The pcapng default is 1e-6, but dumpcap and
    Wireshark commonly write NANOSECOND captures, and this script assumed the
    default -- so every timestamp it printed for such a file was 1000x too large.
    It went unnoticed because the only consumer was a human glancing at `#` lines.

    if_tsresol (option code 9) is one byte: MSB clear means 10^-value, MSB set
    means 2^-value.

    IDB body: linktype(2) reserved(2) snaplen(4) then options, each
    code(2) len(2) value(len) padded up to a 4-byte boundary, ending at code 0.
    """
    off = 8
    while off + 4 <= len(body):
        code, olen = struct.unpack("<HH", body[off:off + 4])
        off += 4
        if code == 0:                       # opt_endofopt
            break
        if code == 9 and olen >= 1:         # if_tsresol
            raw = body[off]
            return (1 << (raw & 0x7F)) if raw & 0x80 else (10 ** raw)
        off += (olen + 3) & ~3
    return 1_000_000


def parse_frame(link_type, data):
    """Return (src, dst, sport, dport, payload) for a UDP frame, else None."""
    if link_type == LINKTYPE_ETHERNET:
        if len(data) < 14:
            return None
        ethertype = struct.unpack(">H", data[12:14])[0]
        offset = 14
        # Step over any VLAN tags.
        while ethertype in (0x8100, 0x88A8) and len(data) >= offset + 4:
            ethertype = struct.unpack(">H", data[offset + 2 : offset + 4])[0]
            offset += 4
        if ethertype != 0x0800:
            return None
        ip = data[offset:]
    elif link_type == LINKTYPE_RAW:
        ip = data
    elif link_type == LINKTYPE_LINUX_SLL:
        if len(data) < 16 or struct.unpack(">H", data[14:16])[0] != 0x0800:
            return None
        ip = data[16:]
    else:
        return None

    if len(ip) < 20 or (ip[0] >> 4) != 4:
        return None
    ihl = (ip[0] & 0x0F) * 4
    if ip[9] != 17:  # not UDP
        return None
    src = ".".join(str(b) for b in ip[12:16])
    dst = ".".join(str(b) for b in ip[16:20])

    udp = ip[ihl:]
    if len(udp) < 8:
        return None
    sport, dport, length = struct.unpack(">HHH", udp[0:6])
    payload = udp[8:length] if 8 <= length <= len(udp) else udp[8:]
    return src, dst, sport, dport, payload


def is_local(addr, local):
    if local:
        return addr == local
    try:
        return ipaddress.ip_address(addr).is_private
    except ValueError:
        return False


def main():
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("capture")
    ap.add_argument("--port", type=int, help="only this UDP port (either end)")
    ap.add_argument("--dir", choices=("c2s", "s2c"), help="only this direction")
    ap.add_argument("--local", help="local IP; default is any RFC1918 address")
    ap.add_argument("--limit", type=int, default=0, help="stop after N datagrams (0 = all)")
    ap.add_argument("--min-len", type=int, default=0, help="skip payloads shorter than this")
    ap.add_argument("--summary", action="store_true", help="list flows and counts, no payloads")
    ap.add_argument("--raw", action="store_true", help="print payload hex only, one per line")
    ap.add_argument("--tagged", action="store_true",
                    help="print '<c2s|s2c> <hex>' per line, for cmd/decodecap")
    args = ap.parse_args()

    link_type = LINKTYPE_ETHERNET
    ts_div = 1_000_000          # pcapng default: microseconds
    flows = Counter()
    shown = 0
    total = 0

    for btype, body in read_blocks(args.capture):
        if btype == IDB:
            link_type = struct.unpack("<H", body[0:2])[0]
            ts_div = idb_timestamp_divisor(body)
            continue
        if btype != EPB:
            continue

        # EPB: interface(4) ts_hi(4) ts_lo(4) cap_len(4) orig_len(4) then data.
        if len(body) < 20:
            continue
        ts_hi, ts_lo, cap_len = struct.unpack("<III", body[4:16])
        data = body[20 : 20 + cap_len]

        parsed = parse_frame(link_type, data)
        if not parsed:
            continue
        src, dst, sport, dport, payload = parsed

        if args.port and args.port not in (sport, dport):
            continue
        if len(payload) < args.min_len:
            continue

        direction = "c2s" if is_local(src, args.local) else "s2c"
        if args.dir and direction != args.dir:
            continue

        total += 1
        if args.summary:
            flows[(src, sport, dst, dport)] += 1
            continue

        if args.limit and shown >= args.limit:
            break

        if args.tagged:
            # Timestamp first, because several open questions are RATE questions
            # that message order alone cannot answer -- the ~20.5s auto-summon
            # poll and its post-session backoff, whether 0x038C's three periods
            # drive anything, how long FromSoftware's server took to turn a
            # trigger into a push. The pcap has microsecond resolution and this
            # mode used to throw it away.
            #
            # cmd/corpus accepts the field as optional, so an older tagged dump
            # still parses.
            ts = ((ts_hi << 32) | ts_lo) / ts_div
            print(f"{direction} {ts:.6f} {payload.hex()}")
        elif args.raw:
            print(payload.hex())
        else:
            ts = ((ts_hi << 32) | ts_lo) / ts_div
            print(f"# {ts:.6f}  {direction}  {src}:{sport} -> {dst}:{dport}  {len(payload)} bytes")
            print(payload.hex())
        shown += 1

    if args.summary:
        print(f"{total} UDP datagram(s) matched\n")
        for (src, sport, dst, dport), n in flows.most_common(40):
            print(f"  {n:>7}  {src}:{sport} -> {dst}:{dport}")
    elif not args.raw and not args.tagged:
        print(f"\n# {total} matched, {shown} shown", file=sys.stderr)


if __name__ == "__main__":
    main()
