#!/system/bin/sh
MODDIR=${0%/*}
CONF_DIR="/data/adb/modules/ColorOSTouchOptimize/common"
PID_FILE="$CONF_DIR/touch_opt.pid"
LOG_FILE="$CONF_DIR/touch_opt.log"

# ---------- 日志（分级） ----------
log_i() { echo "$(date) [INFO] $1" >> "$LOG_FILE"; }
log_w() { echo "$(date) [WARN] $1" >> "$LOG_FILE"; }
log_e() { echo "$(date) [ERROR] $1" >> "$LOG_FILE"; }

# 等待系统完全启动
while [ "$(getprop sys.boot_completed)" != "1" ]; do
    sleep 2
done
sleep 10

# 手动执行 post-fs-data.sh（已移入 common/，Magisk 不再自动执行）
if [ -f "$MODDIR/common/post-fs-data.sh" ]; then
    sh "$MODDIR/common/post-fs-data.sh"
fi

# 检查二进制是否存在且可执行
if [ ! -x "$MODDIR/common/touch_opt" ]; then
    log_e "未找到可执行的 touch_opt 二进制，模块不启动"
    exit 0
fi

# ---------- 启动守护进程 ----------
# 注：PID 文件由 Go 程序自行管理，service.sh 只负责拉起进程
start_daemon() {
    nohup "$MODDIR/common/touch_opt" >/dev/null 2>&1 &
    log_i "守护进程已启动 (PID $!)"
}

# 清理旧 PID 文件（防止残留干扰）
rm -f "$PID_FILE"

# ---------- 自愈守护 ----------
start_daemon
while true; do
    sleep 300    # 每 5 分钟检查一次
    if [ -f "$PID_FILE" ]; then
        pid=$(cat "$PID_FILE" 2>/dev/null)
        if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
            continue
        fi
    fi
    log_w "检测到守护进程退出，自动重启"
    start_daemon
done
