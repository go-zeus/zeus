// Package random 提供随机数生成工具，分两条路径：
//
//   - 密码学安全路径（默认）：RangeRand / Int63 / Bytes 基于 crypto/rand，
//     适合令牌、密钥、ID 等安全场景（~70ns/call）
//   - 快速伪随机路径（Fast* 前缀）：FastRange / FastInt / FastString 基于
//     math/rand/v2，适合 jitter / 采样 / 测试数据 / 随机后缀等无安全要求场景（~5ns/call）
//
// 快速路径非密码学安全，调用方按需选择。
//
// 不做的事：
//   - 不提供 Min/Max/Abs 等通用数学函数（Go 1.21+ 已有 min/max 内置 + math.Abs）
//   - 不提供 shuffle / sample（避免重复造轮子）
package random

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	mathrand "math/rand/v2"
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

// —— 快速伪随机路径（math/rand/v2，非密码学安全）——
//
// 适用：jitter 抖动、退避抖动、采样、测试数据、随机后缀等无安全要求的场景。
// math/rand/v2 顶层函数并发安全且无全局锁（ChaCha8 自动种子），适合高频调用。

// alphanum 快速随机字符串的字符集（数字 + 大小写字母）
const alphanum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// FastRange 生成闭区间 [min, max] 的伪随机整数（非密码学安全，极快）。
//
// min > max 时 panic（与 math/rand/v2 惯例对齐，调用方应保证 min<=max）。
func FastRange(min, max int64) int64 {
	if min > max {
		panic("random: min is greater than max")
	}
	return min + int64(mathrand.Uint64N(uint64(max-min)+1))
}

// FastInt 返回 [0, n) 的伪随机 int（非密码学安全）。
// n <= 0 时 panic（透传 math/rand/v2 行为）。
func FastInt(n int) int {
	return mathrand.IntN(n)
}

// FastString 生成 n 位字母数字随机字符串（大小写字母+数字，非密码学安全）。
// n <= 0 返回空串。密码学场景请用 Bytes + 自行编码。
func FastString(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = alphanum[mathrand.IntN(len(alphanum))]
	}
	return string(b)
}
