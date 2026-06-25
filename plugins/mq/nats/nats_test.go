package nats

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-zeus/zeus/mq"
	"github.com/go-zeus/zeus/propagation"

	natsclient "github.com/nats-io/nats.go"
)

// —— resolver 测试 ——

// TestParseURLOptions_SingleServer 单服务器 URL 解析
func TestParseURLOptions_SingleServer(t *testing.T) {
	opts := parseURLOptions("nats://127.0.0.1:4222")
	if len(opts) < 1 {
		t.Fatal("expected at least 1 option")
	}
	// 通过构造 brokerConfig 验证 url 字段
	cfg := &brokerConfig{
		url:            "fallback",
		errHandler:     defaultErrHandler,
		connectTimeout: defaultConnectTimeout,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.url != "nats://127.0.0.1:4222" {
		t.Errorf("url = %q, want nats://127.0.0.1:4222", cfg.url)
	}
}

// TestParseURLOptions_WithAuth 带认证 URL 解析
func TestParseURLOptions_WithAuth(t *testing.T) {
	opts := parseURLOptions("nats://user:pass@host.example:4222")
	cfg := &brokerConfig{
		url:            "fallback",
		errHandler:     defaultErrHandler,
		connectTimeout: defaultConnectTimeout,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.url != "nats://user:pass@host.example:4222" {
		t.Errorf("url = %q, want nats://user:pass@host.example:4222", cfg.url)
	}
}

// TestParseURLOptions_DefaultPort 默认端口
func TestParseURLOptions_DefaultPort(t *testing.T) {
	opts := parseURLOptions("nats://host.example")
	cfg := &brokerConfig{
		url:            "fallback",
		errHandler:     defaultErrHandler,
		connectTimeout: defaultConnectTimeout,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.url != "nats://host.example:4222" {
		t.Errorf("url = %q, want nats://host.example:4222", cfg.url)
	}
}

// TestParseURLOptions_TimeoutQuery timeout query 参数生效
func TestParseURLOptions_TimeoutQuery(t *testing.T) {
	opts := parseURLOptions("nats://127.0.0.1:4222?timeout=3s")
	cfg := &brokerConfig{
		url:            "fallback",
		errHandler:     defaultErrHandler,
		connectTimeout: defaultConnectTimeout,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.connectTimeout != 3*time.Second {
		t.Errorf("connectTimeout = %v, want 3s", cfg.connectTimeout)
	}
}

// TestParseURLOptions_InvalidTimeout 无效 timeout 静默忽略
func TestParseURLOptions_InvalidTimeout(t *testing.T) {
	opts := parseURLOptions("nats://127.0.0.1:4222?timeout=not-a-duration")
	cfg := &brokerConfig{
		url:            "fallback",
		errHandler:     defaultErrHandler,
		connectTimeout: defaultConnectTimeout,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.connectTimeout != defaultConnectTimeout {
		t.Errorf("connectTimeout = %v, want default %v", cfg.connectTimeout, defaultConnectTimeout)
	}
}

// TestParseURLOptions_MultiServer 多服务器 URL 透传（url.Parse 失败时）
func TestParseURLOptions_MultiServer(t *testing.T) {
	// 多 host 含逗号：url.Parse 会失败 → 走兜底分支透传 rawURL
	opts := parseURLOptions("nats://h1:4222,h2:4222")
	cfg := &brokerConfig{
		url:            "fallback",
		errHandler:     defaultErrHandler,
		connectTimeout: defaultConnectTimeout,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	// 兜底分支会把 rawURL 原样透传
	if !strings.Contains(cfg.url, "h1:4222") || !strings.Contains(cfg.url, "h2:4222") {
		t.Errorf("multi-host URL should be passed through, got %q", cfg.url)
	}
}

// TestResolver_Registered nats scheme 已注册到 mq
func TestResolver_Registered(t *testing.T) {
	all := mq.RegisteredResolvers()
	if _, ok := all["nats"]; !ok {
		t.Errorf("nats scheme not registered, got: %v", all)
	}
}

// TestNew_InvalidURL 无效 URL 连接失败
func TestNew_InvalidURL(t *testing.T) {
	// 用一个绝对不可达的地址 + 短超时，保证快速失败
	_, err := New(
		WithURL("nats://127.0.0.1:1"), // port 1 reserved，连接必失败
		WithConnectTimeout(200*time.Millisecond),
	)
	if err == nil {
		t.Fatal("expected error for unreachable NATS server")
	}
	if !strings.Contains(err.Error(), "connect") && !strings.Contains(err.Error(), "refused") && !strings.Contains(err.Error(), "timeout") {
		t.Errorf("err = %v, want contains 'connect/refused/timeout'", err)
	}
}

// —— baggage 透传单元测试（不需 NATS 实例） ——

// TestEncodeBaggage_W3CFormat Baggage 编码符合 W3C 格式
func TestEncodeBaggage_W3CFormat(t *testing.T) {
	bag := propagation.NewBag().
		With("tenant.id", "acme").
		With("cluster", "canary")
	got := encodeBaggage(bag)
	// 顺序：按插入顺序，逗号分隔
	if !strings.Contains(got, "tenant.id=acme") {
		t.Errorf("missing tenant.id=acme in %q", got)
	}
	if !strings.Contains(got, "cluster=canary") {
		t.Errorf("missing cluster=canary in %q", got)
	}
	if !strings.Contains(got, ",") {
		t.Errorf("expected comma separator in %q", got)
	}
}

// TestEncodeBaggage_Empty 空 Bag 返回空字符串
func TestEncodeBaggage_Empty(t *testing.T) {
	if got := encodeBaggage(propagation.NewBag()); got != "" {
		t.Errorf("empty bag should encode to empty string, got %q", got)
	}
	if got := encodeBaggage(nil); got != "" {
		t.Errorf("nil bag should encode to empty string, got %q", got)
	}
}

// TestNatsMsgToMQ NATS Msg → mq.Message 转换
func TestNatsMsgToMQ(t *testing.T) {
	nmsg := &natsclient.Msg{
		Subject: "test.topic",
		Data:    []byte("hello"),
		Header: natsclient.Header{
			"X-Custom": []string{"value1"},
			"Baggage":  []string{"tenant.id=acme"},
		},
	}
	msg := natsMsgToMQ(nmsg)
	if msg.Topic != "test.topic" {
		t.Errorf("Topic = %q", msg.Topic)
	}
	if string(msg.Payload) != "hello" {
		t.Errorf("Payload = %q", msg.Payload)
	}
	if msg.Headers["X-Custom"] != "value1" {
		t.Errorf("Headers[X-Custom] = %q", msg.Headers["X-Custom"])
	}
	if msg.Headers["Baggage"] != "tenant.id=acme" {
		t.Errorf("Headers[Baggage] = %q", msg.Headers["Baggage"])
	}
}

// —— 集成测试（需要真实 NATS 服务器） ——

// skipIfNoNATS 当 NATS 不可达时跳过测试
func skipIfNoNATS(t *testing.T) mq.Broker {
	t.Helper()
	url := os.Getenv("ZEUS_NATS_URL")
	if url == "" {
		url = "nats://127.0.0.1:4222"
	}
	b, err := New(WithURL(url), WithConnectTimeout(1*time.Second))
	if err != nil {
		t.Skipf("NATS 不可达，跳过集成测试 (connect failed): %v", err)
		return nil
	}
	return b
}

// TestNATS_EndToEnd 端到端：订阅 → 发布 → handler 收到
func TestNATS_EndToEnd(t *testing.T) {
	b := skipIfNoNATS(t)
	defer b.Close()

	var (
		mu         sync.Mutex
		gotTopic   string
		gotPayload []byte
		done       = make(chan struct{})
	)
	_ = b.Subscribe(context.Background(), "zeus.test.e2e", func(_ context.Context, msg *mq.Message) error {
		mu.Lock()
		defer mu.Unlock()
		gotTopic = msg.Topic
		gotPayload = msg.Payload
		close(done)
		return nil
	})

	// 等订阅就绪（NATS 异步）
	time.Sleep(100 * time.Millisecond)

	if err := b.Publish(context.Background(), "zeus.test.e2e", &mq.Message{
		Payload: []byte("hello-nats"),
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler not called within 3s")
	}

	mu.Lock()
	defer mu.Unlock()
	if gotTopic != "zeus.test.e2e" {
		t.Errorf("Topic = %q, want zeus.test.e2e", gotTopic)
	}
	if string(gotPayload) != "hello-nats" {
		t.Errorf("Payload = %q, want hello-nats", gotPayload)
	}
}

// TestNATS_BaggagePropagation baggage 端到端透传
func TestNATS_BaggagePropagation(t *testing.T) {
	b := skipIfNoNATS(t)
	defer b.Close()

	var (
		gotTenant string
		gotOK     bool
		done      = make(chan struct{})
	)
	_ = b.Subscribe(context.Background(), "zeus.test.baggage", func(ctx context.Context, _ *mq.Message) error {
		gotTenant, gotOK = propagation.Get(ctx, "tenant.id")
		close(done)
		return nil
	})

	time.Sleep(100 * time.Millisecond)

	// 发布时注入 baggage
	ctx := propagation.With(context.Background(), "tenant.id", "acme")
	if err := b.Publish(ctx, "zeus.test.baggage", &mq.Message{Payload: []byte("x")}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler not called within 3s")
	}

	if !gotOK {
		t.Fatal("tenant.id not propagated to handler ctx")
	}
	if gotTenant != "acme" {
		t.Errorf("tenant.id = %q, want acme", gotTenant)
	}
}

// TestNATS_HandlerError handler 返回 error 走 ErrorHandler
func TestNATS_HandlerError(t *testing.T) {
	var (
		errCount int32
		errTopic string
	)

	b, err := New(
		WithURL(envOrDefault("ZEUS_NATS_URL", "nats://127.0.0.1:4222")),
		WithConnectTimeout(1*time.Second),
		WithErrorHandler(func(topic string, _ *mq.Message, _ error) {
			atomic.AddInt32(&errCount, 1)
			errTopic = topic
		}),
	)
	if err != nil {
		t.Skipf("NATS 不可达，跳过集成测试: %v", err)
		return
	}
	defer b.Close()

	done := make(chan struct{})
	_ = b.Subscribe(context.Background(), "zeus.test.error", func(_ context.Context, _ *mq.Message) error {
		defer close(done)
		return errors.New("simulated failure")
	})

	time.Sleep(100 * time.Millisecond)
	_ = b.Publish(context.Background(), "zeus.test.error", &mq.Message{Payload: []byte("x")})

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler not called within 3s")
	}

	// 等 ErrorHandler 被调用
	time.Sleep(100 * time.Millisecond)
	if atomic.LoadInt32(&errCount) != 1 {
		t.Errorf("errCount = %d, want 1", errCount)
	}
	if errTopic != "zeus.test.error" {
		t.Errorf("errTopic = %q, want zeus.test.error", errTopic)
	}
}

// envOrDefault 取环境变量，缺省用 fallback
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
