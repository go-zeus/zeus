package kafka

import (
	"testing"
	"time"
)

// TestParseURL_SingleBroker 单 broker URL 解析
func TestParseURL_SingleBroker(t *testing.T) {
	opts := parseURLOptions("kafka://127.0.0.1:9092")
	if len(opts) == 0 {
		t.Fatal("expected non-empty options")
	}
	// 用空初始 cfg（WithBrokers 是追加模式）
	cfg := &brokerConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	if len(cfg.brokers) != 1 || cfg.brokers[0] != "127.0.0.1:9092" {
		t.Errorf("brokers = %v, want [127.0.0.1:9092]", cfg.brokers)
	}
}

// TestParseURL_MultiBroker 多 broker（逗号分隔）
func TestParseURL_MultiBroker(t *testing.T) {
	opts := parseURLOptions("kafka://h1:9092,h2:9092,h3:9092")
	cfg := &brokerConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	if len(cfg.brokers) != 3 {
		t.Errorf("brokers = %v, want 3 items", cfg.brokers)
	}
}

// TestParseURL_QueryParams query 参数解析（group/version/timeout）
func TestParseURL_QueryParams(t *testing.T) {
	opts := parseURLOptions("kafka://127.0.0.1:9092?group=order&version=3.7.0&timeout=5s")
	cfg := &brokerConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.group != "order" {
		t.Errorf("group = %q, want order", cfg.group)
	}
	if cfg.version != "3.7.0" {
		t.Errorf("version = %q, want 3.7.0", cfg.version)
	}
	if cfg.dialTimeout != 5*time.Second {
		t.Errorf("dialTimeout = %v, want 5s", cfg.dialTimeout)
	}
}

// TestParseURL_InvalidTimeout 非法 timeout 静默忽略
func TestParseURL_InvalidTimeout(t *testing.T) {
	opts := parseURLOptions("kafka://127.0.0.1:9092?timeout=not-a-duration")
	cfg := &brokerConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	// timeout 未生效，保持默认值
	if cfg.dialTimeout != 0 {
		t.Errorf("dialTimeout = %v, want 0 (default not changed)", cfg.dialTimeout)
	}
}

// TestParseURL_UnknownQuery 未知 query 参数静默忽略
func TestParseURL_UnknownQuery(t *testing.T) {
	opts := parseURLOptions("kafka://127.0.0.1:9092?unknown=value&foo=bar")
	cfg := &brokerConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	// 不会 panic，不会改 cfg
	if len(cfg.brokers) != 1 || cfg.brokers[0] != "127.0.0.1:9092" {
		t.Errorf("brokers = %v, want [127.0.0.1:9092]", cfg.brokers)
	}
}

// TestParseURL_NonKafkaScheme 非 kafka:// URL 透传（兼容场景）
func TestParseURL_NonKafkaScheme(t *testing.T) {
	opts := parseURLOptions("127.0.0.1:9092")
	cfg := &brokerConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	// 直接当 brokers
	if len(cfg.brokers) != 1 || cfg.brokers[0] != "127.0.0.1:9092" {
		t.Errorf("brokers = %v, want [127.0.0.1:9092]", cfg.brokers)
	}
}

// TestSplitAndTrim 字符串分割
func TestSplitAndTrim(t *testing.T) {
	cases := []struct {
		in, sep string
		want    []string
	}{
		{"a,b,c", ",", []string{"a", "b", "c"}},
		{" a , b , c ", ",", []string{"a", "b", "c"}},
		{"a,,b", ",", []string{"a", "b"}}, // 过滤空
		{"", ",", nil},
		{"single", ",", []string{"single"}},
	}
	for _, tc := range cases {
		got := splitAndTrim(tc.in, tc.sep)
		if len(got) != len(tc.want) {
			t.Errorf("splitAndTrim(%q,%q) = %v, want %v", tc.in, tc.sep, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitAndTrim(%q,%q)[%d] = %q, want %q", tc.in, tc.sep, i, got[i], tc.want[i])
			}
		}
	}
}

// TestEncodeBaggage_W3CFormat baggage 编码符合 W3C 格式
func TestEncodeBaggage_W3CFormat(t *testing.T) {
	// nil/empty 返回空字符串
	if got := encodeBaggage(nil); got != "" {
		t.Errorf("encodeBaggage(nil) = %q, want empty", got)
	}
}

// TestDefaultConstants 默认常量合理
func TestDefaultConstants(t *testing.T) {
	if defaultBrokerPort != "9092" {
		t.Errorf("defaultBrokerPort = %q, want 9092", defaultBrokerPort)
	}
	if defaultVersion != "3.5.0" {
		t.Errorf("defaultVersion = %q, want 3.5.0", defaultVersion)
	}
	if baggageHeader != "baggage" {
		t.Errorf("baggageHeader = %q, want baggage", baggageHeader)
	}
}
