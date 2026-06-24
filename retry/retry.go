package retry

import "time"

// Retrier 重试策略接口
type Retrier interface {
	// Next 返回下一次重试的等待时间，false 表示不再重试
	Next() (time.Duration, bool)
	// Reset 重置重试计数
	Reset()
	// Count 返回已重试次数
	Count() int
}
