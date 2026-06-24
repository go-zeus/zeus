// Package uuid 提供 RFC 4122 兼容的 UUID 生成与解析工具。
//
// 设计目的：
//   - 生成符合 RFC 4122 v4 规范的随机 UUID（含 version + variant 标记）
//   - 通过 PostgreSQL `uuid` 列、第三方 UUID 校验库等严格校验
//   - 零依赖（仅 crypto/rand + encoding/hex）
//
// 不做的事：
//   - 不提供 v1（时间 + MAC）/ v3/v5（命名空间哈希）/ v7（时间有序）—— 用户按需扩展
//   - 不做 URN 前缀（"urn:uuid:..."）—— 调用方按需拼接
package uuid

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

const uuidLen = 16

func New() string {
	uuid, _ := GenerateUUID()
	return uuid
}

// GenerateRandomBytes is used to generate random bytes of given size.
func GenerateRandomBytes(size int) ([]byte, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("uuid: failed to read random bytes: %w", err)
	}
	return buf, nil
}

// GenerateUUID 生成符合 RFC 4122 v4 规范的随机 UUID
//
// v4 标记位（RFC 4122 §4.4）：
//   - 时间高位版本字段（byte[6] 高 4 位）：固定为 0100 = 0x4
//   - 时钟序列变体字段（byte[8] 高 2 位）：固定为 10 = 变体 1（标准 RFC 4122 layout）
//
// 校验：生成的 UUID 可通过 `uuid-validator`、PostgreSQL `uuid` 类型、Python `uuid.UUID` 等严格校验
func GenerateUUID() (string, error) {
	buf, err := GenerateRandomBytes(uuidLen)
	if err != nil {
		return "", err
	}
	applyV4Markers(buf)
	return FormatUUID(buf)
}

// applyV4Markers 原地设置 RFC 4122 v4 版本与变体标记
//
// 实现细节：
//
//	buf[6] = (buf[6] & 0x0f) | 0x40   // 高 4 位 = 0100 (version 4)
//	buf[8] = (buf[8] & 0x3f) | 0x80   // 高 2 位 = 10   (variant 1, RFC 4122)
func applyV4Markers(buf []byte) {
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
}

// FormatUUID 把 16 字节 raw bytes 格式化为标准 UUID 字符串
//
// 输入必须是 16 字节，否则返回 error
// 注意：本函数不校验 version/variant 标记，仅做格式转换。
// 调用方如需确保 v4 标记正确，应使用 GenerateUUID 或预先调用 applyV4Markers
func FormatUUID(buf []byte) (string, error) {
	if buflen := len(buf); buflen != uuidLen {
		return "", fmt.Errorf("uuid: wrong length byte slice (%d)", buflen)
	}

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		buf[0:4],
		buf[4:6],
		buf[6:8],
		buf[8:10],
		buf[10:16]), nil
}

// ParseUUID 解析标准 UUID 字符串为 16 字节 raw bytes
//
// 输入要求：长度 36（8-4-4-4-12 格式），第 9/14/19/24 位是 '-'
// 返回的 bytes 不再做 version/variant 校验（调用方如需严格校验，自行检查 byte[6]>>4==4）
func ParseUUID(uuid string) ([]byte, error) {
	if len(uuid) != 2*uuidLen+4 {
		return nil, fmt.Errorf("uuid: wrong length string")
	}

	if uuid[8] != '-' ||
		uuid[13] != '-' ||
		uuid[18] != '-' ||
		uuid[23] != '-' {
		return nil, fmt.Errorf("uuid: improperly formatted")
	}

	hexStr := uuid[0:8] + uuid[9:13] + uuid[14:18] + uuid[19:23] + uuid[24:36]

	ret, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("uuid: decode hex: %w", err)
	}
	if len(ret) != uuidLen {
		return nil, fmt.Errorf("uuid: decoded hex wrong length")
	}

	return ret, nil
}

// IsV4 检查 raw bytes 是否带 v4 标记。
//
// 典型场景：ParseUUID 后做严格校验，拒绝非 v4 UUID（如 v1/v7 时间序 UUID）。
func IsV4(buf []byte) bool {
	if len(buf) != uuidLen {
		return false
	}
	return (buf[6]>>4)&0x0f == 0x04 && (buf[8]>>6)&0x03 == 0x02
}
