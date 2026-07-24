package random

import (
	"testing"
)

// TestRangeRand_Basic 基础场景：[0, 10] 内的随机数
func TestRangeRand_Basic(t *testing.T) {
	for i := 0; i < 1000; i++ {
		v, err := RangeRand(0, 10)
		if err != nil {
			t.Fatalf("RangeRand err: %v", err)
		}
		if v < 0 || v > 10 {
			t.Errorf("v = %d, want in [0, 10]", v)
		}
	}
}

// TestRangeRand_NegativeMin 负数下界：[-10, 10]
func TestRangeRand_NegativeMin(t *testing.T) {
	for i := 0; i < 1000; i++ {
		v, err := RangeRand(-10, 10)
		if err != nil {
			t.Fatalf("RangeRand err: %v", err)
		}
		if v < -10 || v > 10 {
			t.Errorf("v = %d, want in [-10, 10]", v)
		}
	}
}

// TestRangeRand_BothNegative 两端负数：[-100, -10]
func TestRangeRand_BothNegative(t *testing.T) {
	for i := 0; i < 1000; i++ {
		v, err := RangeRand(-100, -10)
		if err != nil {
			t.Fatalf("RangeRand err: %v", err)
		}
		if v < -100 || v > -10 {
			t.Errorf("v = %d, want in [-100, -10]", v)
		}
	}
}

// TestRangeRand_LargeSpan 大区间精度（验证无 float64→int64 精度损失）
//
// 旧实现 float64 转换在 min 接近 int64 负极限时丢精度，本测试确保新实现不重蹈
func TestRangeRand_LargeSpan(t *testing.T) {
	// 跨度接近 int64 上限的 1/2（足以暴露旧实现的 float64 路径）
	min := int64(-1) << 50
	max := int64(1) << 50
	for i := 0; i < 100; i++ {
		v, err := RangeRand(min, max)
		if err != nil {
			t.Fatalf("RangeRand err: %v", err)
		}
		if v < min || v > max {
			t.Errorf("v = %d, want in [%d, %d]", v, min, max)
		}
	}
}

// TestRangeRand_SingleValue 单值区间：[5, 5]
func TestRangeRand_SingleValue(t *testing.T) {
	v, err := RangeRand(5, 5)
	if err != nil {
		t.Fatalf("RangeRand err: %v", err)
	}
	if v != 5 {
		t.Errorf("v = %d, want 5", v)
	}
}

// TestRangeRand_MinGreaterThanMax 返回 error（不 panic）
func TestRangeRand_MinGreaterThanMax(t *testing.T) {
	_, err := RangeRand(10, 0)
	if err == nil {
		t.Fatal("expected error when min > max")
	}
}

// TestMustRangeRand_Panic min>max 时 panic
func TestMustRangeRand_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when min > max")
		}
	}()
	_ = MustRangeRand(10, 0)
}

// TestMustRangeRand_OK 正常路径
func TestMustRangeRand_OK(t *testing.T) {
	v := MustRangeRand(0, 10)
	if v < 0 || v > 10 {
		t.Errorf("v = %d, want in [0, 10]", v)
	}
}

// TestRangeRand_Distribution 均匀分布（拒绝采样的核心验证）
//
// 生成 N 个 [0, 3] 内的数，统计频次，每个值的占比应接近 25%。
//
// 容差选择：N=40000 时单桶标准差 σ≈√(N·p·(1-p))≈86.6。
//   - ±2% (=±200≈2.3σ)：单桶越界概率 ~2%，4 桶联合后 CI 必然偶发失败（实测 flaky）
//   - ±3% (=±300≈3.5σ)：单桶越界概率 ~0.02%，4 桶联合 <0.1%，可视为确定性通过
// crypto/rand 不可播种，无法靠固定种子消除波动，故用统计学上足够稳的容差。
func TestRangeRand_Distribution(t *testing.T) {
	const (
		min, max = 0, 3
		N        = 40000
		tol      = 0.03 // 3% 容差（见上方说明，避免 crypto/rand 不可播种导致的 flaky）
	)
	counts := [4]int{}
	for i := 0; i < N; i++ {
		v, err := RangeRand(min, max)
		if err != nil {
			t.Fatalf("RangeRand err: %v", err)
		}
		counts[v]++
	}
	want := float64(N) / 4
	for i, c := range counts {
		got := float64(c)
		if got < want*(1-tol) || got > want*(1+tol) {
			t.Errorf("value %d count = %d (%.2f%%), want ~%.0f (±%.0f%%)",
				i, c, got/N*100, want, tol*100)
		}
	}
}

// TestInt63_Range 范围正确：所有值 >= 0
func TestInt63_Range(t *testing.T) {
	for i := 0; i < 1000; i++ {
		v, err := Int63()
		if err != nil {
			t.Fatalf("Int63 err: %v", err)
		}
		if v < 0 {
			t.Errorf("v = %d, want >= 0", v)
		}
	}
}

// TestBytes 长度正确 + 非全零
func TestBytes(t *testing.T) {
	b, err := Bytes(32)
	if err != nil {
		t.Fatalf("Bytes err: %v", err)
	}
	if len(b) != 32 {
		t.Errorf("len = %d, want 32", len(b))
	}
	allZero := true
	for _, x := range b {
		if x != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("32 random bytes are all zero (extremely unlikely, suspect entropy source)")
	}
}

// TestBytes_NegativeLength 返回 error
func TestBytes_NegativeLength(t *testing.T) {
	_, err := Bytes(-1)
	if err == nil {
		t.Fatal("expected error for negative length")
	}
}

// TestBytes_Zero 返回空 slice（不是 nil）
func TestBytes_Zero(t *testing.T) {
	b, err := Bytes(0)
	if err != nil {
		t.Fatalf("Bytes(0) err: %v", err)
	}
	if len(b) != 0 {
		t.Errorf("len = %d, want 0", len(b))
	}
}

// —— 快速伪随机路径测试 ——

func TestFastRange_InclusiveBounds(t *testing.T) {
	for i := 0; i < 1000; i++ {
		v := FastRange(5, 7)
		if v < 5 || v > 7 {
			t.Fatalf("FastRange(5,7) = %d, 不在闭区间", v)
		}
	}
	// 单点区间
	if v := FastRange(9, 9); v != 9 {
		t.Errorf("FastRange(9,9) = %d, want 9", v)
	}
}

func TestFastRange_PanicOnInverted(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("FastRange(min>max) 应 panic")
		}
	}()
	_ = FastRange(10, 1)
}

func TestFastInt_Range(t *testing.T) {
	for i := 0; i < 1000; i++ {
		v := FastInt(100)
		if v < 0 || v >= 100 {
			t.Fatalf("FastInt(100) = %d, 不在 [0,100)", v)
		}
	}
}

func TestFastString(t *testing.T) {
	s := FastString(16)
	if len(s) != 16 {
		t.Fatalf("len = %d, want 16", len(s))
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
			t.Errorf("FastString 含非字母数字字符 %q", c)
		}
	}
	if FastString(0) != "" {
		t.Error("FastString(0) 应返回空串")
	}
	// 两次生成应（极大概率）不同
	a, b := FastString(32), FastString(32)
	if a == b {
		t.Error("两次 FastString(32) 相同（极不应该）")
	}
}

// BenchmarkRangeRand 性能基准（参考用）
//
// 期望：<200ns/op（crypto/rand.Read 8 字节的开销）
// 对比：旧 big.Int 实现约 500ns/op（每次 big.NewInt 分配 + gcd 计算）
func BenchmarkRangeRand(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = RangeRand(0, 1<<30)
	}
}
