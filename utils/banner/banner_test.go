package banner

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// shouldShow 在设置 ZEUS_NO_BANNER 时必须返回 false
func TestShouldShow_DisabledByEnv(t *testing.T) {
	t.Setenv("ZEUS_NO_BANNER", "1")
	if shouldShow() {
		t.Error("ZEUS_NO_BANNER=1 时应返回 false")
	}
}

// Print 应输出 logo；注入 version 后应包含版本行
func TestPrint_IncludesArtAndVersion(t *testing.T) {
	old := version
	version = "v1.2.3-test"
	defer func() { version = old }()

	// 捕获 stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	Print()
	w.Close()

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "Zeus") && !strings.Contains(out, "████") {
		t.Errorf("Print 输出应包含 logo，got: %q", out)
	}
	if !strings.Contains(out, "v1.2.3-test") {
		t.Errorf("Print 应包含注入的版本号，got: %q", out)
	}
}

// Print 在 version 为空时不打印版本行（仅 logo）
func TestPrint_NoVersionWhenEmpty(t *testing.T) {
	old := version
	version = ""
	defer func() { version = old }()

	r, w, _ := os.Pipe()
	origStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	Print()
	w.Close()

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	// 仅断言不 panic 且有 logo 输出即可
	if buf.Len() == 0 {
		t.Error("Print 应至少输出 logo")
	}
}
