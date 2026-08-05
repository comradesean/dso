#!/bin/bash
# Rebuild and restart the server, refusing to do so while anyone is playing.
#
# A restart invalidates every auth token, because the registry is in memory. Both
# clients get dropped at once with "unknown or expired auth token", which looks
# from the game like a server crash or a bug in whatever feature was mid-flight.
# That has already sent us chasing a phantom Mirror Knight bug.
#
#   ./restart.sh          rebuild + restart, refusing if sessions are live
#   ./restart.sh --force   restart anyway
#   ./restart.sh --check   report who is connected and exit
set -u
cd "$(dirname "$0")"
export PATH="$HOME/sdk/go/bin:$HOME/go/bin:$PATH"

# A session counts as live if its peer sent us anything in the last 90s. The
# server's own idle timeout is 60s, so this is deliberately a little longer.
live_peers() {
    [ -f server_debug.log ] || return 0
    local cutoff
    cutoff=$(date -u -d '90 seconds ago' +%Y-%m-%dT%H:%M:%S 2>/dev/null) || return 0
    awk -v cut="$cutoff" '
        match($0, /time=([0-9-]+T[0-9:]+)/, t) && t[1] > cut &&
        match($0, /peer=([0-9.]+:[0-9]+)/, p) { seen[p[1]] = 1 }
        END { for (k in seen) print k }
    ' server_debug.log 2>/dev/null | sort -u
}

peers=$(live_peers)
count=$(printf '%s' "$peers" | grep -c . || true)

if [ "${1:-}" = "--check" ]; then
    if [ "$count" -gt 0 ]; then
        echo "$count client(s) connected in the last 90s:"
        printf '  %s\n' $peers
    else
        echo "no clients connected"
    fi
    exit 0
fi

if [ "$count" -gt 0 ] && [ "${1:-}" != "--force" ]; then
    echo "REFUSING TO RESTART: $count client(s) active in the last 90s:" >&2
    printf '  %s\n' $peers >&2
    echo >&2
    echo "A restart drops them all with 'unknown or expired auth token'." >&2
    echo "Wait for them to finish, or re-run with --force." >&2
    exit 1
fi

go build -o dsoserver ./cmd/dsoserver || exit 1
pkill -f '^\./dsoserver'
sleep 1
mv -f server_debug.log server_debug.prev.log 2>/dev/null
env -i PATH=/usr/bin:/bin HOME="$HOME" nohup ./dsoserver > server_debug.log 2>&1 &
sleep 2

listeners=$(grep -cE 'listening' server_debug.log)
errors=$(grep -c 'level=ERROR' server_debug.log)
echo "restarted: $listeners/5 listeners, $errors errors"
[ "$errors" -gt 0 ] && grep 'level=ERROR' server_debug.log | tail -3
[ "$count" -gt 0 ] && echo "NOTE: $count client(s) were connected; they must re-enter online mode."
exit 0
