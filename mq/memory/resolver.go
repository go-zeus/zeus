// URL scheme resolver 注册：memory:// → memory.New()
//
// 启用方式：
//
//	import _ "github.com/go-zeus/zeus/mq/memory"
//	b, _ := mq.NewBrokerFromURL("memory://")
//
// memory broker 没有可配置项（订阅者 channel 固定无缓冲），
// URL 中 query 参数被静默忽略（前向兼容，未来可加 buffer= 等）。

package memory

import (
	"github.com/go-zeus/zeus/mq"
)

func init() {
	mq.RegisterResolver("memory", resolveFromURL)
}

// resolveFromURL 把 "memory://..." 解析为 memory.New() 实例。
//
// 当前 memory broker 无可配项，任何 query 参数都被静默忽略。
func resolveFromURL(_ string) (mq.Broker, error) {
	return New(), nil
}
