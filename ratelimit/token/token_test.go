package token

import (
	"testing"
	"time"
)

func TestAllow_WithinBurst(t *testing.T) {
	l := New(100, 5)
	for i := 0; i < 5; i++ {
		if !l.Allow() {
			t.Fatalf("request %d should be allowed within burst", i+1)
		}
	}
}

func TestAllow_ExceedBurst(t *testing.T) {
	l := New(100, 2)
	// 消耗掉 2 个令牌
	l.Allow()
	l.Allow()
	// 第 3 次应被拒绝
	if l.Allow() {
		t.Fatal("request exceeding burst should be rejected")
	}
}

func TestReserve(t *testing.T) {
	l := New(100, 5)
	wd := l.Reserve()
	if !wd.Allow {
		t.Fatal("Reserve with available tokens should return Allow=true")
	}
	if wd.Duration != 0 {
		t.Fatalf("expected Duration=0 when tokens available, got %v", wd.Duration)
	}
}

// TestNew_ParameterValidation 验证非法参数被兜底
func TestNew_ParameterValidation(t *testing.T) {
	// rate <= 0：兜底为 1
	l := New(0, 0)
	// 第一次 Allow 应成功（兜底 burst=1）
	if !l.Allow() {
		t.Fatal("Allow after parameter fallback should succeed")
	}
	// 第二次应被拒绝（桶空）
	if l.Allow() {
		t.Fatal("second Allow should be rejected")
	}
}

// TestNew_InitialFullBucket 验证初始桶为满
func TestNew_InitialFullBucket(t *testing.T) {
	l := New(10, 3)
	// 初始满桶，3 次都应成功
	for i := 0; i < 3; i++ {
		if !l.Allow() {
			t.Fatalf("initial full bucket: request %d should be allowed", i+1)
		}
	}
}

// TestWithRate_WithBurst Option 链式配置
func TestWithRate_WithBurst(t *testing.T) {
	l := NewWithOptions(10,
		WithRate(100),
		WithBurst(5),
	)
	// burst=5 应允许 5 次
	for i := 0; i < 5; i++ {
		if !l.Allow() {
			t.Fatalf("WithBurst(5): request %d should be allowed", i+1)
		}
	}
	if l.Allow() {
		t.Error("6th request should be rejected after burst exhausted")
	}
	if l.Rate() != 100 {
		t.Errorf("Rate() = %v, want 100", l.Rate())
	}
}

// TestNewWithOptions_ParameterValidation 非法参数兜底
func TestNewWithOptions_ParameterValidation(t *testing.T) {
	// rate=0 + 无 burst Option → burst=int(0)=0 → 兜底为 1
	l := NewWithOptions(0)
	if !l.Allow() {
		t.Error("Allow after fallback should succeed")
	}
	if l.Allow() {
		t.Error("second Allow should be rejected (burst=1)")
	}
}

// TestReserve_Delay 桶空时 Reserve 返回非零等待时间
func TestReserve_Delay(t *testing.T) {
	l := New(10, 1) // rate=10/s, burst=1
	// 消耗掉唯一令牌
	if !l.Allow() {
		t.Fatal("first Allow should succeed")
	}
	wd := l.Reserve()
	if !wd.Allow {
		t.Error("Reserve should return Allow=true (will wait)")
	}
	if wd.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0 when bucket empty", wd.Duration)
	}
}

// TestAllow_RefillOverTime 令牌随时间补充
func TestAllow_RefillOverTime(t *testing.T) {
	// 高 rate，小 burst，快速补充
	l := New(1000, 1)
	if !l.Allow() {
		t.Fatal("first Allow should succeed")
	}
	if l.Allow() {
		t.Fatal("second Allow should be rejected (bucket empty)")
	}
	// 等待令牌补充（rate=1000/s → 1ms 补 1 个）
	time.Sleep(3 * time.Millisecond)
	if !l.Allow() {
		t.Error("Allow should succeed after refill")
	}
}
