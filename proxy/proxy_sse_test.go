package proxy

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestSSE_EventsInOrder 验证 SSE 事件按顺序到达且无缓冲
func TestSSE_EventsInOrder(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)
		// 串行写 3 条事件
		for i := 1; i <= 3; i++ {
			fmt.Fprintf(w, "data: event-%d\n\n", i)
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(50 * time.Millisecond) // 模拟流式产生
		}
	}))
	defer backend.Close()

	target, _ := url.Parse(backend.URL)
	p := New(WithSelector(NewStaticSelector(target)))
	proxySrv := httptest.NewServer(p)
	defer proxySrv.Close()

	req, _ := http.NewRequest("GET", proxySrv.URL+"/events", nil)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", resp.Header.Get("Cache-Control"))
	}

	// 按行读取，验证事件顺序
	scanner := bufio.NewScanner(resp.Body)
	var got []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			got = append(got, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
		if len(got) == 3 {
			break
		}
	}

	want := []string{"event-1", "event-2", "event-3"}
	if len(got) != 3 {
		t.Fatalf("events count = %d, want 3, got %v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestSSE_ClientDisconnect_ClosesBackend 验证客户端断开后后端连接关闭
//
// 这是 SSE 代理的关键生产特性：长连接必须有正确的清理机制
func TestSSE_ClientDisconnect_ClosesBackend(t *testing.T) {
	var backendDone int32

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer atomic.StoreInt32(&backendDone, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return // 客户端断开
			case <-ticker.C:
				fmt.Fprintf(w, "data: ping\n\n")
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
	}))
	defer backend.Close()

	target, _ := url.Parse(backend.URL)
	p := New(WithSelector(NewStaticSelector(target)))
	proxySrv := httptest.NewServer(p)
	defer proxySrv.Close()

	// 创建可取消的请求
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", proxySrv.URL+"/events", nil)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}

	// 读取 2 条事件后取消
	scanner := bufio.NewScanner(resp.Body)
	count := 0
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "data:") {
			count++
		}
		if count >= 2 {
			break
		}
	}

	// 客户端断开
	cancel()
	resp.Body.Close()

	// 等待后端感知断开
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&backendDone) == 1 {
			return // 成功
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("backend goroutine should exit after client disconnect")
}
