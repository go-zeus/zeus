// W3C Baggage 编解码实现。
//
// 格式规范（W3C Baggage, RFC 类似）：
//
//	baggage = listEntry *( OWS "," OWS listEntry )
//	listEntry = key OWS "=" OWS value [ *( OWS ";" OWS properties ) ]
//	key = token
//	value = token / percent-encoded
//
// 简化决策：
//   - 不处理 properties（;key=value 部分），解码时忽略
//   - 编码时对非 token 字符一律 percent-encode
//   - token 字符集：字母数字 + !#$%&'*+-.^_`|~ (RFC 7230)
package propagation

import (
	"fmt"
	"net/url"
	"strings"
)

// isTokenChar 判断字符是否为合法 token 字符（无需 encode）。
//
// 参考 RFC 7230: tchar = "!" / "#" / "$" / "%" / "&" / "'" / "*" / "+" / "-" / "." /
// "^" / "_" / "`" / "|" / "~" / DIGIT / ALPHA
func isTokenChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= '0' && c <= '9':
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}
	return false
}

// Encode 将 Bag 编码为 W3C baggage header 值。
//
// 例如：Bag{tenant=acme, region=cn-east-1} → "tenant=acme,region=cn-east-1"
// 空 Bag 返回空字符串。
//
// 编码规则：
//   - Key 直接输出（调用方应保证 key 是 token，否则编码时按非 token 字符 encode）
//   - Value 中的非 token 字符 percent-encode（参考 RFC 3986）
//   - 多个 Entry 用 "," 分隔（无空格，紧凑形式）
func Encode(b *Bag) string {
	if b == nil || b.Len() == 0 {
		return ""
	}
	var sb strings.Builder
	for i, e := range b.entries {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(encodeToken(e.Key))
		sb.WriteByte('=')
		sb.WriteString(encodeValue(e.Value))
	}
	return sb.String()
}

// encodeToken 编码 key（非 token 字符 percent-encode）
func encodeToken(s string) string {
	for i := 0; i < len(s); i++ {
		if !isTokenChar(s[i]) {
			return url.PathEscape(s)
		}
	}
	return s
}

// encodeValue 编码 value（非 token 字符 percent-encode；空格也算非 token）
func encodeValue(s string) string {
	for i := 0; i < len(s); i++ {
		if !isTokenChar(s[i]) {
			return url.PathEscape(s)
		}
	}
	return s
}

// Decode 解析 W3C baggage header 值为 Bag。
//
// 输入示例："tenant=acme,region=cn-east-1"
// 容错策略：
//   - 空 / 全空白 → 返回 nil
//   - 单个 Entry 解析失败 → 跳过，继续解析其余
//   - 同 Key 多次出现 → 后写覆盖前写
//   - properties（;key=value）忽略不解析
//   - value 中的 percent-encode 自动 decode
func Decode(raw string) *Bag {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	bag := NewBag()
	for _, item := range strings.Split(raw, ",") {
		entry, ok := decodeEntry(item)
		if !ok {
			continue
		}
		bag = bag.With(entry.Key, entry.Value)
	}
	if bag.Len() == 0 {
		return nil
	}
	return bag
}

// decodeEntry 解析单个 Entry（可能带 properties，忽略之）
//
// 输入格式：key=value 或 key=value;prop1=v1;prop2=v2
// 容错：缺失 "=" → 解析失败
func decodeEntry(item string) (Entry, bool) {
	item = strings.TrimSpace(item)
	if item == "" {
		return Entry{}, false
	}
	// 切掉 properties（";" 之后的部分）
	if idx := strings.Index(item, ";"); idx >= 0 {
		item = strings.TrimSpace(item[:idx])
		if item == "" {
			return Entry{}, false
		}
	}
	eq := strings.Index(item, "=")
	if eq <= 0 {
		return Entry{}, false
	}
	key := strings.TrimSpace(item[:eq])
	value := strings.TrimSpace(item[eq+1:])
	if key == "" {
		return Entry{}, false
	}
	decodedKey, err := decodeToken(key)
	if err != nil {
		return Entry{}, false
	}
	decodedValue, err := decodeToken(value)
	if err != nil {
		return Entry{}, false
	}
	return Entry{Key: decodedKey, Value: decodedValue}, true
}

// decodeToken 解码 percent-encoded token
//
// url.PathUnescape 兼容 percent-decode；非法 escape 返回 error
func decodeToken(s string) (string, error) {
	if !strings.Contains(s, "%") {
		return s, nil
	}
	out, err := url.PathUnescape(s)
	if err != nil {
		return "", fmt.Errorf("invalid percent-encoding %q: %w", s, err)
	}
	return out, nil
}
