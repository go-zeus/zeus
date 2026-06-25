package proxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestNew_RequiresSelector 验证缺失 Selector 时 panic
// 这是编程错误，应当 fail-fast
func TestNew_RequiresSelector(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when WithSelector is missing")
		}
	}()
	_ = New()
}

// TestServeHTTP_ProtocolDetection 验证协议嗅探分流
func TestServeHTTP_ProtocolDetection(t *testing.T) {
	target, _ := url.Parse("http://127.0.0.1:1")
	p := New(WithSelector(NewStaticSelector(target))).(*proxy)

	tests := []struct {
		name    string
		req     *http.Request
		wantWS  bool
		wantSSE bool
	}{
		{
			name:    "plain HTTP",
			req:     httptest.NewRequest("GET", "/api", nil),
			wantWS:  false,
			wantSSE: false,
		},
		{
			name: "websocket upgrade",
			req: func() *http.Request {
				r := httptest.NewRequest("GET", "/ws", nil)
				r.Header.Set("Connection", "Upgrade")
				r.Header.Set("Upgrade", "websocket")
				return r
			}(),
			wantWS: true,
		},
		{
			name: "websocket upgrade case insensitive",
			req: func() *http.Request {
				r := httptest.NewRequest("GET", "/ws", nil)
				r.Header.Set("Connection", "upgrade, keep-alive")
				r.Header.Set("Upgrade", "WEBSOCKET")
				return r
			}(),
			wantWS: true,
		},
		{
			name: "sse accept",
			req: func() *http.Request {
				r := httptest.NewRequest("GET", "/events", nil)
				r.Header.Set("Accept", "text/event-stream")
				return r
			}(),
			wantSSE: true,
		},
		{
			name: "sse accept with other types",
			req: func() *http.Request {
				r := httptest.NewRequest("GET", "/events", nil)
				r.Header.Set("Accept", "text/html, text/event-stream, */*")
				return r
			}(),
			wantSSE: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWS := isWebSocketUpgrade(tt.req)
			gotSSE := isSSERequest(tt.req)
			if gotWS != tt.wantWS {
				t.Errorf("isWebSocketUpgrade = %v, want %v", gotWS, tt.wantWS)
			}
			if gotSSE != tt.wantSSE {
				t.Errorf("isSSERequest = %v, want %v", gotSSE, tt.wantSSE)
			}
			_ = p
		})
	}
}

// TestServeHTTP_HTTPFallback 验证普通 HTTP 请求走 fallback
func TestServeHTTP_HTTPFallback(t *testing.T) {
	// 启动后端服务器
	var gotPath, gotQuery, gotForwarded string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotForwarded = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend"))
	}))
	defer backend.Close()

	target, _ := url.Parse(backend.URL)
	p := New(WithSelector(NewStaticSelector(target)))

	// 通过代理发起请求
	proxySrv := httptest.NewServer(p)
	defer proxySrv.Close()

	resp, err := http.Post(proxySrv.URL+"/api/data?foo=bar", "text/plain", strings.NewReader(""))
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	if gotPath != "/api/data" {
		t.Errorf("backend path = %q, want /api/data", gotPath)
	}
	if gotQuery != "foo=bar" {
		t.Errorf("backend query = %q, want foo=bar", gotQuery)
	}
	if gotForwarded == "" {
		t.Error("X-Forwarded-For should be injected")
	}
}
