<div align="center">
# 🗡️ ColorOS 触控优化
**基于 Go 静态二进制的 ColorOS 触控采样率动态调度引擎**
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![GitHub release](https://img.shields.io/github/v/release/WuGanTeam/ColorOSTouchOptimize)](https://github.com/WuGanTeam/ColorOSTouchOptimize/releases)
[![GitHub stars](https://img.shields.io/github/stars/WuGanTeam/ColorOSTouchOptimize)](https://github.com/WuGanTeam/ColorOSTouchOptimize)
[![Support: Magisk & KernelSU](https://img.shields.io/badge/Support-Magisk%20%26%20KernelSU-blue)](https://github.com/WuGanTeam/ColorOSTouchOptimize)
</div>
---
## ✨ 简介
根据**前台运行的游戏**自动切换触控采样率，玩游戏时获得更极致的响应速度，日常使用自动恢复省电模式。
- 🎮 **游戏高采样率**：默认 360Hz，毫秒级响应
- 🔋 **日用低采样率**：默认 0（系统默认 ~120Hz），省电续航
- 🧠 **Activity 级识别**：精准判断前台应用，不误触发
- ⚙️ **全配置化**：采样率、触控节点、每应用独立策略均可自定义
- 🛡️ **守护自愈**：进程意外退出自动重启，写入失败自动重试+熔断
- 🔁 **保活对抗**：周期性强制重写采样率，对抗系统/游戏助手覆盖
- 🛑 **stop horae**：自动停止 ColorOS 采样率管家，防止它抢回控制权
- 📜 **分级日志**：调试方便，日志自动轮转
## 📱 支持设备

- OPPO / OnePlus / Realme 等搭载 **ColorOS 12+** 的设备
- 需要系统内置 `touchHidlTest` 命令（通常存在于 ColorOS 系统）
- **arm64** 架构
## 🔧 安装
1. 从 [Releases](https://github.com/WuGanTeam/ColorOSTouchOptimize/releases) 下载最新的 `ColorOSTouchOptimize-vX.X.zip`
2. 在 **Magisk** 或 **KernelSU** 中「从本地安装/刷入」该模块
3. 重启设备（或根据提示）
4. 安装时自动生成 `/data/games.conf` 配置并扫描游戏列表
> ✅ 支持 **在线更新**：模块内置 `updateJson`，新版本发布后可直接在应用内点击更新。
## ⚙️ 配置说明
所有配置集中在 `/data/games.conf`，安装时自动生成。编辑后**无需重启**，5 秒内自动生效。
```
# 全局配置（必需，仅一行）
config 游戏采样率=360, 日用采样率=0, touch_node=182, stop_horae=1
# 游戏列表（每行一个包名）
com.tencent.tmgp.sgame           # 使用全局游戏采样率
com.miHoYo.hkrpg rate=360        # 该游戏使用独立采样率 360
```
| 参数 | 说明 | 默认值 |
|------|------|--------|
| `游戏采样率`/`game_rate` | 游戏运行时采样率（直接填 Hz 数值） | 360 |
| `日用采样率`/`default_rate` | 日常采样率（直接填 Hz 数值，0=系统默认） | 0 |
| `touch_node` | 触控节点号 | 182 |
| `stop_horae` | 是否停止 ColorOS 采样率管家（1=开 0=关） | 1 |
> ⚠️ **重要**：采样率直接填 **Hz 数值**（如 360、320、240、120），不要填旧版索引值 6/8。
> 一加 Ace5 Pro（S3910 触控 IC）实测：写 360 → 360Hz，写 0 → 系统默认 ~120Hz。
> 以 `#` 开头的行是注释。
## 📂 文件说明
| 文件 | 作用 |
|------|------|
| `common/touch_opt` | Go 编译的静态守护进程二进制 |
| `service.sh` | 启动守护进程 + 自愈监控 |
| `customize.sh` | 安装流程、权限、兼容性检查 |
| `action.sh` | 打开 MT 管理器编辑配置文件 |
| `module.prop` | 模块信息 + 在线更新地址 |
## 📜 日志
- 文件：`/data/adb/modules/ColorOSTouchOptimize/common/touch_opt.log`（>1MB 自动轮转）
- Logcat：`logcat | grep TouchOpt`
## 🗑️ 卸载
卸载时自动停止守护进程、清理配置与日志，无残留。
## 📝 更新日志
详见 [CHANGELOG.md](CHANGELOG.md)
## 📄 许可证
本项目基于 [MIT](LICENSE) 协议开源。