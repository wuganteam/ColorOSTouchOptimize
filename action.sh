#!/system/bin/sh

MODDIR=${0%/*}

open_mt() {
    local path="$1"
    am start -n bin.mt.plus/bin.mt.plus.Main -d "file://$path" >/dev/null 2>&1
    if [ $? -ne 0 ]; then
        am start -n bin.mt.plus.canary/bin.mt.plus.Main -d "file://$path" >/dev/null 2>&1
    fi
}

open_mt "/data/games.conf"
sleep 3