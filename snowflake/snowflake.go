// Package snowflake 提供 Twitter Snowflake 算法的分布式 ID 生成器。
//
// 设计目标：
//   - 零依赖（仅用标准库）
//   - 趋势递增：高位时间戳，低位机器+序列号
//   - 单机每秒生成 409.6 万个 ID（12 位序列号，每毫秒 4096 个）
//   - 时钟回拨保护：拒绝生成 ID 而非生成重复 ID
//   - 线程安全：基于 sync.Mutex
//
// ID 结构（64 位）：
//
//	0 | 00000000000000000000000000000000000000000 | 0000000000 | 000000000000
//	|                     41 位毫秒时间戳           | 10位机器ID  |  12位序列号
//
// - 1 位符号位（始终为 0）
// - 41 位毫秒时间戳（约 69.7 年）
// - 10 位机器 ID（最多 1024 台机器）
// - 12 位序列号（每毫秒最多 4096 个）
//
// 使用示例：
//
//	n, _ := snowflake.New(1)  // 机器号 1
//	id := n.Next()             // 生成唯一 ID
//	ts, mid, seq := snowflake.Parse(id)
package snowflake

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// —— 常量 ——

const (
	// 总位数
	totalBits = 63 // 1 位符号位保留

	// 各部分位数
	machineIDBits = 10
	sequenceBits  = 12

	// 各部分最大值
	maxMachineID = -1 ^ (-1 << machineIDBits) // 1023
	maxSequence  = -1 ^ (-1 << sequenceBits)  // 4095

	// 位移
	machineIDShift = sequenceBits
	timestampShift = sequenceBits + machineIDBits

	// 起始时间戳（2020-01-01 00:00:00 UTC，毫秒）
	defaultEpoch = 1577836800000
)

// —— Node ——

// Node snowflake 节点
type Node struct {
	mu        sync.Mutex
	epoch     int64 // 起始时间戳（毫秒）
	timestamp int64 // 上次生成 ID 的时间戳（毫秒，相对 epoch）
	machineID int64 // 机器 ID
	sequence  int64 // 当前毫秒内序列号

	// 配置
	clockBackwardTolerance time.Duration // 时钟回拨容忍度（默认 5ms）
}

// Option 节点配置
type Option func(*Node)

// WithEpoch 自定义起始时间（默认 2020-01-01 UTC）
//
// 用途：从其他 snowflake 实现迁移时对齐
func WithEpoch(t time.Time) Option {
	return func(n *Node) {
		n.epoch = t.UnixMilli()
	}
}

// WithClockBackwardTolerance 设置时钟回拨容忍度
//
// 默认 5ms：小于 5ms 的回拨会等待追上，大于则返回 error
// 设为 0 表示零容忍（任何回拨都返回 error）
func WithClockBackwardTolerance(d time.Duration) Option {
	return func(n *Node) {
		n.clockBackwardTolerance = d
	}
}

// New 创建 snowflake 节点
//
// machineID 范围 [0, 1023]，超出返回错误
//
// 用法：
//
//	n, err := snowflake.New(1)
//	if err != nil { ... }
//	id := n.Next()
func New(machineID int64, opts ...Option) (*Node, error) {
	if machineID < 0 || machineID > maxMachineID {
		return nil, fmt.Errorf("snowflake: machineID must be in [0, %d], got %d", maxMachineID, machineID)
	}
	n := &Node{
		epoch:                  defaultEpoch,
		machineID:              machineID,
		clockBackwardTolerance: 5 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(n)
	}
	return n, nil
}

// MustNew 创建节点，失败 panic（开发期配置错误用）
func MustNew(machineID int64, opts ...Option) *Node {
	n, err := New(machineID, opts...)
	if err != nil {
		panic(err)
	}
	return n
}

// Next 生成下一个唯一 ID（线程安全）
//
// 时钟回拨行为：
//   - 回拨量 ≤ tolerance：等待时间追平后生成
//   - 回拨量 > tolerance：返回 ErrClockMovedBackwards
func (n *Node) Next() (int64, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	now := time.Now().UnixMilli() - n.epoch

	// 时钟回拨检查
	if now < n.timestamp {
		diff := time.Duration(n.timestamp-now) * time.Millisecond
		if diff <= n.clockBackwardTolerance {
			// 等待追上
			time.Sleep(diff)
			now = time.Now().UnixMilli() - n.epoch
		} else {
			return 0, fmt.Errorf("%w: backward %s", ErrClockMovedBackwards, diff)
		}
	}

	switch {
	case now == n.timestamp:
		// 同一毫秒内序列号递增
		n.sequence = (n.sequence + 1) & maxSequence
		if n.sequence == 0 {
			// 序列号耗尽，等到下一毫秒
			for now <= n.timestamp {
				now = time.Now().UnixMilli() - n.epoch
			}
		}
	case now > n.timestamp:
		// 新一毫秒，重置序列号
		n.sequence = 0
	}

	n.timestamp = now

	return (now << timestampShift) |
		(n.machineID << machineIDShift) |
		n.sequence, nil
}

// MustNext 生成 ID，失败 panic（业务代码确定不会回拨时用）
func (n *Node) MustNext() int64 {
	id, err := n.Next()
	if err != nil {
		panic(err)
	}
	return id
}

// Parse 解析 ID 为 (时间戳, 机器ID, 序列号)
//
// 时间戳是 UTC 时间（基于节点的 epoch）。
// 若 ID 是用默认 epoch 生成的，得到的时间就是实际生成时间。
func Parse(id int64) (timestamp time.Time, machineID, sequence int64) {
	t := (id >> timestampShift) + defaultEpoch
	return time.UnixMilli(t).UTC(), (id >> machineIDShift) & maxMachineID, id & maxSequence
}

// ParseTimestamp 仅取时间戳（避免 time.Time 转换开销）
func ParseTimestamp(id int64) int64 {
	return (id >> timestampShift) + defaultEpoch
}

// ParseMachineID 仅取机器 ID
func ParseMachineID(id int64) int64 {
	return (id >> machineIDShift) & maxMachineID
}

// ParseSequence 仅取序列号
func ParseSequence(id int64) int64 {
	return id & maxSequence
}

// —— 错误 ——

// ErrClockMovedBackwards 时钟回拨超出容忍度
var ErrClockMovedBackwards = errors.New("snowflake: clock moved backwards")

// —— 内部辅助 ——

// compile-time check
var _ = totalBits
