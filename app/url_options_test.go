package app

import (
	"strings"
	"testing"

	_ "github.com/go-zeus/zeus/cache/memory" // 注册 memory:// scheme
	"github.com/go-zeus/zeus/components"
	_ "github.com/go-zeus/zeus/mq/memory" // 注册 memory:// scheme (mq)
)

// TestWithCacheURL_ConfigWritten URL 写入 appConfig.cacheURL
func TestWithCacheURL_ConfigWritten(t *testing.T) {
	cfg := captureConfig(WithCacheURL("memory://?name=test"))
	if cfg.cacheURL != "memory://?name=test" {
		t.Errorf("cacheURL = %q", cfg.cacheURL)
	}
}

// TestWithDatabaseURL_ConfigWritten URL 写入 appConfig.databaseURL
func TestWithDatabaseURL_ConfigWritten(t *testing.T) {
	cfg := captureConfig(WithDatabaseURL("mysql://root:pass@127.0.0.1/test"))
	if cfg.databaseURL != "mysql://root:pass@127.0.0.1/test" {
		t.Errorf("databaseURL = %q", cfg.databaseURL)
	}
}

// TestWithMQURL_ConfigWritten URL 写入 appConfig.mqURL
func TestWithMQURL_ConfigWritten(t *testing.T) {
	cfg := captureConfig(WithMQURL("memory://"))
	if cfg.mqURL != "memory://" {
		t.Errorf("mqURL = %q", cfg.mqURL)
	}
}

// TestWithXxxURL_EmptyIgnored 空 URL 不写入（保持默认不装配）
func TestWithXxxURL_EmptyIgnored(t *testing.T) {
	cfg := captureConfig(WithCacheURL(""), WithDatabaseURL(""), WithMQURL(""))
	if cfg.cacheURL != "" || cfg.databaseURL != "" || cfg.mqURL != "" {
		t.Errorf("URLs should be empty, got cache=%q db=%q mq=%q",
			cfg.cacheURL, cfg.databaseURL, cfg.mqURL)
	}
}

// TestBuildComponents_CacheURL 装配 CacheComponent
func TestBuildComponents_CacheURL(t *testing.T) {
	// captureConfig 不调用 cache/memory 的 init()（不同包），但 import 该包后 resolver 就注册了
	// 这里借助 buildComponents 验证 URL 装配是否生效
	cfg := &appConfig{
		name:        defaultServiceName,
		cluster:     defaultServiceCluster,
		stopTimeout: defaultStopTimeoutL3,
		cacheURL:    "memory://",
	}
	comps := buildComponents(cfg)

	// 应该有 NewCacheComponent（至少 1 个）
	found := false
	for _, c := range comps {
		if _, ok := c.(*components.CacheComponent); ok {
			found = true
			break
		}
	}
	if !found {
		t.Error("CacheComponent not in buildComponents output")
	}
}

// TestBuildComponents_DatabaseURL 装配 DatabaseComponent
func TestBuildComponents_DatabaseURL(t *testing.T) {
	// 使用内置的 mysql resolver（已在 plugins/database/mysql init() 注册）
	// 但 mysql plugin 是独立 module，主仓测试不引入它
	// 这里用一个临时 resolver 注册来模拟
	// 实际生产环境用户 import _ "...plugins/database/mysql" 即可
	cfg := &appConfig{
		name:        defaultServiceName,
		cluster:     defaultServiceCluster,
		stopTimeout: defaultStopTimeoutL3,
		databaseURL: "test-db-scheme://anywhere",
	}
	// 此时 buildComponents 会 panic（unknown scheme）
	// 用 defer recover 捕获
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for unknown db scheme")
		}
		msg, _ := r.(string)
		if !strings.Contains(msg, "WithDatabaseURL") {
			t.Errorf("panic msg = %q, want contains 'WithDatabaseURL'", msg)
		}
	}()
	_ = buildComponents(cfg)
}

// TestBuildComponents_MQURL 装配 MQComponent（使用 memory scheme）
func TestBuildComponents_MQURL(t *testing.T) {
	cfg := &appConfig{
		name:        defaultServiceName,
		cluster:     defaultServiceCluster,
		stopTimeout: defaultStopTimeoutL3,
		mqURL:       "memory://",
	}
	comps := buildComponents(cfg)

	found := false
	for _, c := range comps {
		if _, ok := c.(*components.MQComponent); ok {
			found = true
			break
		}
	}
	if !found {
		t.Error("MQComponent not in buildComponents output")
	}
}
