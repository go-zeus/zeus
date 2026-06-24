package http

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
	"os"
	"path/filepath"
	"testing"
	"time"
)

// —— 测试辅助：生成自签名证书 ——

func generateSelfSignedCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	dir := t.TempDir()
	certFile = filepath.Join(dir, "server.crt")
	keyFile = filepath.Join(dir, "server.key")

	// 生成 ECDSA 私钥（比 RSA 更快生成）
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// 自签名证书模板
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:             time.Unix(0, 0),
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost", "127.0.0.1"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	// 写 cert PEM
	certOut, err := os.Create(certFile)
	if err != nil {
		t.Fatalf("create cert file: %v", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		t.Fatalf("write cert PEM: %v", err)
	}
	certOut.Close()

	// 写 key PEM（PKCS8）
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyOut, err := os.Create(keyFile)
	if err != nil {
		t.Fatalf("create key file: %v", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}); err != nil {
		t.Fatalf("write key PEM: %v", err)
	}
	keyOut.Close()

	return certFile, keyFile
}

// —— TLS Option 单元测试 ——

// TestTLS_DefaultNoTLS 默认无 TLS 配置
func TestTLS_DefaultNoTLS(t *testing.T) {
	srv := NewHTTP(Port(0)).(*httpServer)
	if srv.TLSConfig != nil {
		t.Error("TLSConfig should be nil by default")
	}
}

// TestTLS_WithConfig TLS Option 注入后 TLSConfig 应非空
func TestTLS_WithConfig(t *testing.T) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	srv := NewHTTP(TLS(cfg)).(*httpServer)
	if srv.TLSConfig != cfg {
		t.Error("TLSConfig should be set via TLS() option")
	}
}

// TestTLS_WithFiles TLSFiles Option 应加载证书到 TLSConfig
func TestTLS_WithFiles(t *testing.T) {
	certFile, keyFile := generateSelfSignedCert(t)
	srv := NewHTTP(TLSFiles(certFile, keyFile)).(*httpServer)

	if srv.TLSConfig == nil {
		t.Fatal("TLSConfig should be populated from cert/key files")
	}
	if len(srv.TLSConfig.Certificates) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(srv.TLSConfig.Certificates))
	}
}

// TestTLS_WithFiles_Missing 加载失败时回退到 HTTP
func TestTLS_WithFiles_Missing(t *testing.T) {
	srv := NewHTTP(
		TLSFiles("/nonexistent/cert.pem", "/nonexistent/key.pem"),
		Port(0),
	).(*httpServer)

	if srv.TLSConfig != nil {
		t.Error("TLSConfig should be nil when cert files are invalid (fallback to HTTP)")
	}
}

// TestTLS_ClientAuthConfigurable mTLS 配置可传入
func TestTLS_ClientAuthConfigurable(t *testing.T) {
	caPool := x509.NewCertPool()
	cfg := &tls.Config{
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caPool,
		MinVersion: tls.VersionTLS12,
	}
	srv := NewHTTP(TLS(cfg)).(*httpServer)
	if srv.TLSConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Error("ClientAuth should be configurable via TLS()")
	}
	if srv.TLSConfig.MinVersion != tls.VersionTLS12 {
		t.Error("MinVersion should be configurable")
	}
}

// —— 集成测试：实际启动 HTTPS server ——

// TestTLS_StartHTTPS HTTPS 启动后能响应 InsecureSkipVerify 请求
func TestTLS_StartHTTPS(t *testing.T) {
	certFile, keyFile := generateSelfSignedCert(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello tls")
	})

	srv := NewHTTP(
		Port(0),
		TLSFiles(certFile, keyFile),
		Mux(mux),
	).(*httpServer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 占用临时端口
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	srv.port = port

	go srv.Start(ctx)

	// 轮询 HTTPS 连接
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/", port)
	var lastErr error
	for i := 0; i < 50; i++ {
		resp, err := client.Get(url)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if string(body) == "hello tls" {
				return // 成功
			}
			lastErr = fmt.Errorf("unexpected body: %q", body)
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server did not respond over HTTPS: %v", lastErr)
}

// TestTLS_StartHTTP_Fallback 无 TLS Option 时 Start 仍走 HTTP
func TestTLS_StartHTTP_Fallback(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello plain")
	})

	srv := NewHTTP(
		Port(0),
		Mux(mux),
	).(*httpServer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	srv.port = port

	go srv.Start(ctx)

	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	var lastErr error
	for i := 0; i < 50; i++ {
		resp, err := http.Get(url)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if string(body) == "hello plain" {
				return
			}
			lastErr = fmt.Errorf("unexpected body: %q", body)
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server did not respond over HTTP: %v", lastErr)
}
