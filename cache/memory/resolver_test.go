package memory

import (
	"context"
	"testing"
	"time"

	"github.com/go-zeus/zeus/cache"
)

// TestResolveFromURL_Default 默认参数 → memory.New()
func TestResolveFromURL_Default(t *testing.T) {
	c, err := cache.NewFromURL("memory://")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	defer func() { _ = c.Close() }()

	// 验证基本功能可用
	ctx := context.Background()
	if err := c.Set(ctx, "k", "v"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if v, ok := c.Get(ctx, "k"); !ok || v != "v" {
		t.Errorf("Get = (%v,%v), want (v,true)", v, ok)
	}
}

// TestResolveFromURL_WithParams query 参数生效
func TestResolveFromURL_WithParams(t *testing.T) {
	c, err := cache.NewFromURL("memory://?cleanup=120s&name=user-cache&recordKey=true")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	defer func() { _ = c.Close() }()

	impl, ok := c.(*cacheImpl)
	if !ok {
		t.Fatalf("type = %T, want *cacheImpl", c)
	}
	if impl.cleanupInterval != 120*time.Second {
		t.Errorf("cleanupInterval = %v, want 120s", impl.cleanupInterval)
	}
	if impl.name != "user-cache" {
		t.Errorf("name = %q, want user-cache", impl.name)
	}
	if !impl.recordKey {
		t.Error("recordKey should be true")
	}
}

// TestResolveFromURL_MalformedQuery 不识别的 query 参数静默忽略（前向兼容）
func TestResolveFromURL_MalformedQuery(t *testing.T) {
	c, err := cache.NewFromURL("memory://?unknown=foo&alsounknown=bar")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	defer func() { _ = c.Close() }()
	// 仅验证不报错即可（默认值生效）
}

// TestResolveFromURL_BadDuration cleanup 时长无效时退回默认 60s
func TestResolveFromURL_BadDuration(t *testing.T) {
	c, err := cache.NewFromURL("memory://?cleanup=not-a-duration")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	defer func() { _ = c.Close() }()

	impl, ok := c.(*cacheImpl)
	if !ok {
		t.Fatalf("type = %T", c)
	}
	// 默认 60s（New() 内部填充）
	if impl.cleanupInterval != defaultCleanupInterval {
		t.Errorf("cleanupInterval = %v, want default %v", impl.cleanupInterval, defaultCleanupInterval)
	}
}
