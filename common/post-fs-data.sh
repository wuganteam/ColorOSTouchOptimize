#!/system/bin/sh
# 本脚本位于 common/，由 service.sh 手动调用（Magisk 不自动执行 common 内脚本）
LOG_FILE="/data/adb/modules/ColorOSTouchOptimize/common/touch_opt.log"
if [ -f "$LOG_FILE" ]; then
    rm "$LOG_FILE"
    echo "$(date) 日志文件已删除" >> "$LOG_FILE"
fi