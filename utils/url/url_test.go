package url

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestJoinPaths 表格驱动测试各种路径合并场景
func TestJoinPaths(t *testing.T) {
	tests := []struct {
		name         string
		absolutePath string
		relativePath string
		want         string
	}{
		{"空相对路径", "/api", "", "/api"},
		{"带 scheme", "http://example.com/api", "/v1", "http://example.com/api/v1"},
		{"无 scheme", "/api", "/v1", "/api/v1"},
		{"相对路径带尾部斜杠", "/api", "/v1/", "/api/v1/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := JoinPaths(tt.absolutePath, tt.relativePath)
			if got != tt.want {
				t.Errorf("JoinPaths(%q, %q) = %q, 期望 %q", tt.absolutePath, tt.relativePath, got, tt.want)
			}
		})
	}
}

// TestSingleJoiningSlash 表格驱动测试斜杠合并
func TestSingleJoiningSlash(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want string
	}{
		{"两端都有斜杠", "http://host/", "/path", "http://host/path"},
		{"两端都没有斜杠", "http://host", "path", "http://host/path"},
		{"只有 a 有尾部斜杠", "http://host/", "path", "http://host/path"},
		{"只有 b 有前缀斜杠", "http://host", "/path", "http://host/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SingleJoiningSlash(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("SingleJoiningSlash(%q, %q) = %q, 期望 %q", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestClientIP_XForwardedFor 验证 X-Forwarded-For 头解析
func TestClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")

	got := ClientIP(req)
	if got != "1.2.3.4" {
		t.Errorf("ClientIP() = %q, 期望 %q", got, "1.2.3.4")
	}
}

// TestClientIP_XRealIp 验证 X-Real-Ip 头解析
func TestClientIP_XRealIp(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-Ip", "9.8.7.6")

	got := ClientIP(req)
	if got != "9.8.7.6" {
		t.Errorf("ClientIP() = %q, 期望 %q", got, "9.8.7.6")
	}
}

// TestClientIP_RemoteAddr 无 header 时从 RemoteAddr 解析
func TestClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"

	got := ClientIP(req)
	if got != "10.0.0.1" {
		t.Errorf("ClientIP() = %q, 期望 %q", got, "10.0.0.1")
	}
}

// TestURL_HTTP 无 TLS 时返回 http:// 前缀
func TestURL_HTTP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "example.com"

	got := URL(req)
	if got != "http://example.com" {
		t.Errorf("URL() = %q, 期望 %q", got, "http://example.com")
	}
}

// TestURL_HTTPS 有 TLS 时返回 https:// 前缀
func TestURL_HTTPS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "example.com"
	req.TLS = &tls.ConnectionState{}

	got := URL(req)
	if got != "https://example.com" {
		t.Errorf("URL() = %q, 期望 %q", got, "https://example.com")
	}
}

// —— Forwarded (RFC 7239) 支持 ——

// TestClientIP_Forwarded_RFC7239 RFC 7239 Forwarded header 解析
func TestClientIP_Forwarded_RFC7239(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Forwarded", "for=192.0.2.43")

	got := ClientIP(req)
	if got != "192.0.2.43" {
		t.Errorf("ClientIP() = %q, 期望 %q", got, "192.0.2.43")
	}
}

// TestClientIP_Forwarded_WithPort 带 IPv4 端口
func TestClientIP_Forwarded_WithPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Forwarded", `for="192.0.2.43:8080"`)

	got := ClientIP(req)
	if got != "192.0.2.43" {
		t.Errorf("ClientIP() = %q, 期望 %q（去端口）", got, "192.0.2.43")
	}
}

// TestClientIP_Forwarded_IPv6 IPv6 地址
func TestClientIP_Forwarded_IPv6(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Forwarded", `for="[2001:db8::1]:8080"`)

	got := ClientIP(req)
	if got != "2001:db8::1" {
		t.Errorf("ClientIP() = %q, 期望 %q（IPv6 去括号去端口）", got, "2001:db8::1")
	}
}

// TestClientIP_Forwarded_MultiHop 多 hop 链路取最远端
func TestClientIP_Forwarded_MultiHop(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Forwarded", "for=192.0.2.43, for=198.51.100.17")

	got := ClientIP(req)
	if got != "192.0.2.43" {
		t.Errorf("ClientIP() = %q, 期望 %q（取链路最远端）", got, "192.0.2.43")
	}
}

