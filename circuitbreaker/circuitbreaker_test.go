package circuitbreaker

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// mockBreaker 用于测试的模拟熔断器
type mockBreaker struct {
	state    State
	allowErr error
	successN int
	failedN  int
}

func (m *mockBreaker) Allow() error { return m.allowErr }
func (m *mockBreaker) MarkSuccess() { m.successN++ }
func (m *mockBreaker) MarkFailed()  { m.failedN++ }
func (m *mockBreaker) State() State { return m.state }

// testCountBreaker 简易计数熔断器，用于测试完整生命周期
type testCountBreaker struct {
	mu          sync.Mutex
	state       State
	failures    int
	successes   int
	threshold   int
	timeout     time.Duration
	halfOpenMax int
	halfOpenCnt int
	openedAt    time.Time
}

func newTestCountBreaker(threshold int, timeout time.Duration, halfOpenMax int) *testCountBreaker {
	return &testCountBreaker{
		state:       StateClosed,
		threshold:   threshold,
		timeout:     timeout,
		halfOpenMax: halfOpenMax,
	}
}

func (c *testCountBreaker) Allow() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch c.state {
	case StateClosed:
		return nil
	case StateOpen:
		if time.Since(c.openedAt) > c.timeout {
			c.state = StateHalfOpen
			c.halfOpenCnt = 0
			return nil
		}
		return errors.New("circuit open")
	case StateHalfOpen:
		if c.halfOpenCnt < c.halfOpenMax {
			c.halfOpenCnt++
			return nil
		}
		return errors.New("circuit open")
	}
	return nil
}

func (c *testCountBreaker) MarkSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch c.state {
	case StateHalfOpen:
		c.successes++
		if c.successes >= c.halfOpenMax {
			c.state = StateClosed
			c.failures = 0
			c.successes = 0
		}
	case StateClosed:
		c.failures = 0
	}
}

func (c *testCountBreaker) MarkFailed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch c.state {
	case StateHalfOpen:
		c.state = StateOpen
		c.openedAt = time.Now()
		c.successes = 0
	case StateClosed:
		c.failures++
		if c.failures >= c.threshold {
			c.state = StateOpen
			c.openedAt = time.Now()
		}
	}
}

func (c *testCountBreaker) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == StateOpen && time.Since(c.openedAt) > c.timeout {
		return StateHalfOpen
	}
	return c.state
}

func TestNewCircuitBreaker(t *testing.T) {
	d := &mockBreaker{state: StateClosed}
	cb := NewCircuitBreaker(d)
	if cb == nil {
		t.Fatal("NewCircuitBreaker returned nil")
	}
	if cb.State() != StateClosed {
		t.Fatalf("expected StateClosed, got %v", cb.State())
	}
}

func TestExecute_Success(t *testing.T) {
	d := &mockBreaker{state: StateClosed}
	cb := NewCircuitBreaker(d)
	err := cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if d.successN != 1 {
		t.Fatalf("expected 1 MarkSuccess call, got %d", d.successN)
	}
}

func TestExecute_Failed(t *testing.T) {
	d := &mockBreaker{state: StateClosed}
	cb := NewCircuitBreaker(d)
	wantErr := errors.New("fail")
	err := cb.Execute(func() error { return wantErr })
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
	if d.failedN != 1 {
		t.Fatalf("expected 1 MarkFailed call, got %d", d.failedN)
	}
}

func TestCountBreaker(t *testing.T) {
	d := newTestCountBreaker(3, 100*time.Millisecond, 1)
	cb := NewCircuitBreaker(d)

	// Closed -> Open: 连续失败达到阈值
	for i := 0; i < 3; i++ {
		_ = cb.Execute(func() error { return errors.New("fail") })
	}
	if cb.State() != StateOpen {
		t.Fatalf("expected StateOpen, got %v", cb.State())
	}

	// Open: 拒绝请求
	err := cb.Allow()
	if err == nil {
		t.Fatal("expected error in Open state, got nil")
	}

	// Open -> HalfOpen: 等待超时
	time.Sleep(150 * time.Millisecond)
	if cb.State() != StateHalfOpen {
		t.Fatalf("expected StateHalfOpen, got %v", cb.State())
	}

	// HalfOpen -> Closed: 探测成功
	err = cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("expected no error in HalfOpen, got %v", err)
	}
	if cb.State() != StateClosed {
		t.Fatalf("expected StateClosed, got %v", cb.State())
	}
}
