package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// mustParseURL 解析 URL，测试辅助
func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}

// TestHTTP_ForwardPathAndQuery 验证 HTTP 代理转发 path 和 query
func TestHTTP_ForwardPathAndQuery(t *testing.T) {
	var gotPath, gotQuery string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	target := mustParseURL(t, backend.URL)
	p := New(WithSelector(NewStaticSelector(target)))
	proxySrv := httptest.NewServer(p)
	defer proxySrv.Close()

	resp, err := http.Get(proxySrv.URL + "/api/users?page=1&size=20")
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	if gotPath != "/api/users" {
		t.Errorf("path = %q, want /api/users", gotPath)
	}
	if gotQuery != "page=1&size=20" {
		t.Errorf("query = %q, want page=1&size=20", gotQuery)
	}
}

// TestHTTP_InjectsForwardedHeaders 验证默认 Director 注入 X-Forwarded-For/X-Real-IP/X-Request-ID
func TestHTTP_InjectsForwardedHeaders(t *testing.T) {
	var gotXFF, gotXRealIP, gotReqID string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotXFF = r.Header.Get("X-Forwarded-For")
		gotXRealIP = r.Header.Get("X-Real-IP")
		gotReqID = r.Header.Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	target := mustParseURL(t, backend.URL)
	p := New(WithSelector(NewStaticSelector(target)))
	proxySrv := httptest.NewServer(p)
	defer proxySrv.Close()

	resp, err := http.Get(proxySrv.URL + "/")
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	if gotXFF == "" {
		t.Error("X-Forwarded-For should be injected")
	}
	if gotXRealIP == "" {
		t.Error("X-Real-IP should be injected")
	}
	if gotReqID == "" {
		t.Error("X-Request-ID should be injected")
	}
}

// TestHTTP_BackendErrorPassThrough 验证后端错误状态码透传
func TestHTTP_BackendErrorPassThrough(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer backend.Close()

	target := mustParseURL(t, backend.URL)
	p := New(WithSelector(NewStaticSelector(target)))
	proxySrv := httptest.NewServer(p)
	defer proxySrv.Close()

	resp, err := http.Get(proxySrv.URL + "/")
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "internal error") {
		t.Errorf("body should contain backend error, got %q", string(body))
	}
}

// TestHTTP_CustomDirector 验证用户自定义 Director 叠加生效
func TestHTTP_CustomDirector(t *testing.T) {
	var gotCustom string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCustom = r.Header.Get("X-Custom-Header")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	target := mustParseURL(t, backend.URL)
	p := New(
		WithSelector(NewStaticSelector(target)),
		WithDirector(func(target *url.URL, req *http.Request) {
			req.Header.Set("X-Custom-Header", "injected-by-user")
		}),
	)
	proxySrv := httptest.NewServer(p)
	defer proxySrv.Close()

	resp, err := http.Get(proxySrv.URL + "/")
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	if gotCustom != "injected-by-user" {
		t.Errorf("custom header = %q, want injected-by-user", gotCustom)
	}
}

// TestHTTP_DefaultErrorHandler_502 验证 Selector 错误时返回 502
func TestHTTP_DefaultErrorHandler_502(t *testing.T) {
	// Selector 返回错误
	errSelector := &errorSelector{}
	p := New(WithSelector(errSelector))
	proxySrv := httptest.NewServer(p)
	defer proxySrv.Close()

	resp, err := http.Get(proxySrv.URL + "/")
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
}

// errorSelector 总是返回错误
type errorSelector struct{}

func (e *errorSelector) Pick(_ *http.Request) (*url.URL, error) {
	return nil, fmt.Errorf("no backend available")
}

// 编译期检查 errorSelector 实现 Selector 接口
var _ Selector = (*errorSelector)(nil)
