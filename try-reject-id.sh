#!/bin/bash
# Cycle the reject push id and restart. Usage: ./nextid.sh 961
cd /mnt/f/ClaudeHole/dso
sed -i "s/^DSO_BREAKIN_REJECT_PUSH_ID=.*/DSO_BREAKIN_REJECT_PUSH_ID=$1/" dso.env
pkill -f '^\./dsoserver'; sleep 1
mv -f server_debug.log server_debug.prev.log 2>/dev/null
env -i PATH=/usr/bin:/bin HOME="$HOME" nohup ./dsoserver > server_debug.log 2>&1 &
sleep 2
printf "now testing push id %s (%#x); listeners %s/5\n" "$1" "$1" "$(grep -cE 'listening' server_debug.log)"
