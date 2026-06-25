package backoff

import (
	"context"
	"math"
	"testing"
	"time"
)

// —— Exponential ——

func TestExponential_DefaultFactor2(t *testing.T) {
	b := NewExponential(time.Millisecond, WithMax(time.Second))
	// 序列：1ms, 2ms, 4ms, 8ms, 16ms, 32ms, 64ms, 128ms, 256ms, 512ms
	expected := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		4 * time.Millisecond,
		8 * time.Millisecond,
		16 * time.Millisecond,
		32 * time.Millisecond,
		64 * time.Millisecond,
		128 * time.Millisecond,
		256 * time.Millisecond,
		512 * time.Millisecond,
	}
	for i, want := range expected {
		got := b.Next()
		if got != want {
			t.Errorf("attempt %d: got %v, want %v", i, got, want)
		}
	}
}

func TestExponential_RespectsMax(t *testing.T) {
	b := NewExponential(time.Millisecond, WithMax(10*time.Millisecond))
	// 序列：1ms, 2ms, 4ms, 8ms, 10ms, 10ms, ...
	values := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		4 * time.Millisecond,
		8 * time.Millisecond,
		10 * time.Millisecond, // 上限
		10 * time.Millisecond, // 上限
	}
	for i, want := range values {
		got := b.Next()
		if got != want {
			t.Errorf("attempt %d: got %v, want %v", i, got, want)
		}
	}
}

func TestExponential_DefaultMaxIsHuge(t *testing.T) {
	b := NewExponential(time.Millisecond)
	// 默认 max 应是 math.MaxInt64，让 30 次重试不会触及上限
	for i := 0; i < 30; i++ {
		got := b.Next()
		if got <= 0 {
			t.Errorf("attempt %d: got %v, want positive", i, got)
		}
		// 提前停止：避免 attempt 太大溢出
		if got > time.Duration(math.MaxInt64/2) {
			break
		}
	}
}

func TestExponential_CustomFactor(t *testing.T) {
	b := NewExponential(time.Millisecond, WithFactor(1.5), WithMax(time.Second))
	// 序列：1ms, 1.5ms, 2.25ms, 3.375ms, ...
	got0 := b.Next()
	if got0 != time.Millisecond {
		t.Errorf("attempt 0: got %v, want 1ms", got0)
	}
	got1 := b.Next()
	// 浮点比较：1.5ms 应是 1.5 * time.Millisecond = 1500000 ns
	want1 := time.Duration(1.5 * float64(time.Millisecond))
	if got1 != want1 {
		t.Errorf("attempt 1: got %v, want %v", got1, want1)
	}
}

func TestExponential_JitterBounds(t *testing.T) {
	const base = 100 * time.Millisecond
	b := NewExponential(base, WithJitter(0.1)) // ±10%
	// attempt=0 时，理论值 100ms，jitter 后应在 [90ms, 110ms]
	for i := 0; i < 100; i++ {
		b.Reset()
		got := b.Next()
		minBound := 90 * time.Millisecond
		maxBound := 110 * time.Millisecond
		if got < minBound || got > maxBound {
			t.Errorf("attempt %d: got %v, want in [%v, %v]", i, got, minBound, maxBound)
		}
	}
}

func TestExponential_JitterClampsToRange(t *testing.T) {
	// jitter=1.0 时范围应是 [0, 2*base]
	const base = 100 * time.Millisecond
	b := NewExponential(base, WithJitter(1.0))
	for i := 0; i < 100; i++ {
		b.Reset()
		got := b.Next()
		minBound := 0 * time.Millisecond
		maxBound := 200 * time.Millisecond
		if got < minBound || got > maxBound {
			t.Errorf("attempt %d: got %v, want in [%v, %v]", i, got, minBound, maxBound)
		}
	}
}

func TestExponential_JitterOutOfRangeClamps(t *testing.T) {
	b := NewExponential(time.Millisecond, WithJitter(-0.5))
	// jitter < 0 应被 clamp 到 0
	if b.jitter != 0 {
		t.Errorf("jitter = %v, want 0 (clamped)", b.jitter)
	}

	b2 := NewExponential(time.Millisecond, WithJitter(2.0))
	// jitter > 1 应被 clamp 到 1
	if b2.jitter != 1.0 {
		t.Errorf("jitter = %v, want 1 (clamped)", b2.jitter)
	}
}

