// URL scheme resolver 注册：cron:// → cron.New(...)
//
// 启用方式：
//
//	import _ "github.com/go-zeus/zeus/plugins/job/cron"
//	s, _ := job.NewSchedulerFromURL("cron://?seconds=true&loc=America/New_York")
//
// 支持的 query 参数：
//   - seconds：启用 6 字段 cron 表达式（含秒）—— "true" / "false"；默认 false
//   - loc：默认时区（IANA 名称，如 Asia/Shanghai）—— 默认 Local
//
// 不支持的参数静默忽略（前向兼容）。

package cron

import (
	"net/url"
	"strconv"
	"time"

	"github.com/go-zeus/zeus/job"
)

func init() {
	job.RegisterResolver("cron", resolveFromURL)
}

// resolveFromURL 把 "cron://?seconds=true&loc=UTC" 解析为 cron.New(...) 实例。
func resolveFromURL(rawURL string) (job.Scheduler, error) {
	var opts []Option

	u, err := url.Parse(rawURL)
	if err == nil && u.Scheme == "cron" {
		q := u.Query()
		if v := q.Get("seconds"); v != "" {
			if b, e := strconv.ParseBool(v); e == nil && b {
				opts = append(opts, WithSeconds())
			}
		}
		if v := q.Get("loc"); v != "" {
			if loc, e := time.LoadLocation(v); e == nil {
				opts = append(opts, WithLocation(loc))
			}
		}
	}

	return New(opts...), nil
}
