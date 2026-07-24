package signal

import (
	"os"
	"syscall"
	"testing"
)

// TestShutdown 验证返回的信号列表包含预期的关闭信号
func TestShutdown(t *testing.T) {
	sigs := Shutdown()

	// 仅包含真正的"停止"信号；SIGHUP 语义为 reload（重载配置），不纳入关闭列表
	wantSigs := []os.Signal{
		syscall.SIGTERM,
		syscall.SIGINT,
		syscall.SIGQUIT,
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

	// SIGHUP 不应出现（reload 信号，误纳入会致 reload 场景误杀进程）
	for _, sig := range sigs {
		if sig == syscall.SIGHUP {
			t.Error("Shutdown 不应包含 SIGHUP：语义为 reload 而非 shutdown")
		}
	}

	// 同时验证列表长度
	if len(sigs) != 3 {
		t.Errorf("期望返回 3 个信号, 实际 = %d", len(sigs))
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
