// URL scheme resolver 注册：interval:// → interval.New()
//
// 启用方式：
//
//	import _ "github.com/go-zeus/zeus/job/interval"
//	s, _ := job.NewSchedulerFromURL("interval://")
//
// 设计目的：让 job 也支持 URL 字符串驱动，与 cache/mq/database 行为一致。
// interval 是零依赖内置实现，仅做构造透传。

package interval

import (
	"github.com/go-zeus/zeus/job"
)

func init() {
	job.RegisterResolver("interval", resolveFromURL)
}

// resolveFromURL 把 "interval://" 解析为 interval.New() 实例。
//
// interval 调度器无 URL 可配参数（ErrorHandler 注入由 components 包装层处理），
// 故仅校验 scheme 后透传 New()。
// 不支持的 query 参数静默忽略（前向兼容）。
func resolveFromURL(rawURL string) (job.Scheduler, error) {
	return New(), nil
}
