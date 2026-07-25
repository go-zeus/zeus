// Package timex 提供常用时间便利工具，聚焦标准库写法繁琐的高频场景。
//
// 设计取舍（避免成为"对 time.Now().Unix() 毫无增值的薄包装"）：
//   - 内置常用布局常量，消除到处复制 "2006-01-02 15:04:05" 魔法字符串
//   - Format/Parse 省略 layout 时用 DateTime 默认值，常见场景零样板
//   - 提供按天/周/月起止时间（本地时区），覆盖业务 DB 查询范围的高频需求
//   - 当前秒/毫秒/字符串的简短入口（日志、缓存 key、DB 字段）
//
// 包名 timex 而非 time，避免与标准库 time 冲突导致调用方被迫起别名导入。
package timex

import "time"

// 常用时间布局（消除魔法字符串）。与标准库 time.DateTime/time.DateOnly 对齐并扩展。
const (
	DateTime   = "2006-01-02 15:04:05" // 日期 + 时间（最常用）
	DateTimeMs = "2006-01-02 15:04:05.000" // 带毫秒
	DateOnly   = "2006-01-02"              // 仅日期
	TimeOnly   = "15:04:05"                // 仅时间
)

const defaultLayout = DateTime

// —— 当前时间快捷入口 ——

// Now 返回当前 time.Time（等价 time.Now()，提供 timex 命名空间下的统一入口）。
func Now() time.Time { return time.Now() }

// NowUnix 返回当前 Unix 秒时间戳。
func NowUnix() int64 { return time.Now().Unix() }

// NowMs 返回当前 Unix 毫秒时间戳。
func NowMs() int64 { return time.Now().UnixMilli() }

// NowString 返回当前时间格式化字符串，layout 省略时用 DateTime。
func NowString(layout ...string) string {
	return time.Now().Format(pickLayout(layout))
}

// —— 格式化 ——

// Format 格式化 time.Time，layout 省略时用 DateTime。
func Format(t time.Time, layout ...string) string {
	return t.Format(pickLayout(layout))
}

// FormatUnix 格式化 Unix 秒时间戳，layout 省略时用 DateTime。
func FormatUnix(ts int64, layout ...string) string {
	return time.Unix(ts, 0).Format(pickLayout(layout))
}

// —— 解析（本地时区）——

// Parse 按布局解析字符串到 time.Time（本地时区），layout 省略时用 DateTime。
// 解析失败返回零值 time.Time + error（不吞错误）。
func Parse(s string, layout ...string) (time.Time, error) {
	return time.ParseInLocation(pickLayout(layout), s, time.Local)
}

// —— 时间范围（业务查询高频，stdlib 写法繁琐）——
//
// 所有范围基于本地时区（time.Local）；可选参数 t 缺省时取当前时间。
// 返回 [start, end] 闭区间风格，便于 DB 时间字段范围查询。

// BeginningOfDay 返回 t 所在天的 00:00:00（本地时区）。
func BeginningOfDay(t ...time.Time) time.Time {
	now := pickTime(t)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
}

// EndOfDay 返回 t 所在天的 23:59:59.999999999（本地时区）。
func EndOfDay(t ...time.Time) time.Time {
	now := pickTime(t)
	return time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, time.Local)
}

// BeginningOfWeek 返回 t 所在周的周一 00:00:00（本地时区，ISO 惯例周一起算）。
func BeginningOfWeek(t ...time.Time) time.Time {
	now := pickTime(t)
	// Go 的 Weekday(): 周日=0..周六=6；换算到周一的偏移天数
	daysSinceMonday := int(now.Weekday()) - 1
	if daysSinceMonday < 0 {
		daysSinceMonday = 6 // 周日回到上一个周一
	}
	return BeginningOfDay(now).AddDate(0, 0, -daysSinceMonday)
}

// BeginningOfMonth 返回 t 所在月 1 号的 00:00:00（本地时区）。
func BeginningOfMonth(t ...time.Time) time.Time {
	now := pickTime(t)
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
}

// EndOfMonth 返回 t 所在月最后一天的 23:59:59.999999999（本地时区）。
func EndOfMonth(t ...time.Time) time.Time {
	return BeginningOfMonth(pickTime(t)).AddDate(0, 1, 0).Add(-time.Nanosecond)
}

// Tomorrow 返回明天 00:00:00（本地时区）。
func Tomorrow(t ...time.Time) time.Time {
	return BeginningOfDay(t...).AddDate(0, 0, 1)
}

// —— 内部辅助 ——

// pickTime 从可变参数取时间，缺省返回当前时间。
func pickTime(t []time.Time) time.Time {
	if len(t) > 0 {
		return t[0]
	}
	return time.Now()
}

// pickLayout 从可变参数取布局，缺省或空串返回默认布局。
func pickLayout(layout []string) string {
	if len(layout) > 0 && layout[0] != "" {
		return layout[0]
	}
	return defaultLayout
}