func TestExponential_Reset(t *testing.T) {
	b := NewExponential(time.Millisecond, WithMax(time.Second))
	// 前进到 attempt=3
	for i := 0; i < 3; i++ {
		b.Next()
	}
	if b.Attempt() != 3 {
		t.Errorf("Attempt = %d, want 3", b.Attempt())
	}
	// Reset 后下次 Next 应回到 attempt=0 的值（1ms）
	b.Reset()
	if b.Attempt() != 0 {
		t.Errorf("After Reset: Attempt = %d, want 0", b.Attempt())
	}
	got := b.Next()
	if got != time.Millisecond {
		t.Errorf("After Reset+Next: got %v, want 1ms", got)
	}
}

func TestExponential_AttemptTracking(t *testing.T) {
	b := NewExponential(time.Millisecond, WithMax(time.Second))
	for i := 0; i < 5; i++ {
		if b.Attempt() != i {
			t.Errorf("before Next %d: Attempt = %d, want %d", i, b.Attempt(), i)
		}
		b.Next()
	}
	if b.Attempt() != 5 {
		t.Errorf("after 5 Next: Attempt = %d, want 5", b.Attempt())
	}
}

// —— Constant ——

func TestConstant_AlwaysSame(t *testing.T) {
	c := NewConstant(50 * time.Millisecond)
	for i := 0; i < 5; i++ {
		got := c.Next()
		if got != 50*time.Millisecond {
			t.Errorf("attempt %d: got %v, want 50ms", i, got)
		}
	}
	if c.Attempt() != 5 {
		t.Errorf("Attempt = %d, want 5", c.Attempt())
	}
}

func TestConstant_Reset(t *testing.T) {
	c := NewConstant(time.Second)
	for i := 0; i < 3; i++ {
		c.Next()
	}
	if c.Attempt() != 3 {
		t.Errorf("Attempt = %d, want 3", c.Attempt())
	}
	c.Reset()
	if c.Attempt() != 0 {
		t.Errorf("After Reset: Attempt = %d, want 0", c.Attempt())
	}
}

// —— Sleep ——

func TestSleep_WaitsAndReturns(t *testing.T) {
	b := NewConstant(20 * time.Millisecond)
	start := time.Now()
	err := Sleep(context.Background(), b)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if elapsed < 15*time.Millisecond {
		t.Errorf("Sleep returned too soon: %v", elapsed)
	}
}

func TestSleep_CtxCanceledReturnsErr(t *testing.T) {
	b := NewConstant(2 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := Sleep(ctx, b)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from canceled ctx")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Sleep should return shortly after ctx cancel, elapsed = %v", elapsed)
	}
}

func TestSleep_ZeroDurationNoop(t *testing.T) {
	// 极端 case：Next 返回 0 应立即返回
	b := NewConstant(0)
	start := time.Now()
	err := Sleep(context.Background(), b)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("err = %v", err)
	}
	if elapsed > 10*time.Millisecond {
		t.Errorf("0 duration Sleep should be noop, elapsed = %v", elapsed)
	}
}

// —— 综合：典型重试循环 ——

func TestComposite_RetryLoop(t *testing.T) {
	b := NewExponential(time.Millisecond, WithMax(10*time.Millisecond), WithJitter(0.1))

	attempts := 0
	success := false
	totalWait := time.Duration(0)

	for i := 0; i < 5; i++ {
		attempts++
		// 模拟：第 3 次成功
		if attempts == 3 {
			success = true
			break
		}
		w := b.Next()
		totalWait += w
		// 实际不 sleep，只累加等待时间
	}

	if !success {
		t.Fatal("should succeed on attempt 3")
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
	// 等待时间 = 1ms + 2ms = 3ms（第 3 次成功时不 Next）
	// 但 jitter ±10% 可能轻微扰动
	minWait := 2 * time.Millisecond
	maxWait := 4 * time.Millisecond
	if totalWait < minWait || totalWait > maxWait {
		t.Errorf("totalWait = %v, want in [%v, %v]", totalWait, minWait, maxWait)
	}
}

// —— Benchmark ——

func BenchmarkExponential_Next(b *testing.B) {
	back := NewExponential(time.Millisecond, WithMax(time.Second), WithJitter(0.1))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = back.Next()
	}
}

func BenchmarkConstant_Next(b *testing.B) {
	back := NewConstant(time.Millisecond)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = back.Next()
	}
}

func BenchmarkExponential_Reset(b *testing.B) {
	back := NewExponential(time.Millisecond)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		back.Reset()
	}
}
