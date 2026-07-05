package etcd

import (
	"testing"
	"time"
)

// apply 解析 URL 并把 Option 应用到一个空的 etcdRegistry，便于断言字段
func apply(t *testing.T, rawURL string) *etcdRegistry {
	t.Helper()
	opts, err := parseURLOptions(rawURL)
	if err != nil {
		t.Fatalf("parseURLOptions(%q) err: %v", rawURL, err)
	}
	r := &etcdRegistry{
		endpoints:   []string{DefaultEndpoint},
		ttl:         DefaultTTL,
		prefix:      DefaultPrefix,
		dialTimeout: DefaultDialTimeout,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// TestParseURL_SingleEndpoint 单 endpoint
func TestParseURL_SingleEndpoint(t *testing.T) {
	r := apply(t, "etcd://127.0.0.1:2379")
	if len(r.endpoints) != 1 || r.endpoints[0] != "127.0.0.1:2379" {
		t.Errorf("endpoints = %v, want [127.0.0.1:2379]", r.endpoints)
	}
}

// TestParseURL_DefaultPort 无端口补 :2379
func TestParseURL_DefaultPort(t *testing.T) {
	r := apply(t, "etcd://host")
	if len(r.endpoints) != 1 || r.endpoints[0] != "host:2379" {
		t.Errorf("endpoints = %v, want [host:2379]", r.endpoints)
	}
}

// TestParseURL_MultiEndpoints 多 endpoint cluster
func TestParseURL_MultiEndpoints(t *testing.T) {
	r := apply(t, "etcd://h1:2379,h2:2379,h3:2379")
	if len(r.endpoints) != 3 {
		t.Fatalf("endpoints = %v, want 3 items", r.endpoints)
	}
	want := []string{"h1:2379", "h2:2379", "h3:2379"}
	for i, ep := range want {
		if r.endpoints[i] != ep {
			t.Errorf("endpoints[%d] = %q, want %q", i, r.endpoints[i], ep)
		}
	}
}

// TestParseURL_Credentials 用户名密码鉴权
func TestParseURL_Credentials(t *testing.T) {
	r := apply(t, "etcd://user:pass@host:2379")
	if r.username != "user" || r.password != "pass" {
		t.Errorf("credentials = %q/%q, want user/pass", r.username, r.password)
	}
	if len(r.endpoints) != 1 || r.endpoints[0] != "host:2379" {
		t.Errorf("endpoints = %v, want [host:2379]", r.endpoints)
	}
}

// TestParseURL_QueryParams ttl/prefix/timeout query 参数
func TestParseURL_QueryParams(t *testing.T) {
	r := apply(t, "etcd://127.0.0.1:2379?ttl=10s&prefix=/x/&timeout=5s")
	if r.ttl != 10*time.Second {
		t.Errorf("ttl = %v, want 10s", r.ttl)
	}
	if r.prefix != "/x/" {
		t.Errorf("prefix = %q, want /x/", r.prefix)
	}
	if r.dialTimeout != 5*time.Second {
		t.Errorf("dialTimeout = %v, want 5s", r.dialTimeout)
	}
}

// TestParseURL_TTLBelowMinimum ttl < 5s 被 WithTTL 忽略（保留默认 30s）
func TestParseURL_TTLBelowMinimum(t *testing.T) {
	r := apply(t, "etcd://127.0.0.1:2379?ttl=2s")
	if r.ttl != DefaultTTL {
		t.Errorf("ttl = %v, want default %v (below 5s minimum rejected)", r.ttl, DefaultTTL)
	}
}

// TestParseURL_InvalidScheme 非 etcd:// 报错
func TestParseURL_InvalidScheme(t *testing.T) {
	if _, err := parseURLOptions("nacos://host:8848"); err == nil {
		t.Error("expected error for non-etcd scheme")
	}
}

// TestParseURL_UnknownQueryIgnored 未知 query 参数静默忽略
func TestParseURL_UnknownQueryIgnored(t *testing.T) {
	r := apply(t, "etcd://127.0.0.1:2379?foo=bar&baz=qux")
	if len(r.endpoints) != 1 || r.endpoints[0] != "127.0.0.1:2379" {
		t.Errorf("endpoints = %v, want [127.0.0.1:2379]", r.endpoints)
	}
}
