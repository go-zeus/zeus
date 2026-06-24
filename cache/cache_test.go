package cache

import (
	"testing"
	"time"
)

// TestWithTTL_Positive 正值 TTL
func TestWithTTL_Positive(t *testing.T) {
	item := &Item{}
	WithTTL(5 * time.Minute)(item)
	if item.TTL != 5*time.Minute {
		t.Errorf("TTL = %v, want 5m", item.TTL)
	}
}

// TestWithTTL_Zero 0 TTL 视为永久（不设置）
func TestWithTTL_Zero(t *testing.T) {
	item := &Item{TTL: 99 * time.Second} // 已有值
	WithTTL(0)(item)
	if item.TTL != 99*time.Second {
		t.Errorf("TTL = %v, want unchanged 99s (0 = noop)", item.TTL)
	}
}

// TestWithTTL_Negative 负值 TTL 视为永久
func TestWithTTL_Negative(t *testing.T) {
	item := &Item{}
	WithTTL(-time.Second)(item)
	if item.TTL != 0 {
		t.Errorf("TTL = %v, want 0 (negative = noop)", item.TTL)
	}
}

// TestNewItem_Basic 基础构造
func TestNewItem_Basic(t *testing.T) {
	item := NewItem("k", "v")
	if item.Key != "k" || item.Value != "v" {
		t.Errorf("item = %+v, want {k, v}", item)
	}
	if item.TTL != 0 {
		t.Errorf("TTL = %v, want 0 (no opts)", item.TTL)
	}
}

// TestNewItem_WithOpts 带 Option 构造
func TestNewItem_WithOpts(t *testing.T) {
	item := NewItem("k", "v", WithTTL(10*time.Second))
	if item.TTL != 10*time.Second {
		t.Errorf("TTL = %v, want 10s", item.TTL)
	}
}

// TestNewItem_NilOption nil Option 不 panic
func TestNewItem_NilOption(t *testing.T) {
	item := NewItem("k", "v", nil, WithTTL(5*time.Second), nil)
	if item.Key != "k" {
		t.Errorf("Key = %q", item.Key)
	}
	if item.TTL != 5*time.Second {
		t.Errorf("TTL = %v, want 5s", item.TTL)
	}
}

// TestNewItem_MultipleOpts 多个 Option 按顺序应用
func TestNewItem_MultipleOpts(t *testing.T) {
	// 后者覆盖前者
	item := NewItem("k", "v",
		WithTTL(5*time.Second),
		WithTTL(10*time.Second),
	)
	if item.TTL != 10*time.Second {
		t.Errorf("TTL = %v, want 10s (last wins)", item.TTL)
	}
}

// TestItem_ZeroValue 零值合理
func TestItem_ZeroValue(t *testing.T) {
	var item Item
	if item.Key != "" || item.Value != nil || item.TTL != 0 {
		t.Errorf("zero Item = %+v, want all zero", item)
	}
}