// TestClientIP_Forwarded_WithOtherParams Forwarded 包含多个参数（proto/host 等）
func TestClientIP_Forwarded_WithOtherParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Forwarded", "for=192.0.2.43; proto=https; host=example.com; by=203.0.113.43")

	got := ClientIP(req)
	if got != "192.0.2.43" {
		t.Errorf("ClientIP() = %q, 期望 %q（应从 for= 取值）", got, "192.0.2.43")
	}
}

// TestClientIP_Forwarded_CaseInsensitive key 不区分大小写
func TestClientIP_Forwarded_CaseInsensitive(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Forwarded", "FOR=192.0.2.43")

	got := ClientIP(req)
	if got != "192.0.2.43" {
		t.Errorf("ClientIP() = %q, 期望 %q（应大小写不敏感）", got, "192.0.2.43")
	}
}

// TestClientIP_XForwardedFor_TakesPrecedenceOverForwarded X-Forwarded-For 优先于 Forwarded
func TestClientIP_XForwardedFor_TakesPrecedenceOverForwarded(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "1.1.1.1")
	req.Header.Set("Forwarded", "for=2.2.2.2")

	got := ClientIP(req)
	if got != "1.1.1.1" {
		t.Errorf("ClientIP() = %q, 期望 %q（XFF 优先）", got, "1.1.1.1")
	}
}

// TestClientIP_Forwarded_TakesPrecedenceOverXRealIp Forwarded 优先于 X-Real-Ip
func TestClientIP_Forwarded_TakesPrecedenceOverXRealIp(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Forwarded", "for=192.0.2.43")
	req.Header.Set("X-Real-Ip", "9.8.7.6")

	got := ClientIP(req)
	if got != "192.0.2.43" {
		t.Errorf("ClientIP() = %q, 期望 %q（Forwarded 优先于 X-Real-Ip）", got, "192.0.2.43")
	}
}

// TestClientIP_Forwarded_MalformedDegradesToNextSource Forwarded 格式错误时降级到下个来源
func TestClientIP_Forwarded_MalformedDegradesToNextSource(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// 没有 for= 字段，仅 proto
	req.Header.Set("Forwarded", "proto=https;host=example.com")
	req.Header.Set("X-Real-Ip", "9.8.7.6")

	got := ClientIP(req)
	if got != "9.8.7.6" {
		t.Errorf("ClientIP() = %q, 期望 %q（应降级到 X-Real-Ip）", got, "9.8.7.6")
	}
}

// TestClientIP_Forwarded_NoFor_ReturnsEmpty 仅 proto=host= 时不应误报 IP
func TestClientIP_Forwarded_NoFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Forwarded", "proto=https")
	req.RemoteAddr = "10.0.0.1:12345"

	got := ClientIP(req)
	if got != "10.0.0.1" {
		t.Errorf("ClientIP() = %q, 期望 %q（无 for= 时降级到 RemoteAddr）", got, "10.0.0.1")
	}
}

// TestClientIP_Forwarded_BracketlessIPv6 无括号 IPv6（少见但合法）
func TestClientIP_Forwarded_BracketlessIPv6(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Forwarded", "for=2001:db8::1")

	got := ClientIP(req)
	if got != "2001:db8::1" {
		t.Errorf("ClientIP() = %q, 期望 %q", got, "2001:db8::1")
	}
}

// —— parseForwardedFor 单元测试 ——

func TestParseForwardedFor_Simple(t *testing.T) {
	if got := parseForwardedFor("for=192.0.2.43"); got != "192.0.2.43" {
		t.Errorf("got %q", got)
	}
}

func TestParseForwardedFor_MultiParams(t *testing.T) {
	got := parseForwardedFor("for=192.0.2.43; proto=https; host=example.com")
	if got != "192.0.2.43" {
		t.Errorf("got %q", got)
	}
}

func TestParseForwardedFor_NoForReturnsEmpty(t *testing.T) {
	if got := parseForwardedFor("proto=https"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestParseForwardedFor_QuotedIPv6WithPort(t *testing.T) {
	got := parseForwardedFor(`for="[2001:db8::1]:8080"`)
	if got != "2001:db8::1" {
		t.Errorf("got %q", got)
	}
}

func TestParseForwardedFor_MultiHop(t *testing.T) {
	got := parseForwardedFor("for=1.2.3.4, for=5.6.7.8")
	if got != "1.2.3.4" {
		t.Errorf("got %q", got)
	}
}

func TestParseForwardedFor_EmptyReturnsEmpty(t *testing.T) {
	if got := parseForwardedFor(""); got != "" {
		t.Errorf("got %q", got)
	}
}
