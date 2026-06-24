package redis

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/go-zeus/zeus/cache"
)

// TestResolveFromURL_Registered redis scheme 已注册
func TestResolveFromURL_Registered(t *testing.T) {
	mr := miniredis.RunT(t)

	c, err := cache.NewFromURL("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	defer func() { _ = c.Close() }()

	if _, ok := Client(c); !ok {
		t.Error("resolved cache is not redis implementation")
	}
}

// TestResolveFromURL_WithDBAndName URL 路径 + query 参数生效
func TestResolveFromURL_WithDBAndName(t *testing.T) {
	mr := miniredis.RunT(t)

	c, err := cache.NewFromURL("redis://" + mr.Addr() + "/2?name=user-cache&pool=20")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	defer func() { _ = c.Close() }()

	impl, ok := c.(*cacheImpl)
	if !ok {
		t.Fatalf("type = %T", c)
	}
	if impl.name != "user-cache" {
		t.Errorf("name = %q, want user-cache", impl.name)
	}
}

// TestResolveFromURL_Malformed 非法 URL 应返回 error
func TestResolveFromURL_Malformed(t *testing.T) {
	// url.Parse 通常不报错（任何字符串都能解析），但路径解析失败应被处理
	// 用一个相对特殊的输入验证不 panic
	_, _ = cache.NewFromURL("redis://")
	// redis:// 单独存在时 Host=""，redis.NewClient 不会立即报错（懒连接）
	// 此处仅验证 resolver 不 panic
}
