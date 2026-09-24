package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-zeus/zeus/examples/20-full-demo/internal/gwapi"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/registry/memory"
	"github.com/go-zeus/zeus/routing"
)

// postRegister 向 handleRegister 发送一次注册请求
func postRegister(t *testing.T, reg registry.Registrar, cache *instanceCache, ins gwapi.Instance) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(gwapi.RegisterRequest{Instance: ins})
	req := httptest.NewRequest(http.MethodPost, "/internal/register", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	handleRegister(reg, cache, rec, req)
	return rec
}

// TestHandleRegister_Idempotent 心跳场景依赖：同 ID 重复注册必须幂等成功，
// 而不是把 memory registry 的"重复实例"错误透传成 500（否则心跳被误判为失败）
func TestHandleRegister_Idempotent(t *testing.T) {
	reg := memory.New()
	cache := newCache()
	ins := gwapi.Instance{
		ID: "srv1-test", Name: "srv1", Cluster: routing.Default,
		Protocol: "http", IP: "127.0.0.1", Port: 9001,
	}

	// 首次注册：200
	if rec := postRegister(t, reg, cache, ins); rec.Code != http.StatusOK {
		t.Fatalf("first register: got %d, body=%s", rec.Code, rec.Body.String())
	}

	// 同 ID 重复注册（心跳）：应幂等返回 200 {"ok":true}，而非 500
	rec := postRegister(t, reg, cache, ins)
	if rec.Code != http.StatusOK {
		t.Fatalf("duplicate register should be idempotent: got %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp gwapi.RegisterResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatal("duplicate register should return ok=true")
	}

	// 且不产生重复实例
	entry, err := reg.(registry.Discovery).GetService(context.Background(), "srv1")
	if err != nil || entry == nil {
		t.Fatalf("get service: %v", err)
	}
	if len(entry.Instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(entry.Instances))
	}
}

// TestHandleRegister_EmptyClusterDefaults 未指定 cluster 时默认填充 default
func TestHandleRegister_EmptyClusterDefaults(t *testing.T) {
	reg := memory.New()
	cache := newCache()
	ins := gwapi.Instance{ID: "srv2-test", Name: "srv2", Protocol: "http", IP: "127.0.0.1", Port: 9002}

	if rec := postRegister(t, reg, cache, ins); rec.Code != http.StatusOK {
		t.Fatalf("register: got %d", rec.Code)
	}
	entry, err := reg.(registry.Discovery).GetService(context.Background(), "srv2")
	if err != nil || entry == nil {
		t.Fatalf("get service: %v", err)
	}
	for _, i := range entry.Instances {
		if i.Cluster != routing.Default {
			t.Fatalf("cluster should default to %q, got %q", routing.Default, i.Cluster)
		}
	}
}
