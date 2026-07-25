// Package banner 在应用启动时输出 Zeus logo。
//
// 设计取舍：
//   - 仅在交互式终端（TTY）自动展示，避免污染管道/重定向/CI/日志采集/测试输出
//     （如 `app | jq` 不会被 logo 破坏 JSON 解析）
//   - 输出到 stderr，不污染 stdout 数据流
//   - 支持 ZEUS_NO_BANNER=1 强制关闭（生产/守护进程场景）
//   - version 可经 -ldflags "-X .../banner.version=v1.0.0" 注入，缺失则不打印版本行
//
// 显式调用：任何时机可 banner.Print() 主动展示（如自定义启动横幅）。
package banner

import (
	"fmt"
	"os"
)

// asciiArt Zeus 启动 logo
const asciiArt = " ██████   ██████        ███████ ███████ ██    ██ ███████ \n" +
	"██       ██    ██          ███  ██      ██    ██ ██      \n" +
	"██   ███ ██    ██ █████   ███   █████   ██    ██ ███████ \n" +
	"██    ██ ██    ██        ███    ██      ██    ██      ██ \n" +
	" ██████   ██████        ███████ ███████  ██████  ███████\n"

// version 可通过 ldflags 注入，便于发布版本展示：
//
//	go build -ldflags "-X github.com/go-zeus/zeus/utils/banner.version=v1.0.0"
//
// 默认空字符串（不打印版本行，仅展示 logo）。
var version = ""

// Print 输出 logo（及版本信息）到 stderr。
// 直接调用则无条件展示，不受 TTY / 环境变量控制。
func Print() {
	fmt.Fprint(os.Stderr, asciiArt)
	if version != "" {
		fmt.Fprintf(os.Stderr, "  Zeus %s\n", version)
	}
	fmt.Fprint(os.Stderr, "\n")
}

// shouldShow 判断是否应在启动时自动展示 logo：
//   - ZEUS_NO_BANNER 非空 → 关闭
//   - stdout 非字符设备（被管道/重定向，如 `app | jq`、CI、守护进程）→ 关闭
func shouldShow() bool {
	if os.Getenv("ZEUS_NO_BANNER") != "" {
		return false
	}
	if fi, err := os.Stdout.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	return true
}

func init() {
	if shouldShow() {
		Print()
	}
}
