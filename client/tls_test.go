package client

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-zeus/zeus/balancer"
	"github.com/go-zeus/zeus/balancer/roundrobin"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/registry/memory"
	"github.com/go-zeus/zeus/types"
)

// —— 测试辅助：生成自签名证书 ——

func generateClientTestCert(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Unix(0, 0),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true,
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

// —— WithTLS / WithTransport / WithTimeout 单元测试 ——

// TestWithTLS_DefaultWithNewClient 默认情况下 Transport 应为 *http.Transport
func TestWithTLS_DefaultWithNewClient(t *testing.T) {
	c := newTestClient()
	tr, ok := c.cc.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.cc.Transport)
	}
	if tr == nil {
		t.Error("Transport should be non-nil")
	}
}

// TestWithTLS_AppliedToTransport TLS 配置应注入到 Transport
func TestWithTLS_AppliedToTransport(t *testing.T) {
	cfg := &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}
	c := newTestClient(WithTLS(cfg))

	tr, ok := c.cc.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.cc.Transport)
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig should be non-nil")
	}
	if !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Error("MinVersion should be TLS 1.2")
	}
}

// TestWithTransport_CustomTransport 自定义 Transport 应被采用
func TestWithTransport_CustomTransport(t *testing.T) {
	custom := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     30 * time.Second,
	}
	c := newTestClient(WithTransport(custom))

	tr, ok := c.cc.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.cc.Transport)
	}
	if tr != custom {
		t.Error("WithTransport should install custom transport")
	}
	if tr.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns = %d, want 100", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 20 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 20", tr.MaxIdleConnsPerHost)
	}
}

// TestWithTransport_PlusTLS 自定义 Transport + TLS 应合并
func TestWithTransport_PlusTLS(t *testing.T) {
	custom := &http.Transport{MaxIdleConns: 50}
	c := newTestClient(
		WithTransport(custom),
		WithTLS(&tls.Config{InsecureSkipVerify: true}),
	)
	tr, ok := c.cc.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.cc.Transport)
	}
	if tr != custom {
		t.Error("custom Transport should be preserved")
	}
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("TLS config should be merged into custom Transport")
	}
}

// TestWithTimeout_AppliedToClient 全局超时应设置到 http.Client.Timeout
func TestWithTimeout_AppliedToClient(t *testing.T) {
	c := newTestClient(WithTimeout(5 * time.Second))
	if c.cc.Timeout != 5*time.Second {
		t.Errorf("cc.Timeout = %v, want 5s", c.cc.Timeout)
	}
}

// TestWithHTTPClient_PreservesUserTransport 用户注入的 http.Client 的 Transport 应被保留
func TestWithHTTPClient_PreservesUserTransport(t *testing.T) {
	customTransport := &http.Transport{MaxIdleConns: 7}
	userClient := &http.Client{Transport: customTransport}

	c := newTestClient(WithHTTPClient(userClient))
	if c.cc != userClient {
		t.Error("WithHTTPClient should install user's client")
	}
	tr, ok := c.cc.Transport.(*http.Transport)
	if !ok || tr != customTransport {
		t.Error("user Transport should be preserved")
	}
	if c.ownsClient {
		t.Error("ownsClient should be false for WithHTTPClient")
	}
}

// TestWithHTTPClient_TLSMergedToUserTransport 用户注入 Client + TLS 应注入到现有 Transport
func TestWithHTTPClient_TLSMergedToUserTransport(t *testing.T) {
	customTransport := &http.Transport{MaxIdleConns: 7}
	userClient := &http.Client{Transport: customTransport}

	c := newTestClient(
		WithHTTPClient(userClient),
		WithTLS(&tls.Config{InsecureSkipVerify: true}),
	)

	tr, ok := c.cc.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.cc.Transport)
	}
	if tr != customTransport {
		t.Error("custom Transport should be preserved")
	}
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("TLS should be merged into user Transport")
	}
}

// —— 集成测试：实际调用 HTTPS server ——

// TestDo_HTTPS_EndToEnd 调用 HTTPS 服务能成功
func TestDo_HTTPS_EndToEnd(t *testing.T) {
	// 启动 HTTPS httptest server
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok from https")
	}))
	defer ts.Close()

	// 用自定义 Transport 让 client 跳过证书校验（测试自签名）
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	c := newTestClient(
		WithTransport(transport),
	)
	defer c.Close()

	// 直接构造 request 走 ts 的 URL（不经过服务发现）
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := c.cc.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok from https" {
		t.Errorf("body = %q", body)
	}
}

// TestDo_HTTP_ViaServiceDiscovery 端到端：HTTP server + 服务发现 + client.Do
func TestDo_HTTP_ViaServiceDiscovery(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		fmt.Fprint(w, "ok from discovery")
	}))
	defer ts.Close()

	// 从 httptest URL 提取 host:port
	host, port, _ := net.SplitHostPort(stringsTrimPrefix(ts.URL, "http://"))

	// 注册实例
	reg := memory.New()
	ins := &types.Instance{
		ID:       "test-1",
		Name:     "test-svc", // 与 newTestClient 默认 name 一致
		Cluster:  "default",
		Protocol: "http",
		IP:       host,
		Port:     atoi(port),
	}
	_ = reg.Register(context.Background(), ins)

	// memory 同时实现 Registrar 和 Discovery，类型断言后传入 client
	dis := reg.(registry.Discovery)

	c := newTestClient(
		Discovery(dis),
		LoadBalance(roundrobin.New()),
	)
	c.load() // 触发首次同步
	defer c.Close()

	req, _ := http.NewRequest(http.MethodGet, "http://test-svc/ping", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok from discovery" {
		t.Errorf("body = %q", body)
	}
}

// —— 辅助：从 ts.URL 解析（避免引入额外 import） ——

func stringsTrimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// —— Benchmark ——

func BenchmarkWithTLS_Apply(b *testing.B) {
	cfg := &tls.Config{InsecureSkipVerify: true}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := &client{
			cc:         &http.Client{},
			ownsClient: true,
			clusters:   make(map[string]balancer.Balancer),
			stopCh:     make(chan struct{}),
		}
		opt := WithTLS(cfg)
		opt(c)
	}
}
