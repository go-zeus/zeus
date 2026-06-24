// URL scheme resolver 注册：kafka:// → kafka.New()
//
// 启用方式：
//
//	import _ "github.com/go-zeus/zeus/plugins/mq/kafka"
//	b, _ := mq.NewBrokerFromURL("kafka://127.0.0.1:9092?group=g1")
//
// URL 格式：
//   - kafka://host:port                              （单 broker）
//   - kafka://h1:9092,h2:9092,h3:9092                （多 broker cluster）
//   - kafka://host:9092?group=order-consumers        （指定 consumer group）
//
// query 参数：
//   - group：consumer group ID（默认 "zeus-default"）
//   - version：Kafka broker 版本字符串（默认 "3.5.0"）
//   - timeout：连接超时（time.Duration 字符串，如 10s）
//
// 不支持的参数静默忽略（前向兼容）。
// 高级配置（SASL/TLS/Idempotent）请用 kafka.New + WithProducerConfig。

package kafka

import (
	"net/url"
	"strings"
	"time"

	"github.com/go-zeus/zeus/mq"
)

func init() {
	mq.RegisterResolver("kafka", resolveFromURL)
}

// resolveFromURL 把 "kafka://..." URL 解析为 kafka.New(...) 实例
func resolveFromURL(rawURL string) (mq.Broker, error) {
	opts := parseURLOptions(rawURL)
	return New(opts...)
}

// parseURLOptions 把 kafka:// URL 解析为 New() 的 Option 列表
//
// 单独抽出便于单元测试（不实际建立 Kafka 连接）
func parseURLOptions(rawURL string) []Option {
	var opts []Option

	// 提取 scheme 后的部分（host[?query]）
	// 直接用 url.Parse 解析
	// 多 host（含逗号）场景 url.Parse 不支持，做特殊处理
	rawURL = strings.TrimSpace(rawURL)
	if !strings.HasPrefix(rawURL, "kafka://") {
		opts = append(opts, WithBrokers(rawURL))
		return opts
	}

	body := rawURL[len("kafka://"):]

	var hostPart, queryPart string
	if idx := strings.Index(body, "?"); idx >= 0 {
		hostPart = body[:idx]
		queryPart = body[idx+1:]
	} else {
		hostPart = body
	}

	// 多 broker：逗号分隔
	hosts := splitAndTrim(hostPart, ",")
	if len(hosts) > 0 {
		opts = append(opts, WithBrokers(hosts...))
	}

	if queryPart != "" {
		query, err := url.ParseQuery(queryPart)
		if err == nil {
			if v := query.Get("group"); v != "" {
				opts = append(opts, WithGroup(v))
			}
			if v := query.Get("version"); v != "" {
				opts = append(opts, WithVersion(v))
			}
			if v := query.Get("timeout"); v != "" {
				if d, e := time.ParseDuration(v); e == nil {
					opts = append(opts, WithDialTimeout(d))
				}
			}
		}
	}

	return opts
}

// splitAndTrim 分割字符串并 trim 每段（过滤空段）
func splitAndTrim(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
