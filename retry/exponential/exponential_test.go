package exponential

import (
	"testing"
	"time"
)

func TestNext_Backoff(t *testing.T) {
	r := NewWithOptions(
		WithBaseDelay(100*time.Millisecond),
		WithMaxRetries(5),
		WithMaxDelay(10*time.Second),
	)

	var prev time.Duration
	for i := 0; i < 5; i++ {
		wait, ok := r.Next()
		if !ok {
			t.Fatalf("Next %d should return ok=true", i+1)
		}
		if i > 0 && wait <= prev {
			t.Fatalf("wait should increase: prev=%v, current=%v", prev, wait)
		}
		prev = wait
	}
}

func TestNext_MaxRetries(t *testing.T) {
	r := NewWithOptions(WithMaxRetries(2))

	_, ok := r.Next()
	if !ok {
		t.Fatal("first Next should return ok=true")
	}
	_, ok = r.Next()
	if !ok {
		t.Fatal("second Next should return ok=true")
	}
	_, ok = r.Next()
	if ok {
		t.Fatal("third Next should return ok=false (exceeded max retries)")
	}
}

func TestReset(t *testing.T) {
	r := NewWithOptions(WithMaxRetries(1))

	_, ok := r.Next()
	if !ok {
		t.Fatal("first Next should return ok=true")
	}
	_, ok = r.Next()
	if ok {
		t.Fatal("second Next should return ok=false")
	}

	r.Reset()
	if r.Count() != 0 {
		t.Fatalf("expected count 0 after reset, got %d", r.Count())
	}
	_, ok = r.Next()
	if !ok {
		t.Fatal("Next after Reset should return ok=true")
	}
}

// TestNew_Positional 验证位置参数构造器
func TestNew_Positional(t *testing.T) {
	r := New(3, 100*time.Millisecond)
	for i := 0; i < 3; i++ {
		if _, ok := r.Next(); !ok {
			t.Fatalf("Next %d should be ok", i+1)
		}
	}
	if _, ok := r.Next(); ok {
		t.Fatal("4th Next should be false (exceeded max retries=3)")
	}
}

// TestNew_InvalidParams 验证非法参数兜底
func TestNew_InvalidParams(t *testing.T) {
	r := New(-1, 0)
	// maxRetries=-1 兜底为 0，第一次 Next 应返回 false
	if _, ok := r.Next(); ok {
		t.Fatal("Next with maxRetries=0 should return ok=false immediately")
	}
}
