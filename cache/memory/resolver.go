// URL scheme resolver 注册：memory:// → memory.New()
//
// 启用方式：
//
//	import _ "github.com/go-zeus/zeus/cache/memory"
//	c, _ := cache.NewFromURL("memory://?cleanup=60s&name=user-cache")
//
// 设计目的：让 cache 也支持 URL 字符串驱动，与 registry 行为一致。
// 零依赖：仅用标准库（net/url / strconv / time）。

package memory

import (
	"net/url"
	"strconv"
	"time"

	"github.com/go-zeus/zeus/cache"
)

func init() {
	cache.RegisterResolver("memory", resolveFromURL)
}

// resolveFromURL 把 "memory://?cleanup=60s&name=user-cache" 解析为 memory.New(...) 实例。
//
// 支持的 query 参数：
//   - cleanup：后台清理周期（time.Duration 字符串，如 60s / 5m）；默认 60s
//   - name：metric label 中的 cache 标识；默认 "memory"
//   - recordKey：是否记录 cache_key 到 span（"true"/"false"）；默认 false
//
// 不支持的参数静默忽略（前向兼容）。
func resolveFromURL(rawURL string) (cache.Cache, error) {
	opts := []Option{
		WithCleanupInterval(defaultCleanupInterval),
		WithName("memory"),
	}

	u, err := url.Parse(rawURL)
	if err == nil && u.Scheme == "memory" {
		q := u.Query()
		if v := q.Get("cleanup"); v != "" {
			if d, e := time.ParseDuration(v); e == nil {
				opts = append(opts, WithCleanupInterval(d))
			}
		}
		if v := q.Get("name"); v != "" {
			opts = append(opts, WithName(v))
		}
		if v := q.Get("recordKey"); v != "" {
			if b, e := strconv.ParseBool(v); e == nil {
				opts = append(opts, WithRecordKey(b))
			}
		}
	}

	return New(opts...), nil
}
