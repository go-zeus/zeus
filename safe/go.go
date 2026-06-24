package safe

import (
	"github.com/go-zeus/zeus/log"
	"runtime/debug"
)

// GO 启动带 panic 恢复的 goroutine
// 注意：fn 返回的 error 在 goroutine 中无法直接返回，仅用于约定 fn 签名
// 若需感知错误，请在 fn 内部处理（如记录日志、写入 channel）
func GO(fn func() error) {
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Error("Go panic:%v \n%s", err, debug.Stack())
			}
		}()
		_ = fn()
	}()
}
