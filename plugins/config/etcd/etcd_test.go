package etcd

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func testEndpoint() string {
	if v := os.Getenv("ZEUS_ETCD_ENDPOINT"); v != "" {
		return v
	}
	return DefaultEndpoint
}

// skipIfNoEtcd 探活，不可达时跳过集成测试
func skipIfNoEtcd(t *testing.T) {
	t.Helper()
	// 用 New 一次试探：拿不到 client 则跳过
	l := New(WithEndpoints(testEndpoint()), WithPrefix("/zeus/config/test/"))
	kvs, err := l.Load()
	if err != nil {
		t.Skipf("etcd not reachable: %v", err)
	}
	_ = kvs
}

func TestNew_Defaults(t *testing.T) {
	l := New().(*loader)
	if len(l.endpoints) != 1 || l.endpoints[0] != DefaultEndpoint {
		t.Fatalf("expected default endpoint %s, got %v", DefaultEndpoint, l.endpoints)
	}
	if l.prefix != DefaultPrefix {
		t.Fatalf("expected default prefix %s, got %s", DefaultPrefix, l.prefix)
	}
	if !l.ownsClient {
		t.Fatal("expected ownsClient=true by default")
	}
}

func TestNew_OptionsApply(t *testing.T) {
	l := New(
		WithEndpoints("a:2379", "b:2379"),
		WithPrefix("/myapp/"),
		WithKey("db/dsn"),
		WithDialTimeout(5*time.Second),
		WithCredentials("user", "pass"),
	).(*loader)

	if len(l.endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %v", l.endpoints)
	}
	if l.prefix != "/myapp/" {
		t.Fatalf("prefix not applied: %s", l.prefix)
	}
	if l.key != "db/dsn" {
		t.Fatalf("key not applied: %s", l.key)
	}
	if l.dialTimeout != 5*time.Second {
		t.Fatalf("dialTimeout not applied: %v", l.dialTimeout)
	}
	if l.username != "user" || l.password != "pass" {
		t.Fatalf("credentials not applied: %s/%s", l.username, l.password)
	}
}

func TestWatchTarget_PrefixVsKey(t *testing.T) {
	cases := []struct {
		name       string
		key        string
		prefix     string
		wantTarget string
		wantPrefix bool
	}{
		{"prefix only", "", "/myapp/", "/myapp/", true},
		{"key only", "db/dsn", "/myapp/", "/myapp/db/dsn", false},
		{"key with leading slash", "/db/dsn", "/myapp/", "/myapp/db/dsn", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := &loader{prefix: c.prefix, key: c.key}
			target, isPrefix := l.watchTarget()
			if target != c.wantTarget {
				t.Errorf("target=%s want %s", target, c.wantTarget)
			}
			if isPrefix != c.wantPrefix {
				t.Errorf("isPrefix=%v want %v", isPrefix, c.wantPrefix)
			}
		})
	}
}

func TestOptions_NoOpOnEmpty(t *testing.T) {
	// 空参数不应修改默认值（防 nil/空字符串穿透）
	l := New(
		WithEndpoints(),
		WithPrefix(""),
		WithKey(""),
		WithDialTimeout(0),
	).(*loader)
	if l.prefix != DefaultPrefix {
		t.Errorf("empty WithPrefix should not modify default: %s", l.prefix)
	}
	if l.key != "" {
		t.Errorf("empty WithKey should not set key: %s", l.key)
	}
	if l.dialTimeout != DefaultDialTimeout {
		t.Errorf("zero WithDialTimeout should not modify default: %v", l.dialTimeout)
	}
}

// ===== 集成测试（需要真实 etcd）=====

func TestIntegration_Load_Empty(t *testing.T) {
	skipIfNoEtcd(t)
	l := New(WithEndpoints(testEndpoint()), WithPrefix("/zeus/config/test-nonexistent/"))
	kvs, err := l.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(kvs) != 0 {
		t.Fatalf("expected 0 kvs on empty prefix, got %d", len(kvs))
	}
}

