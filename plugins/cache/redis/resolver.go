// URL scheme resolver 注册：redis:// → redis.New()
//
// 启用方式：
//
//	import _ "github.com/go-zeus/zeus/plugins/cache/redis"
//	c, _ := cache.NewFromURL("redis://127.0.0.1:6379/0?pool=50&name=user-cache")
//
// 设计目的：让 cache 也支持 URL 字符串驱动，与 registry 行为一致。
// redis plugin 自带依赖（github.com/redis/go-redis/v9），所以放在 plugins 下。

package redis

import (
	"net/url"
	"strconv"
	"strings"

	goredis "github.com/redis/go-redis/v9"

	"github.com/go-zeus/zeus/cache"
)

func init() {
	cache.RegisterResolver("redis", resolveFromURL)
}

// resolveFromURL 把 "redis://[user:pass@]host:port[/db]?pool=50&name=...&recordKey=true" 解析为 redis cache 实例。
//
// 支持的 URL 形态：
//   - redis://host:port
//   - redis://host:port/0          （指定 DB）
//   - redis://user:pass@host:port   （用户名密码）
//   - redis://host:port?pool=50&name=user-cache&recordKey=true
//
// query 参数：
//   - pool：连接池大小（默认不设置，由 go-redis 自身决定）
//   - name：metric label 中的 cache 标识；默认 "redis"
//   - recordKey：是否记录 cache_key 到 span；默认 false
//
// tracer/meter 不通过 URL 注入（URL 不应承担依赖注入职责）；由 cache 调用方注入。
// 业务方拿到 Cache 后若需注入 tracer/meter，应直接调用 New(client, WithTracer(...), WithMeter(...))。
func resolveFromURL(rawURL string) (cache.Cache, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	var db int
	if path := strings.TrimPrefix(u.Path, "/"); path != "" {
		if n, e := strconv.Atoi(path); e == nil {
			db = n
		}
	}

	opts := &goredis.Options{
		Addr:     u.Host,
		Password: "", // 默认无密码
		DB:       db,
	}
	if u.User != nil {
		opts.Password, _ = u.User.Password()
		if user := u.User.Username(); user != "" {
			opts.Username = user
		}
	}
	if v := u.Query().Get("pool"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			opts.PoolSize = n
		}
	}

	cli := goredis.NewClient(opts)

	cacheOpts := []Option{WithName("redis")}
	if v := u.Query().Get("name"); v != "" {
		cacheOpts = append(cacheOpts, WithName(v))
	}
	if v := u.Query().Get("recordKey"); v != "" {
		if b, e := strconv.ParseBool(v); e == nil {
			cacheOpts = append(cacheOpts, WithRecordKey(b))
		}
	}

	return New(cli, cacheOpts...), nil
}
