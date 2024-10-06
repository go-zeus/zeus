package signal

import (
	"os"
	"syscall"
)

// Shutdown 返回所有要关闭服务的信号
func Shutdown() []os.Signal {
	return []os.Signal{
		syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGKILL,
	}
}
