// ============================================================
// ColorOS 触控优化模块 - 核心守护进程（Go 二进制版）
// 版本：v3.0.0_go
// 功能：
//   - 前台 Activity 检测（优先）+ 进程检测（回退）
//   - 触控节点、采样率值全配置化
//   - 支持按应用独立采样率（rate=）
//   - 开机强制校准
//   - 日志分级 + logcat 输出 + 日志轮转
//   - 单实例守护（PID 文件）
// 构建：
//   GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-s -w" -o touch_opt main.go
// ============================================================

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ---------------- 常量 ---------------
const (
	ConfDir      = "/data/adb/modules/ColorOSTouchOptimize/common"
	LogFile      = ConfDir + "/touch_opt.log"
	GamesConf    = "/data/games.conf"
	PidFile      = ConfDir + "/touch_opt.pid"
	NoGameMarker = "__NO_GAME_PROCESS__"

	PollInterval    = 5 * time.Second
	RequiredConfirm = 2 // 连续确认次数才切游戏
	RequiredLost    = 3 // 连续丢失次数才切回日用
	MaxWriteFail    = 3 // 连续写入失败熔断上限
	RetryTimes      = 3 // 单次写入重试次数

	KeepAliveEvery = 3 // 每 N 轮循环（N*5 秒）强制保活重写，对抗系统/游戏助手覆盖
)

// ---------------- 全局状态 ---------------
type PkgEntry struct {
	Pkg     string
	Rate    int
	HasRate bool
}

type AppConfig struct {
	TouchNode   int
	GameRate    int
	DefaultRate int
	StopHorae   bool
	Packages    []PkgEntry
	Pattern     string // "|" 分隔的包名，用于日志
}

