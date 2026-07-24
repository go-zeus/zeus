package signal

import (
	"os"
	"syscall"
)

// Shutdown 返回触发服务优雅关闭的信号
//
// 信号清单（与容器/K8s/POSIX 语义对齐）：
//   - SIGTERM：K8s/docker stop 默认信号，容器编排的"优雅停止"标准
//   - SIGINT：Ctrl+C，本地开发场景
//   - SIGQUIT：Ctrl+\，Unix 风格"强退 + dump stack"（Go runtime 默认行为）
//
// 不包含的信号及原因：
//   - SIGHUP：传统 Unix 重载信号（nginx -s reload / 容器 reload），语义为"重载配置"
//     而非"停止进程"。若纳入关闭列表，reload 场景会误杀进程，故不纳入。
//   - SIGKILL（kill -9）：OS 不允许捕获，加入此列表会被 Go signal 包静默忽略，
//     且让调用方误以为能优雅处理 KILL → 误导。如需"强制停止"，直接 kill -9 即可。
//   - SIGABRT/SIGSEGV：runtime 异常，由 Go panic recover 路径处理，不属于"主动停止"
func Shutdown() []os.Signal {
	return []os.Signal{
		syscall.SIGTERM,
		syscall.SIGINT,
		syscall.SIGQUIT,
	}
}
