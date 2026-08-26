#!/system/bin/sh
# 卸载清理：停掉守护进程，删除配置与日志，无残留
PID_FILE="/data/adb/modules/ColorOSTouchOptimize/common/touch_opt.pid"

# 停止守护进程（Go 二进制）
if [ -f "$PID_FILE" ]; then
    pid=$(cat "$PID_FILE" 2>/dev/null)
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
        kill "$pid" 2>/dev/null
    fi
    rm -f "$PID_FILE"
fi

# 删除配置与日志（含新旧路径，兼容）
rm -f /data/games.conf
rm -f /data/adb/modules/ColorOSTouchOptimize/touch_opt.log*
rm -f /data/adb/modules/ColorOSTouchOptimize/common/touch_opt.log*
exit 0