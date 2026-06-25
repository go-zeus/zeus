package signal

import (
	"os"
	"syscall"
	"testing"
)

// TestShutdown 验证返回的信号列表包含预期的关闭信号
func TestShutdown(t *testing.T) {
	sigs := Shutdown()

	wantSigs := []os.Signal{
		syscall.SIGTERM,
		syscall.SIGINT,
		syscall.SIGQUIT,
		syscall.SIGHUP,
	}

	for _, want := range wantSigs {
		found := false
		for _, sig := range sigs {
			if sig == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("期望信号列表中包含 %v", want)
		}
	}

	// 同时验证列表长度
	if len(sigs) != 4 {
		t.Errorf("期望返回 4 个信号, 实际 = %d", len(sigs))
	}
}

// TestShutdown_NotContainsSIGKILL 验证列表不含 SIGKILL（不可被捕获，加入反而误导）
func TestShutdown_NotContainsSIGKILL(t *testing.T) {
	sigs := Shutdown()
	for _, sig := range sigs {
		if sig == syscall.SIGKILL {
			t.Error("Shutdown 不应包含 SIGKILL：OS 不允许捕获，加入列表会误导调用方")
		}
	}
}