func TestIntegration_LoadAndWatch_PrefixMode(t *testing.T) {
	skipIfNoEtcd(t)

	// 用 registry/etcd 测试模式：直接通过 NewWithClient 注入拨号好的 client
	// 这里用最简单的方式：直接 Put 后 Load
	l := New(WithEndpoints(testEndpoint()), WithPrefix("/zeus/config/test-integration/"))

	cli, err := l.(*loader).getClient()
	if err != nil {
		t.Fatalf("getClient: %v", err)
	}
	defer func() {
		// 清理
		cleanupCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_, _ = cli.Delete(cleanupCtx, "/zeus/config/test-integration/")
	}()

	// Put 几个测试 key
	ctx := t.Context()
	keys := map[string]string{
		"/zeus/config/test-integration/db/dsn":  "postgres://localhost",
		"/zeus/config/test-integration/db/pool": "10",
	}
	for k, v := range keys {
		_, err := cli.Put(ctx, k, v)
		if err != nil {
			t.Fatalf("put %s: %v", k, err)
		}
	}

	// Load 应该拿到 2 个 KV，key 已去掉 prefix
	kvs, err := l.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(kvs) != 2 {
		t.Fatalf("expected 2 kvs, got %d: %+v", len(kvs), kvs)
	}
	got := map[string]string{}
	for _, kv := range kvs {
		got[kv.Key] = string(kv.Value)
	}
	if got["db/dsn"] != "postgres://localhost" {
		t.Errorf("db/dsn mismatch: %s", got["db/dsn"])
	}
	if got["db/pool"] != "10" {
		t.Errorf("db/pool mismatch: %s", got["db/pool"])
	}
}

func TestIntegration_Load_KeyMode(t *testing.T) {
	skipIfNoEtcd(t)

	l := New(
		WithEndpoints(testEndpoint()),
		WithPrefix("/zeus/config/test-key/"),
		WithKey("greeting"),
	)
	cli, err := l.(*loader).getClient()
	if err != nil {
		t.Fatalf("getClient: %v", err)
	}
	defer func() {
		cleanupCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_, _ = cli.Delete(cleanupCtx, "/zeus/config/test-key/")
	}()

	ctx := t.Context()
	_, err = cli.Put(ctx, "/zeus/config/test-key/greeting", "hello world")
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	kvs, err := l.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(kvs) != 1 {
		t.Fatalf("expected 1 kv in key mode, got %d", len(kvs))
	}
	if kvs[0].Key != "greeting" {
		t.Errorf("expected relative key 'greeting', got %q", kvs[0].Key)
	}
	if string(kvs[0].Value) != "hello world" {
		t.Errorf("value mismatch: %s", kvs[0].Value)
	}
}

func TestIntegration_Watch_DetectsChange(t *testing.T) {
	skipIfNoEtcd(t)

	prefix := "/zeus/config/test-watch/"
	l := New(WithEndpoints(testEndpoint()), WithPrefix(prefix))
	cli, err := l.(*loader).getClient()
	if err != nil {
		t.Fatalf("getClient: %v", err)
	}
	defer func() {
		cleanupCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_, _ = cli.Delete(cleanupCtx, prefix)
	}()

	// 初始 Put 一个 key
	ctx := t.Context()
	_, _ = cli.Put(ctx, prefix+"k1", "v1")

	w, err := l.Watch()
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	defer w.Stop()

	// 异步触发变更
	go func() {
		time.Sleep(200 * time.Millisecond)
		_, _ = cli.Put(t.Context(), prefix+"k2", "v2")
	}()

	// Next 应在变更后返回包含 k1 + k2 的快照
	nextCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	done := make(chan []string, 1)
	go func() {
		kvs, _ := w.Next()
		var keys []string
		for _, kv := range kvs {
			keys = append(keys, kv.Key+"="+string(kv.Value))
		}
		done <- keys
	}()

	select {
	case <-nextCtx.Done():
		t.Fatal("watch did not detect change within 5s")
	case keys := <-done:
		joined := strings.Join(keys, ",")
		if !strings.Contains(joined, "k1=v1") || !strings.Contains(joined, "k2=v2") {
			t.Fatalf("watch snapshot missing keys: %s", joined)
		}
	}
}