var (
	cfg          AppConfig
	lastMtime    int64  = -1
	currentMode  string = "default"
	activePkg    string = ""
	confirmCount int    = 0
	lostCount    int    = 0
	writeFail    int    = 0
	keepAliveCnt int    = 0

	// 上次成功写入的（值,描述），用于保活判断与日志去重
	lastApplied int    = -1
	lastDesc    string = ""

	// 前台包名正则：匹配 "com.example.app/" 或 "com.example.app/Activity"
	frontRe = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z0-9_]+)+)/[a-zA-Z0-9_.]+`)
)

// ---------------- 基础工具 ---------------

// runCmd 执行命令并返回去除首尾空白后的 stdout
func runCmd(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// runOK 执行命令，返回是否成功（exit 0）
func runOK(name string, args ...string) bool {
	return exec.Command(name, args...).Run() == nil
}

// logMsg 分级日志：写文件 + logcat
func logMsg(level, msg string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("%s [%s] %s\n", ts, level, msg)

	if f, err := os.OpenFile(LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		f.WriteString(line)
		f.Close()
	}
	// logcat（tag=TouchOpt），2>/dev/null 静默失败
	exec.Command("log", "-t", "TouchOpt", "-p", strings.ToLower(level[:1]), msg).Run()
}

func logI(msg string) { logMsg("INFO", msg) }
func logW(msg string) { logMsg("WARN", msg) }
func logE(msg string) { logMsg("ERROR", msg) }

// rotateLog 日志轮转：>1MB 移动为 .old
func rotateLog() {
	if st, err := os.Stat(LogFile); err == nil && st.Size() > 1<<20 {
		os.Rename(LogFile, LogFile+".old")
		logI("日志轮转（旧日志已备份）")
	}
}

// ensurePID 单实例保护：写入 PID；若已有存活实例则返回 false
func ensurePID() bool {
	mypid := os.Getpid()
	if data, err := os.ReadFile(PidFile); err == nil {
		old, perr := strconv.Atoi(strings.TrimSpace(string(data)))
		// 仅当旧 PID 存在、存活且不是自己时才视为冲突
		if perr == nil && old > 0 && old != mypid {
			if runOK("kill", "-0", strconv.Itoa(old)) {
				logW(fmt.Sprintf("发现已运行的实例 PID=%d，本进程退出", old))
				return false
			}
		}
	}
	// 写入/覆盖 PID 文件为当前进程 PID
	if err := os.WriteFile(PidFile, []byte(strconv.Itoa(mypid)), 0644); err != nil {
		logE("无法写入 PID 文件: " + err.Error())
		return false
	}
	return true
}

func cleanupPID() {
	os.Remove(PidFile)
}

// ---------------- 配置解析 ---------------

// parseValue 从 "key=value" 中取数值，非法返回 -1
func parseValue(part string) int {
	idx := strings.Index(part, "=")
	if idx < 0 {
		return -1
	}
	v := strings.TrimSpace(part[idx+1:])
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return -1
	}
	return n
}

func parseConfig() {
	var nc AppConfig
	nc.TouchNode = 182
	nc.GameRate = 360
	nc.DefaultRate = 360
	nc.StopHorae = true

	pkgList := []string{}

	data, err := os.ReadFile(GamesConf)
	if err != nil {
		nc.Pattern = NoGameMarker
		cfg = nc
		return
	}

	lines := strings.Split(string(data), "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// ---- config 行 ----
		if strings.HasPrefix(line, "config") {
			body := strings.TrimSpace(strings.TrimPrefix(line, "config"))
			parts := strings.Split(body, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				switch {
				case strings.HasPrefix(p, "游戏采样率="), strings.HasPrefix(p, "game_rate="):
					if v := parseValue(p); v >= 0 {
						nc.GameRate = v
					}
				case strings.HasPrefix(p, "日用采样率="), strings.HasPrefix(p, "default_rate="):
					if v := parseValue(p); v >= 0 {
						nc.DefaultRate = v
					}
				case strings.HasPrefix(p, "touch_node="):
					if v := parseValue(p); v >= 0 {
						nc.TouchNode = v
					}
				case strings.HasPrefix(p, "stop_horae="):
					if v := parseValue(p); v >= 0 {
						nc.StopHorae = (v == 1)
					}
				}
			}
			continue
		}

		// ---- 包名行（支持 "pkg" 或 "pkg rate=N"）----
		pkg := line
		rate := -1
		if fields := strings.Fields(line); len(fields) >= 2 {
			pkg = fields[0]
			for _, f := range fields[1:] {
				if strings.HasPrefix(f, "rate=") {
					rate = parseValue(f)
				}
			}
		}
		if pkg == "" {
			continue
		}
		entry := PkgEntry{Pkg: pkg}
		if rate >= 0 {
			entry.Rate = rate
			entry.HasRate = true
		}
		nc.Packages = append(nc.Packages, entry)
		pkgList = append(pkgList, pkg)
	}

	if len(pkgList) > 0 {
		nc.Pattern = strings.Join(pkgList, "|")
	} else {
		nc.Pattern = NoGameMarker
	}

	cfg = nc

	if st, err := os.Stat(GamesConf); err == nil {
		lastMtime = st.ModTime().Unix()
	}
	logI(fmt.Sprintf("配置已解析：节点=%d, 游戏采样率=%d, 日用采样率=%d, stop_horae=%v, 匹配包数=%d",
		cfg.TouchNode, cfg.GameRate, cfg.DefaultRate, cfg.StopHorae, len(cfg.Packages)))
}

// getPerAppRate 返回指定包名的独立采样率；无则返回 -1
func getPerAppRate(pkg string) int {
	for _, e := range cfg.Packages {
		if e.HasRate && e.Pkg == pkg {
			return e.Rate
		}
	}
	return -1
}

// ---------------- 系统状态检测 ---------------

// getFrontApp 获取前台应用包名（Activity 优先，Window 回退）
func getFrontApp() string {
	// 方案1：dumpsys activity activities
	out := runCmd("dumpsys", "activity", "activities")
	if out != "" {
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "topResumedActivity") || strings.Contains(line, "mResumedActivity") {
				if m := frontRe.FindStringSubmatch(line); len(m) > 1 {
					return m[1]
				}
			}
		}
	}
	// 方案2：dumpsys window
	out = runCmd("dumpsys", "window")
	if out != "" {
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "mCurrentFocus") || strings.Contains(line, "mFocusedApp") {
				if m := frontRe.FindStringSubmatch(line); len(m) > 1 {
					return m[1]
				}
			}
		}
	}
	return ""
}

// isScreenOn 屏幕状态检测
func isScreenOn() bool {
	out := runCmd("dumpsys", "power")
	if strings.Contains(out, "mWakefulness=Awake") {
		return true
	}
	// 回退：亮度检测
	matches, _ := filepath.Glob("/sys/class/backlight/*/brightness")
	for _, p := range matches {
		if data, err := os.ReadFile(p); err == nil {
			v := strings.TrimSpace(string(data))
			if v != "" && v != "0" {
				return true
			}
		}
	}
	return false
}

// processAlive 进程是否存在
func processAlive(pkg string) bool {
	if runCmd("pidof", pkg) != "" {
		return true
	}
	if runOK("pgrep", "-f", pkg) {
		return true
	}
	return false
}

// checkGame 检测游戏：前台优先 + 进程回退；命中返回 *PkgEntry，否则 nil
func checkGame() *PkgEntry {
	if cfg.Pattern == NoGameMarker || len(cfg.Packages) == 0 {
		return nil
	}

	// 1. 前台 Activity 精确匹配
	if front := getFrontApp(); front != "" {
		for i := range cfg.Packages {
			if front == cfg.Packages[i].Pkg {
				activePkg = cfg.Packages[i].Pkg
				return &cfg.Packages[i]
			}
		}
		// 2. 前缀匹配：配置 com.tencent.tmgp，前台 com.tencent.tmgp.sgame
		for i := range cfg.Packages {
			p := cfg.Packages[i].Pkg
			if strings.HasPrefix(front, p+".") {
				activePkg = p
				return &cfg.Packages[i]
			}
		}
	}

	// 3. 进程检测回退
	for i := range cfg.Packages {
		if processAlive(cfg.Packages[i].Pkg) {
			activePkg = cfg.Packages[i].Pkg
			return &cfg.Packages[i]
		}
	}

	activePkg = ""
	return nil
}

// ---------------- 采样率写入 ---------------

// stopHorae 停止 ColorOS 采样率管理服务（horae），防止其覆盖我们的设置
func stopHorae() {
	if !cfg.StopHorae {
		return
	}
	// 检查服务是否在运行
	out := runCmd("getprop", "init.svc.horae")
	if out == "running" {
		if runOK("stop", "horae") {
			logI("已停止 horae 服务（防止系统覆盖采样率）")
		} else {
			logW("停止 horae 服务失败")
		}
	}
}

func writeRate(val int) bool {
	for retry := 0; retry < RetryTimes; retry++ {
		if runOK("touchHidlTest", "-c", "wo", "0", strconv.Itoa(cfg.TouchNode), strconv.Itoa(val)) {
			return true
		}
		time.Sleep(time.Second)
	}
	return false
}

// applySetting 写入采样率。
// force=false：仅当值与上次成功写入不同时才写；
// force=true：无论是否相同都强制重写（保活循环用）。
func applySetting(val int, desc string, force bool) bool {
	if !force && val == lastApplied {
		return true // 已应用相同值，跳过
	}
	// 值变化时先尝试停掉采样率管家
	if val != lastApplied {
		stopHorae()
	}
	if writeRate(val) {
		if val != lastApplied || desc != lastDesc {
			logI(fmt.Sprintf("写入成功 节点=%d 值=%d (%s)", cfg.TouchNode, val, desc))
		}
		lastApplied = val
		lastDesc = desc
		writeFail = 0
		return true
	}
	logE(fmt.Sprintf("写入失败 节点=%d 值=%d (%s)，重试 %d 次后仍失败", cfg.TouchNode, val, desc, RetryTimes))
	writeFail++
	if writeFail >= MaxWriteFail {
		logW("写入失败次数过多，暂停 60 秒")
		time.Sleep(60 * time.Second)
		writeFail = 0
	}
	return false
}

// readCurrentRate 读取当前采样率值；失败返回 -1
func readCurrentRate() int {
	out := runCmd("touchHidlTest", "-c", "ro", "0", strconv.Itoa(cfg.TouchNode))
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if n, err := strconv.Atoi(line); err == nil {
			return n
		}
	}
	return -1
}

// calibrateDefault 开机强制校准日用采样率
func calibrateDefault() {
	cur := readCurrentRate()
	if cur < 0 {
		logW("无法读取当前采样率值，跳过校准")
		return
	}
	if cur != cfg.DefaultRate {
		logI(fmt.Sprintf("校准：当前值=%d ≠ 日用值=%d，强制写入", cur, cfg.DefaultRate))
		applySetting(cfg.DefaultRate, "开机校准-日用", false)
	} else {
		logI(fmt.Sprintf("校准通过：当前值=%d = 日用值=%d", cur, cfg.DefaultRate))
	}
}

// ---------------- 主入口 ----------------

func main() {
	os.MkdirAll(ConfDir, 0755)

	logI("=== touch_opt (Go) 守护进程启动 PID=" + strconv.Itoa(os.Getpid()) + " ===")
	defer cleanupPID()

	if !ensurePID() {
		return
	}

	// 等待系统完全启动
	logI("等待系统完全启动...")
	for runCmd("getprop", "sys.boot_completed") != "1" {
		time.Sleep(2 * time.Second)
	}
	// 额外等待 10 秒，让系统服务就绪
	time.Sleep(10 * time.Second)
	logI("系统已完全启动")

	if _, err := exec.LookPath("touchHidlTest"); err != nil {
		logE("未找到 touchHidlTest，退出")
		return
	}

	// 首次解析配置 + 开机校准
	parseConfig()
	calibrateDefault()
	currentMode = "default"

	// 主循环
	for {
		rotateLog()

		// ---- 配置热重载（mtime 检测）----
		if st, err := os.Stat(GamesConf); err == nil {
			mt := st.ModTime().Unix()
			if mt != lastMtime {
				logI("检测到配置文件变更，重新解析")
				parseConfig()
				// 配置变更后立即应用当前模式的采样率（不再等待下一轮切换）
				if currentMode == "game" {
					applySetting(cfg.GameRate, "全局游戏采样率", true)
				} else {
					applySetting(cfg.DefaultRate, "日用模式", true)
				}
			}
		} else {
			if lastMtime != -1 {
				cfg.Pattern = NoGameMarker
				cfg.Packages = nil
				lastMtime = -1
				logW("配置文件被删除，停止游戏检测")
			}
		}

		// ---- 屏幕熄灭时跳过 ----
		if !isScreenOn() {
			time.Sleep(PollInterval)
			continue
		}

		// ---- 游戏检测与切换 ----
		entry := checkGame()
		if entry != nil {
			confirmCount++
			lostCount = 0
			if confirmCount >= RequiredConfirm && currentMode != "game" {
				target := cfg.GameRate
				desc := "全局游戏采样率"
				if per := getPerAppRate(entry.Pkg); per >= 0 {
					target = per
					desc = "应用独立采样率(" + entry.Pkg + ")"
				}
				logI(fmt.Sprintf("确认游戏启动（%s），切换至 %s=%d", entry.Pkg, desc, target))
				applySetting(target, desc, false)
				currentMode = "game"
				confirmCount = 0
			}
		} else {
			lostCount++
			confirmCount = 0
			if lostCount >= RequiredLost && currentMode != "default" {
				logI(fmt.Sprintf("游戏已退出，恢复日用采样率=%d", cfg.DefaultRate))
				applySetting(cfg.DefaultRate, "日用模式", false)
				currentMode = "default"
				lostCount = 0
			}
		}

		// ---- 保活：周期性强制重写当前模式值，对抗系统/游戏助手覆盖 ----
		keepAliveCnt++
		if keepAliveCnt >= KeepAliveEvery {
			keepAliveCnt = 0
			if currentMode == "game" {
				applySetting(cfg.GameRate, "全局游戏采样率", true)
			} else {
				applySetting(cfg.DefaultRate, "日用模式", true)
			}
		}

		time.Sleep(PollInterval)
	}
}
