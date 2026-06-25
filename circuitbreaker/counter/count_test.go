package counter

import (
	"testing"
	"time"

	"github.com/go-zeus/zeus/circuitbreaker"
)

func TestClosed_Allow(t *testing.T) {
	c := NewCount(WithThreshold(5)).(*countDriver)
	if err := c.Allow(); err != nil {
		t.Fatalf("Closed state should allow request, got error: %v", err)
	}
}

func TestClosed_TransitionToOpen(t *testing.T) {
	c := NewCount(WithThreshold(3)).(*countDriver)
	for i := 0; i < 3; i++ {
		c.MarkFailed()
	}
	if c.State() != circuitbreaker.StateOpen {
		t.Fatalf("expected StateOpen after %d failures, got %v", 3, c.State())
	}
}

func TestOpen_RejectRequest(t *testing.T) {
	c := NewCount(WithThreshold(1), WithTimeout(10*time.Second)).(*countDriver)
	c.MarkFailed() // 触发 Open
	if c.State() != circuitbreaker.StateOpen {
		t.Fatalf("expected StateOpen, got %v", c.State())
	}
	if err := c.Allow(); err == nil {
		t.Fatal("Open state should reject request")
	}
}

func TestHalfOpen_AllowLimitedRequests(t *testing.T) {
	c := NewCount(
		WithThreshold(1),
		WithTimeout(50*time.Millisecond),
		WithHalfOpenMax(2),
	).(*countDriver)

	c.MarkFailed() // Closed -> Open
	time.Sleep(80 * time.Millisecond)

	// 第一次 Allow 触发 Open->HalfOpen 转换，允许通过
	if err := c.Allow(); err != nil {
		t.Fatalf("first Allow (transition to HalfOpen) should be allowed, got %v", err)
	}

	// HalfOpen 状态允许 halfOpenMax 次请求（halfOpenCnt 从 0 开始）
	if err := c.Allow(); err != nil {
		t.Fatalf("second Allow (halfOpenCnt=0) should be allowed, got %v", err)
	}
	if err := c.Allow(); err != nil {
		t.Fatalf("third Allow (halfOpenCnt=1) should be allowed, got %v", err)
	}
	// 超过 halfOpenMax 后应被拒绝
	if err := c.Allow(); err == nil {
		t.Fatal("request exceeding halfOpenMax should be rejected")
	}
}

func TestHalfOpen_TransitionToClosed(t *testing.T) {
	c := NewCount(
		WithThreshold(1),
		WithTimeout(50*time.Millisecond),
		WithHalfOpenMax(1),
	).(*countDriver)

	c.MarkFailed() // Closed -> Open
	time.Sleep(80 * time.Millisecond)

	// 先调用 Allow 触发 Open->HalfOpen 实际转换
	if err := c.Allow(); err != nil {
		t.Fatalf("Allow should succeed (transition to HalfOpen), got %v", err)
	}

	// HalfOpen 成功后转 Closed
	c.MarkSuccess()
	if c.State() != circuitbreaker.StateClosed {
		t.Fatalf("expected StateClosed after HalfOpen success, got %v", c.State())
	}
}

func TestHalfOpen_TransitionToOpen(t *testing.T) {
	c := NewCount(
		WithThreshold(1),
		WithTimeout(50*time.Millisecond),
		WithHalfOpenMax(1),
	).(*countDriver)

	c.MarkFailed() // Closed -> Open
	time.Sleep(80 * time.Millisecond)

	// 先调用 Allow 触发 Open->HalfOpen 实际转换
	if err := c.Allow(); err != nil {
		t.Fatalf("Allow should succeed (transition to HalfOpen), got %v", err)
	}

	// HalfOpen 失败后转 Open
	c.MarkFailed()
	if c.State() != circuitbreaker.StateOpen {
		t.Fatalf("expected StateOpen after HalfOpen failure, got %v", c.State())
	}
}
