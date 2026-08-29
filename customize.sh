#!/system/bin/sh
# 注意：Magisk 会自动设置 $MODPATH 指向模块最终目录

# ---------- 权限设置 ----------
set_perm_recursive $MODPATH 0 0 0755 0755
set_perm $MODPATH/service.sh 0 0 0755
set_perm $MODPATH/common/touch_opt 0 0 0755
set_perm $MODPATH/common/post-fs-data.sh 0 0 0755
set_perm $MODPATH/uninstall.sh 0 0 0755

# ---------- 显示信息 ----------
echo "*********************************************"
echo "- ColorOS 触控优化模块安装中..."
echo "*********************************************"

# ---------- 兼容性检查 ----------
brand=$(getprop ro.product.brand)
model=$(getprop ro.product.model)
sdk=$(getprop ro.build.version.sdk)
coloros=$(getprop ro.build.version.oplus 2>/dev/null || getprop ro.build.version.coloros 2>/dev/null)

echo "设备信息：$brand $model，Android SDK $sdk，ColorOS $coloros"

case "$brand" in
    OPPO|OnePlus|realme)
        echo "✅ 品牌支持"
        ;;
    *)
        echo "⚠️ 此模块主要为 ColorOS 设计，当前品牌为 $brand，可能不兼容。"
        ;;
esac

# ---------- 检测 touchHidlTest ----------
if ! command -v touchHidlTest >/dev/null; then
    echo "❌ 错误：未找到 touchHidlTest 命令，您的设备可能不支持此模块。"
    echo "   安装终止。"
    exit 1
fi
echo "✅ touchHidlTest 命令存在"

# 验证读取/写入功能
test_val=$(touchHidlTest -c ro 0 182 2>/dev/null | grep -E '^[0-9]+' | head -1)
if [ -z "$test_val" ]; then
    echo "⚠️ 警告：无法读取触控节点值，模块可能无法正常工作，但将继续安装。"
else
    echo "✅ 当前采样率值: $test_val"
    if ! touchHidlTest -c wo 0 182 0 >/dev/null 2>&1; then
        echo "⚠️ 警告：写入测试失败（权限或节点问题），模块可能无法切换采样率。"
    else
        echo "✅ 写入测试成功。"
        touchHidlTest -c wo 0 182 "$test_val" >/dev/null 2>&1
    fi
fi

# ---------- 设置默认采样率（125Hz） ----------
echo "- 正在设置默认触控采样率（125Hz）..."
if touchHidlTest -c wo 0 182 0; then
    echo "✅ 默认采样率设置成功（当前生效）"
else
    echo "⚠️ 设置失败（可能权限不足），开机后守护进程会再次尝试。"
fi

# ---------- 获取当前用户（支持多用户） ----------
CURRENT_USER=$(am get-current-user 2>/dev/null | tr -d ' ')
[ -z "$CURRENT_USER" ] && CURRENT_USER=0
echo "- 当前用户: $CURRENT_USER"

# ---------- 自动扫描游戏并生成/更新 /data/games.conf ----------
GAMES_CONF="/data/games.conf"
TEMP_LIST="/data/temp_game_list.txt"
> "$TEMP_LIST"

echo "- 正在扫描已安装的游戏..."

# 1. 从 SCENE 读取已标记的游戏（如果存在，主用户数据）
scene_games="/data/data/com.omarea.vtools/shared_prefs/games.xml"
if [ -f "$scene_games" ]; then
    echo "  >> 从 SCENE 添加游戏"
    grep '="true"' "$scene_games" | cut -f2 -d '"' | while read pkg; do
        echo "$pkg" >> "$TEMP_LIST"
    done
else
    echo "  >> 未找到 SCENE 配置文件，跳过"
fi

# 2. 基于 Unity/UE4 引擎检测第三方应用（指定用户范围）
# 2. 添加内置默认游戏列表（确保开箱即用）
default_games="com.tencent.tmgp.sgame com.tencent.tmgp.pubgmhd com.miHoYo.Yuanshen com.miHoYo.GenshinImpact com.miHoYo.hkrpg com.kurogame.mingchao com.tencent.tmgp.dfm com.tencent.tmgp.cf com.tencent.tmgp.cod com.tencent.lolm com.netease.dwrg com.netease.l22 com.tencent.ig com.pubg.krmobile"
for g in $default_games; do
    echo "$g" >> "$TEMP_LIST"
