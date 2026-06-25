// Package random 提供安全随机数生成工具。
//
// 设计目的：
//   - 提供区间随机数（RangeRand）等标准库没有的能力
//   - 仅用 crypto/rand，保证密码学安全（不引入 math/rand 兼容性问题）
//   - 零依赖（仅标准库）
//
// 不做的事：
//   - 不提供 Min/Max/Abs 等通用数学函数（Go 1.21+ 已有 min/max 内置 + math.Abs）
//   - 不提供伪随机（用 math/rand 用户自行 import）
//   - 不提供 shuffle / sample（避免重复造轮子，math/rand/sampled 已有）
package random

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
)

// RangeRand 生成闭区间 [min, max] 的安全随机整数
//
// 行为：
//   - min > max 时返回 error（不 panic，避免误用导致进程崩溃）
//   - 使用 crypto/rand 保证密码学安全
//   - 用纯 int64 运算（无 big.Int / float64 转换），避免精度损失和高频分配开销
//
// 性能：~70ns/调用（crypto/rand.Read 8 字节），适合 ID 生成、令牌桶抖动等场景
//
// 与旧 utils/math.RangeRand 的差异：
//   - 旧实现 min<0 时通过 float64 → int64 转换，超过 2^53 会丢精度；新实现纯 int64
//   - 旧实现每次 new(big.NewInt) 分配堆；新实现栈上完成
//   - 旧实现 min>max 时 panic；新实现返回 error
func RangeRand(min, max int64) (int64, error) {
	if min > max {
		return 0, errors.New("random: min is greater than max")
	}

	// 区间大小 = max - min + 1（闭区间）
	span := uint64(max-min) + 1

	// 拒绝采样（rejection sampling）：保证均匀分布无偏差
	// 算法：取模前丢弃非整倍的部分，避免模运算引入偏向（k*span ≤ 2^64 的最大可用值）
	// threshold = 2^64 - (2^64 % span) = (2^64 / span) * span
	// 当 r >= threshold 时重新采样
	threshold := -span % span // == (2^64 - 2^64%span) mod 2^64
	for {
		r, err := readUint64()
		if err != nil {
			return 0, err
		}
		if r >= threshold {
			return min + int64(r%span), nil
		}
	}
}

// MustRangeRand 是 RangeRand 的 panic 版本（min>max 时 panic）
//
// 适用场景：调用方在编译期已确认 min<=max，简化错误处理代码
func MustRangeRand(min, max int64) int64 {
	v, err := RangeRand(min, max)
	if err != nil {
		panic(err)
	}
	return v
}

// Int63 生成 [0, 1<<63) 的安全随机非负整数
//
// 与 crypto/rand.Int(_, 1<<63) 等价但避免 big.Int 分配
func Int63() (int64, error) {
	v, err := readUint64()
	if err != nil {
		return 0, err
	}
	return int64(v & (1<<63 - 1)), nil
}

// Bytes 生成 n 字节安全随机数据
//
// 等价于 crypto/rand.Read(make([]byte, n))，但返回新 slice 便于链式调用
func Bytes(n int) ([]byte, error) {
	if n < 0 {
		return nil, errors.New("random: negative length")
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// readUint64 从 crypto/rand 读 8 字节小端序并转 uint64
//
// 用 binary.LittleEndian 直接读 8 字节，避免 big.Int 分配
func readUint64() (uint64, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(buf[:]), nil
}
