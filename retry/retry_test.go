package retry

import (
	"testing"
	"time"
)

// mockRetrier 用于测试的模拟重试策略
type mockRetrier struct {
	calls     int
	maxCalls  int
	baseDelay time.Duration
}

func (m *mockRetrier) Next() (time.Duration, bool) {
	m.calls++
	if m.calls > m.maxCalls {
		return 0, false
	}
	return m.baseDelay, true
}
func (m *mockRetrier) Reset()     { m.calls = 0 }
func (m *mockRetrier) Count() int { return m.calls }

func TestRetrierInterface(t *testing.T) {
	var r Retrier = &mockRetrier{maxCalls: 3}
	if r.Count() != 0 {
		t.Fatalf("expected count 0, got %d", r.Count())
	}
	_, ok := r.Next()
	if !ok {
		t.Fatal("first Next should return ok=true")
	}
	if r.Count() != 1 {
		t.Fatalf("expected count 1, got %d", r.Count())
	}
	r.Reset()
	if r.Count() != 0 {
		t.Fatalf("expected count 0 after reset, got %d", r.Count())
	}
}
