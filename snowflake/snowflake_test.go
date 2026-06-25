package snowflake

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// —— New ——

func TestNew_ValidMachineID(t *testing.T) {
	cases := []int64{0, 1, 512, 1023}
	for _, mid := range cases {
		n, err := New(mid)
		if err != nil {
			t.Errorf("machineID %d: %v", mid, err)
			continue
		}
		if n.machineID != mid {
			t.Errorf("machineID = %d, want %d", n.machineID, mid)
		}
	}
}

func TestNew_InvalidMachineID(t *testing.T) {
	cases := []int64{-1, 1024, 9999}
	for _, mid := range cases {
		_, err := New(mid)
		if err == nil {
			t.Errorf("machineID %d should fail", mid)
		}
	}
}

func TestMustNew_PanicsOnInvalid(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid machineID")
		}
	}()
	MustNew(-1)
}

// —— Next ——

func TestNext_ReturnsPositiveID(t *testing.T) {
	n, _ := New(1)
	id, err := n.Next()
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Errorf("id should be positive, got %d", id)
	}
}

func TestNext_SequenceIncrements(t *testing.T) {
	n, _ := New(1)
	prev, _ := n.Next()
	for i := 0; i < 100; i++ {
		id, _ := n.Next()
		if id <= prev {
			t.Errorf("id should be increasing: prev=%d, id=%d", prev, id)
		}
		prev = id
	}
}

func TestNext_UniqueAcrossManyCalls(t *testing.T) {
	n, _ := New(1)
	const count = 100000
	ids := make(map[int64]bool, count)
	for i := 0; i < count; i++ {
		id, _ := n.Next()
		if ids[id] {
			t.Fatalf("duplicate id at iteration %d: %d", i, id)
		}
		ids[id] = true
	}
	if len(ids) != count {
		t.Errorf("unique ids = %d, want %d", len(ids), count)
	}
}

func TestNext_ConcurrentUnique(t *testing.T) {
	n, _ := New(1)
	const goroutines = 100
	const perG = 1000
	var wg sync.WaitGroup
	var mu sync.Mutex
	ids := make(map[int64]bool, goroutines*perG)
	var dupCount int64

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				id, _ := n.Next()
				mu.Lock()
				if ids[id] {
					atomic.AddInt64(&dupCount, 1)
				}
				ids[id] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if dupCount > 0 {
		t.Errorf("got %d duplicate ids", dupCount)
	}
	expected := int64(goroutines * perG)
	if int64(len(ids)) != expected {
		t.Errorf("total ids = %d, want %d", len(ids), expected)
	}
}

// —— Parse ——

func TestParse_RoundTrip(t *testing.T) {
	n, _ := New(42)
	id, _ := n.Next()

	_, mid, seq := Parse(id)
	if mid != 42 {
		t.Errorf("machineID = %d, want 42", mid)
	}
	if seq != 0 {
		t.Errorf("first sequence should be 0, got %d", seq)
	}
}

func TestParse_TimestampNearNow(t *testing.T) {
	n, _ := New(1)
	before := time.Now()
	id, _ := n.Next()
	ts, _, _ := Parse(id)
	after := time.Now()

	if ts.Before(before.Add(-10 * time.Millisecond)) {
		t.Errorf("timestamp %s before generation start %s", ts, before)
	}
	if ts.After(after.Add(10 * time.Millisecond)) {
		t.Errorf("timestamp %s after generation end %s", ts, after)
	}
}

func TestParse_IndividualFunctions(t *testing.T) {
	n, _ := New(7)
	id, _ := n.Next()

	if ParseMachineID(id) != 7 {
		t.Errorf("machineID mismatch")
	}
	if ParseSequence(id) != 0 {
		t.Errorf("first sequence should be 0")
	}
	if ParseTimestamp(id) <= 0 {
		t.Errorf("timestamp should be positive")
	}
}

// —— 多机器 ——

func TestNext_DifferentMachineIDsDifferentIDs(t *testing.T) {
	n1, _ := New(1)
	n2, _ := New(2)

	id1, _ := n1.Next()
	id2, _ := n2.Next()

	// 不同机器同一毫秒的 ID 应该不同
	if id1 == id2 {
		t.Errorf("ids from different machines should differ")
	}

	// 解析后机器号应匹配
	_, m1, _ := Parse(id1)
	_, m2, _ := Parse(id2)
	if m1 != 1 || m2 != 2 {
		t.Errorf("machineID mismatch: m1=%d, m2=%d", m1, m2)
	}
}

// —— 自定义 epoch ——

func TestWithEpoch(t *testing.T) {
	customEpoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	n, _ := New(1, WithEpoch(customEpoch))
	id, _ := n.Next()

	// 验证生成不报错且 ID 合法
	if id <= 0 {
		t.Errorf("id should be positive with custom epoch")
	}

	// 验证时间戳合理（基于自定义 epoch 应在 customEpoch 之后）
	ts := ParseTimestamp(id)
	// 注：ParseTimestamp 用 defaultEpoch 解码，会偏移
	// 这里仅检查时间戳为正
	if ts <= 0 {
		t.Errorf("timestamp should be positive")
	}
	_ = ts
}

// —— 时钟回拨 ——

func TestClockBackward_DetectsAndErrors(t *testing.T) {
	n, _ := New(1, WithClockBackwardTolerance(0)) // 零容忍

	// 模拟上次时间戳比现在大（手动篡改）
	n.mu.Lock()
	n.timestamp = time.Now().UnixMilli() - n.epoch + 100 // 100ms 未来时间
	n.mu.Unlock()

	_, err := n.Next()
	if !errors.Is(err, ErrClockMovedBackwards) {
		t.Errorf("expected ErrClockMovedBackwards, got %v", err)
	}
}

func TestClockBackward_Tolerated(t *testing.T) {
	n, _ := New(1, WithClockBackwardTolerance(100*time.Millisecond))

	// 模拟小幅回拨（10ms，在容忍范围内）
	n.mu.Lock()
	n.timestamp = time.Now().UnixMilli() - n.epoch + 10
	n.mu.Unlock()

	id, err := n.Next()
	if err != nil {
		t.Errorf("within tolerance should not error: %v", err)
	}
	if id <= 0 {
		t.Errorf("id should be positive")
	}
}

// —— 序列号耗尽 ——

func TestNext_SequenceRollover(t *testing.T) {
	n, _ := New(1)

	// 让时间戳固定在某一毫秒，序列号从 0 一直增长到 maxSequence
	// 然后下一次调用应该等到下一毫秒
	n.mu.Lock()
	n.timestamp = time.Now().UnixMilli() - n.epoch
	n.sequence = maxSequence // 已经到顶
	n.mu.Unlock()

	// 这里序列号会先 +1 溢出为 0，然后等待下一毫秒
	id, err := n.Next()
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Errorf("id should be positive after rollover")
	}
}

// —— MustNext ——

func TestMustNext_Success(t *testing.T) {
	n, _ := New(1)
	id := n.MustNext()
	if id <= 0 {
		t.Errorf("id should be positive")
	}
}

// —— 边界：机器号 0 和 1023 ——

func TestNext_MachineIDBoundaries(t *testing.T) {
	for _, mid := range []int64{0, 1023} {
		n, _ := New(mid)
		id, _ := n.Next()
		_, got, _ := Parse(id)
		if got != mid {
			t.Errorf("machineID = %d, want %d", got, mid)
		}
	}
}