done
echo "  >> 已添加默认游戏列表"
# 去重排序
if [ -s "$TEMP_LIST" ]; then
    sort -u "$TEMP_LIST" -o "$TEMP_LIST"
    count=$(wc -l < "$TEMP_LIST")
    echo "✅ 扫描完成，共发现 $count 个游戏"
else
    echo "⚠️ 未检测到任何游戏"
    > "$TEMP_LIST"
fi

# ---------- 处理 /data/games.conf ----------
# v1.1fix：直接覆盖配置文件，确保包含最新配置项（stop_horae），避免老配置缺失
# 注意：覆盖前备份用户旧配置
if [ -f "$GAMES_CONF" ]; then
    cp "$GAMES_CONF" "$GAMES_CONF.bak" 2>/dev/null
    echo "- 已备份旧配置为 $GAMES_CONF.bak，正在重新生成配置..."
else
    echo "- 创建配置文件 /data/games.conf"
fi
    cat > "$GAMES_CONF" <<'EOF'
# ============================================
# ColorOS 触控优化模块 - 游戏列表与配置
# ============================================
# 您可以在此文件中手动添加/删除游戏包名，并调整采样率值。
# 修改后无需重启，模块会在 5 秒内自动生效。
#
# ---------- 全局配置（必需，且只能有一行） ----------
# 格式：config 游戏采样率=<值>, 日用采样率=<值>, touch_node=<节点号>
#   - 游戏采样率：游戏运行时使用的采样率值，直接填 Hz 数值（默认 360 → 设备默认支持最大值）
#   - 日用采样率：日常使用的采样率值，直接填 Hz 数值（默认 0 → 系统默认 120Hz）
#   - touch_node：触控节点号（默认 182，一般无需修改；部分机型可能不同）
#   - stop_horae：是否停止 ColorOS 采样率管家防止覆盖，1=启用 0=关闭（默认 0）
#   也支持英文键名：config game_rate=360, default_rate=0, touch_node=182, stop_horae=1
config 游戏采样率=360, 日用采样率=0, touch_node=182, stop_horae=0
#
# ---------- 游戏列表（每行一个包名） ----------
# 支持两种格式：
#   com.example.game                使用全局游戏采样率
#   com.example.game rate=8         该游戏使用独立采样率值 8
#
# 以下为安装时自动扫描到的游戏，您也可以手动增删
EOF
    if [ -s "$TEMP_LIST" ]; then
        cat "$TEMP_LIST" >> "$GAMES_CONF"
    else
        echo "# （未检测到任何游戏，请手动添加）" >> "$GAMES_CONF"
    fi
    cat >> "$GAMES_CONF" <<'EOF'

# ---------- 常用游戏示例（取消注释即可使用） ----------
# com.tencent.tmgp.sgame          # 王者荣耀
# com.miHoYo.GenshinImpact        # 原神
# com.tencent.tmgp.pubgmhd        # 和平精英
# com.activision.callofduty.shooter # 使命召唤手游
# com.netease.dwrg                # 第五人格
# com.kurogame.mingchao           # 鸣潮
# com.miHoYo.hkrpg rate=8         # 崩坏：星穹铁道（独立采样率示例）
EOF
    echo "✅ 已生成配置文件，包含自动扫描到的游戏"

# 清理临时文件
rm -f "$TEMP_LIST"
chmod 644 "$GAMES_CONF"

echo "- 安装完成！"
echo "- 模块将根据 /data/games.conf 中的配置检测游戏并切换采样率"
echo "- 修改配置文件后无需重启，最多 5 秒生效"
echo "- 如遇问题请查看日志：/data/adb/modules/ColorOSTouchOptimize/common/touch_opt.log"

# 删除本安装脚本，不在模块目录保留
rm -f "$MODPATH/customize.sh"
exit 0