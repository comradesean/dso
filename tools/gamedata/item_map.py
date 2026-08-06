#!/usr/bin/env python3
"""Generate docs/item-ids-0114.md: every item id available in a regulation.

An item lot row's item field (`+0x2C`) takes an id from the game's unified item
id space. This cross-references two independent sources:

  * the id space itself, from the game's own item database, already extracted to
    tools/gamedata/ds2_items_english.tsv (1334 id -> English name pairs)
  * which of those ids a given regulation actually defines, from ItemParam,
    WeaponParam, ArmorParam and RingParam inside it

An id present in both is safe to put in a lot. An id in the name table but not in
the regulation is NOT necessarily unusable -- it may be defined in a param this
script does not read -- but it is unverified, so it is listed separately rather
than silently dropped.

Usage:

    python3 tools/calibration/calibration.py unpack \\
        data/calibrations/regulation_0114.bin /tmp/reg0114.bnd
    python3 tools/gamedata/item_map.py /tmp/reg0114.bnd -o docs/item-ids-0114.md
"""

import argparse
import csv
import os
import struct
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from regparam import bnd4_entries, param_info  # noqa: E402

ITEM_PARAMS = ("ItemParam", "WeaponParam", "ArmorParam", "RingParam")

# The id space is banded by category. These are the bands actually observed; the
# labels come from what the names in each band turn out to be, not from any table
# in the game, so treat them as descriptive rather than authoritative.
BANDS = [
    (1_000_000, 6_999_999, "Weapons"),
    (11_000_000, 11_999_999, "Shields"),
    (19_000_000, 19_999_999, "Armor (internal ids, unnamed)"),
    (21_000_000, 27_999_999, "Armor"),
    (31_000_000, 35_999_999, "Spells"),
    (40_000_000, 42_999_999, "Rings"),
    (50_000_000, 53_999_999, "Keys and quest items"),
    (60_000_000, 61_999_999, "Consumables"),
    (62_000_000, 62_999_999, "Multiplayer items"),
    (63_000_000, 63_999_999, "Gestures"),
    (64_000_000, 64_999_999, "Boss souls"),
]


def band_of(item_id):
    for lo, hi, label in BANDS:
        if lo <= item_id <= hi:
            return lo, hi, label
    return None, None, "Other"


def load_names(path):
    with open(path, encoding="utf-8") as fh:
        return {int(r["id"]): r["name"] for r in csv.DictReader(fh, delimiter="\t")}


def load_regulation_ids(bnd_path):
    """Return {id: [param names that define it]} for the item-bearing params."""
    data = open(bnd_path, "rb").read()
    wanted = {p + ".param": p for p in ITEM_PARAMS}
    out = {}
    counts = {}
    for name, off, size in bnd4_entries(data):
        base = name.split("/")[-1]
        if base not in wanted:
            continue
        blob = data[off : off + size]
        _, row_count, _, rows = param_info(blob)
        counts[wanted[base]] = row_count
        for row_id, _ in rows:
            out.setdefault(row_id, []).append(wanted[base])
    return out, counts


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("bnd", help="unpacked regulation BND4")
    ap.add_argument("--names", default="tools/gamedata/ds2_items_english.tsv")
    ap.add_argument("--version", default="0114")
    ap.add_argument("-o", "--out", default="docs/item-ids-0114.md")
    args = ap.parse_args()

    names = load_names(args.names)
    reg, counts = load_regulation_ids(args.bnd)

    present = sorted(i for i in names if i in reg)
    unverified = sorted(i for i in names if i not in reg)
    unnamed = sorted(i for i in reg if i not in names)

    w = []
    w.append(f"# Item ids available in calibration {args.version}\n")
    w.append(
        "**Generated — do not edit by hand.** Regenerate with "
        "`python3 tools/gamedata/item_map.py <unpacked.bnd> -o docs/item-ids-0114.md`.\n"
    )
    w.append(
        "This is the menu for the Majula event chest. An `ItemLotParam2_SvrEvent` row's item field\n"
        "at `+0x2C` takes one of these ids, and that row can be rewritten and pushed to a running\n"
        "client over `0x038B` — see `tasks/majula-event-chest.md`.\n"
    )
    w.append("## How this was built, and what it does not prove\n")
    w.append(
        "Two independent sources, intersected:\n\n"
        "- **The id space and English names** come from the game's own item database, extracted to\n"
        "  `tools/gamedata/ds2_items_english.tsv`.\n"
        f"- **What this regulation defines** comes from reading {', '.join(ITEM_PARAMS)} out of the\n"
        "  unpacked BND4.\n\n"
        "An id in both columns is defined by the regulation the client is running, which is the\n"
        "strongest statement available without putting it in a chest. It does **not** prove the item\n"
        "is obtainable, that its icon and description exist, or that a lot referencing it behaves\n"
        "sensibly. Only a live claim proves that, and so far exactly one id has been proven that\n"
        "way: `60420000` Torch, which the chest handed over on 2026-08-06.\n"
    )
    w.append("## Totals\n")
    w.append("| source | rows |\n|---|---|")
    for p in ITEM_PARAMS:
        w.append(f"| `{p}` | {counts.get(p, 0)} |")
    w.append(f"| **named ids defined by this regulation** | **{len(present)}** |")
    w.append(f"| named ids NOT found in those params (unverified) | {len(unverified)} |")
    w.append(f"| ids defined but with no English name | {len(unnamed)} |\n")

    w.append("## Bands\n")
    w.append("| range | category | available |\n|---|---|---|")
    for lo, hi, label in BANDS:
        n = sum(1 for i in present if lo <= i <= hi)
        if n:
            w.append(f"| `{lo}`–`{hi}` | {label} | {n} |")
    w.append("")

    w.append("## Available ids\n")
    current = None
    for item_id in present:
        lo, hi, label = band_of(item_id)
        if label != current:
            current = label
            w.append(f"\n### {label}\n")
            w.append("| id | name | defined in |\n|---|---|---|")
        w.append(f"| `{item_id}` | {names[item_id]} | {', '.join(reg[item_id])} |")

    if unverified:
        w.append("\n## Named but not found in this regulation's item params\n")
        w.append(
            "These carry names in the game's database but no row in the four params read here.\n"
            "The `892xxxxxx` and `900xxxxxx` blocks are the bulk of them and look like preset or\n"
            "catalogue entries rather than real items. Untested — not known to be unusable.\n"
        )
        w.append("| id | name |\n|---|---|")
        for item_id in unverified:
            w.append(f"| `{item_id}` | {names[item_id]} |")

    w.append(
        f"\n## Ids with no English name\n\n{len(unnamed)} rows across those params have no entry in\n"
        "the name table. They are overwhelmingly `ArmorParam`'s `19xxxxxx` block and enemy-only\n"
        "weapons — internal variants rather than anything a player is meant to hold. Listed by range\n"
        "only, since a nameless id is not much use as a prize.\n"
    )
    ranges = {}
    for item_id in unnamed:
        ranges.setdefault(item_id // 1_000_000, []).append(item_id)
    w.append("| range | count | example |\n|---|---|---|")
    for k in sorted(ranges):
        v = ranges[k]
        w.append(f"| `{k}000000`–`{k}999999` | {len(v)} | `{v[0]}` |")

    out = "\n".join(w) + "\n"
    with open(args.out, "w", encoding="utf-8") as fh:
        fh.write(out)
    print(f"wrote {args.out}: {len(present)} available, {len(unverified)} unverified, {len(unnamed)} unnamed")


if __name__ == "__main__":
    sys.exit(main())
