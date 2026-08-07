#!/bin/bash
# ingest.sh — decrypt a directory of Frpg2 captures into the corpus.
#
#   tools/pcap/ingest.sh "/path/to/PACKETS/SECOND RODEO" [-out corpus] [-prefix TAG]
#
# Every step here exists because doing it by hand got it wrong at least once.
#
#   - THE KEY IS NOT THE LAST ONE DUMPED, and keydump reports several per run
#     (auth stream, then game service). The only reliable test is running every
#     candidate through verifykey and taking the line that says ** VERIFIED **.
#     Grepping the first hex string out of that output returns candidate [1],
#     which is usually a FAILURE, and makes every capture look like it shares one
#     key — which quietly contradicts keys being per-session.
#   - THE PORT VARIES. 50000 and 50001 have both been seen in the same batch.
#   - RING-BUFFER FILES ARE ONE SESSION, not several. dumpcap rolls a new file
#     every ~100 MB, so a single login arrives as ds2_00001..ds2_0000N sharing one
#     key. They are labelled as one session here, with a part number, so the
#     grouping is visible rather than implied.
#   - A CAPTURE WITHOUT ITS KEYS IS PERMANENTLY UNREADABLE. Keys are derived per
#     login and never appear on the wire. If a directory has no keys.txt, this
#     says so loudly rather than skipping quietly.
#
# Candidate keys are taken from every keys.txt at or below the given directory,
# plus tmp/keys.txt if present, so a misfiled key file still gets tried.
set -uo pipefail
cd "$(dirname "$0")/../.." || exit 1
export PATH="$HOME/sdk/go/bin:$HOME/go/bin:$PATH"

root="${1:?usage: ingest.sh <capture-dir> [-out DIR] [-prefix TAG]}"; shift
out="corpus"; prefix=""
while [ $# -gt 0 ]; do
    case "$1" in
        -out)    out="$2"; shift 2 ;;
        -prefix) prefix="$2"; shift 2 ;;
        *) echo "unknown option: $1" >&2; exit 2 ;;
    esac
done
[ -d "$root" ] || { echo "no such directory: $root" >&2; exit 1; }

tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
go build -o "$tmp/corpus" ./cmd/corpus || exit 1
go build -o "$tmp/verifykey" ./cmd/verifykey || exit 1

# Every candidate key we can find, deduplicated.
{ find "$root" -iname 'keys.txt' -exec cat {} \; ; cat tmp/keys.txt 2>/dev/null ; } \
    | grep -oE '\b[0-9a-f]{32}\b' | sort -u > "$tmp/keys"
nkeys=$(wc -l < "$tmp/keys")
[ "$nkeys" -gt 0 ] || { echo "no keys found under $root — captures without keys are unreadable" >&2; exit 1; }
echo "$nkeys candidate key(s)"

mapfile -t caps < <(find "$root" -iname '*.pcapng' -o -iname '*.pcap' | sort)
[ "${#caps[@]}" -gt 0 ] || { echo "no captures under $root" >&2; exit 1; }
echo "${#caps[@]} capture(s)"
echo

# Group state: the key/port most recently established. A ring-buffer continuation
# file has no handshake to verify against, so it inherits — but ONLY after the
# file has been given a chance to verify on its own.
#
# Inheriting first was the bug: one directory here holds TWO logins (a short
# early session, then a six-file ring-buffer set), and reusing the first key for
# the rest produced 0 decrypted / 1258 failed on every one of them. Loud, but
# entirely self-inflicted.
cur_key=""; cur_port=""; group=0; part=0
ok=0; skipped=0; inherited=0

for cap in "${caps[@]}"; do
    base=$(basename "$cap"); dir=$(dirname "$cap")

    key=""; port=""
    for p in 50000 50001; do
        dg=$(timeout 120 python3 tools/pcap/udpdump.py "$cap" --port "$p" --dir c2s \
                --raw --limit 1 --min-len 40 2>/dev/null | head -1)
        [ -z "$dg" ] && continue
        k=$("$tmp/verifykey" -keys "$tmp/keys" -datagram "$dg" c2s 2>&1 \
                | grep 'VERIFIED' | grep -oE '\b[0-9a-f]{32}\b' | head -1)
        if [ -n "$k" ]; then key="$k"; port="$p"; break; fi
    done

    if [ -n "$key" ]; then
        # A key different from the running one means a new login starts here.
        if [ "$key" != "$cur_key" ]; then
            group=$((group+1)); part=0
            cur_key="$key"; cur_port="$port"
        fi
    elif [ -n "$cur_key" ]; then
        key="$cur_key"; port="$cur_port"
        inherited=$((inherited+1))
    else
        echo "SKIP  $base — no candidate key authenticates it, and no earlier key to inherit"
        skipped=$((skipped+1)); continue
    fi

    part=$((part+1))
    tag=$(basename "$dir" | tr -cd '[:alnum:]')
    stamp=$(echo "$base" | grep -oE '[0-9]{14}' | head -1)
    label="${prefix}${tag}_s${group}p${part}_${stamp}"

    timeout 900 python3 tools/pcap/udpdump.py "$cap" --port "$port" --tagged \
        | "$tmp/corpus" -out "$out" -session "$label" -key "$key" 2>&1 | head -1
    ok=$((ok+1))
done

echo
echo "ingested $ok capture(s) in $group login group(s); $inherited inherited a key, $skipped skipped -> $out/"
find "$out" -type f 2>/dev/null | wc -l | xargs echo "corpus now holds"
