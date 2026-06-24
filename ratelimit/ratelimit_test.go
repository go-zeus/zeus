package ratelimit

import (
	"testing"
)

// mockLimiter 用于测试的模拟限流器
type mockLimiter struct {
	allow   bool
	reserve WaitDuration
	rate    float64
}

func (m *mockLimiter) Allow() bool           { return m.allow }
func (m *mockLimiter) Reserve() WaitDuration { return m.reserve }
func (m *mockLimiter) Rate() float64         { return m.rate }

func TestLimiterInterface(t *testing.T) {
	var l Limiter = &mockLimiter{allow: true, rate: 100}
	if !l.Allow() {
		t.Fatal("expected Allow to return true")
	}
	if l.Rate() != 100 {
		t.Fatalf("expected rate 100, got %f", l.Rate())
	}
}
