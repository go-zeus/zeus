package app

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	zeushttp "github.com/go-zeus/zeus/server/http"
)

// TestWithPprof_AddsPprofServer WithPprof 应把 pprof server 加入 servers
func TestWithPprof_AddsPprofServer(t *testing.T) {
	cfg := &appConfig{
		name:        defaultServiceName,
		cluster:     defaultServiceCluster,
		stopTimeout: defaultStopTimeoutL3,
	}
	opt := WithPprof(6060)
	opt(cfg)
	if len(cfg.servers) != 1 {
		t.Fatalf("expected 1 pprof server, got %d", len(cfg.servers))
	}
}

// TestWithPprof_NegativePortIgnored port <= 0 应被忽略
func TestWithPprof_NegativePortIgnored(t *testing.T) {
	cfg := &appConfig{}
	WithPprof(0)(cfg)
	WithPprof(-1)(cfg)
	if len(cfg.servers) != 0 {
		t.Errorf("non-positive port should be ignored, got %d servers", len(cfg.servers))
	}
}

// TestWithPprof_PprofEndpoints 端到端验证 pprof 端点可访问
func TestWithPprof_PprofEndpoints(t *testing.T) {
	appPort := freePort(t)
	pprofPort := freePort(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/hi", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hi"))
	})

	a := NewApp(
		AddServer(zeushttp.NewHTTP(zeushttp.Port(appPort), zeushttp.Mux(mux))),
		WithPprof(pprofPort),
		WithServiceName("test-pprof"),
	)

	errCh := make(chan error, 1)
	go func() { errCh <- a.Run() }()

	// 等 pprof 端口就绪
	pprofURL := fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", pprofPort)
	waitForReady(t, pprofURL, 3*time.Second)

	// 业务端口也应可用
	appURL := fmt.Sprintf("http://127.0.0.1:%d/hi", appPort)
	waitForReady(t, appURL, 3*time.Second)

	// —— pprof 索引页应包含 "goroutine" 等关键字 ——
	resp, err := http.Get(pprofURL)
	if err != nil {
		t.Fatalf("Get pprof index: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	bodyStr := string(body)
	for _, want := range []string{"goroutine", "heap", "threadcreate"} {
		if !strings.Contains(bodyStr, want) {
			t.Errorf("pprof index should mention %q, body: %q", want, bodyStr)
		}
	}

	// —— 各命名 profile 端点应返回 200 ——
	for _, path := range []string{
		"/debug/pprof/goroutine",
		"/debug/pprof/heap",
		"/debug/pprof/allocs",
		"/debug/pprof/mutex",
		"/debug/pprof/block",
		"/debug/pprof/threadcreate",
		"/debug/pprof/cmdline",
		"/debug/pprof/symbol",
	} {
		url := fmt.Sprintf("http://127.0.0.1:%d%s", pprofPort, path)
		resp, err := http.Get(url)
		if err != nil {
			t.Errorf("Get %s: %v", path, err)
			continue
		}
		if resp.StatusCode != 200 {
			t.Errorf("%s status = %d, want 200", path, resp.StatusCode)
		}
		resp.Body.Close()
	}

	signalShutdown(t)
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s after signal")
	}
}

// TestWithPprof_BusinessPortUnaffectedWithPprof 启动 pprof 后业务端口行为不变
func TestWithPprof_BusinessPortUnaffected(t *testing.T) {
	appPort := freePort(t)
	pprofPort := freePort(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/biz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("business"))
	})

	a := NewApp(
		AddServer(zeushttp.NewHTTP(zeushttp.Port(appPort), zeushttp.Mux(mux))),
		WithPprof(pprofPort),
	)

	errCh := make(chan error, 1)
	go func() { errCh <- a.Run() }()

	// 业务端口正常
	appURL := fmt.Sprintf("http://127.0.0.1:%d/biz", appPort)
	waitForReady(t, appURL, 3*time.Second)

	resp, err := http.Get(appURL)
	if err != nil {
		t.Fatalf("Get biz: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "business" {
		t.Errorf("biz body = %q, want 'business'", body)
	}

	// 业务端口不应有 /debug/pprof（隔离验证）
	bizPprofURL := fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", appPort)
	resp2, err := http.Get(bizPprofURL)
	if err == nil {
		resp2.Body.Close()
		if resp2.StatusCode == 200 {
			t.Error("business port should NOT expose /debug/pprof/")
		}
	}

	signalShutdown(t)
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s after signal")
	}
}
