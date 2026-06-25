package time

import (
	"time"
)

const timeLayout = "2006-01-02 15:04:05"
const timeLayoutMs = "2006-01-02 15:04:05.000"
const timeYmd = "2006-01-02"

// Timestamp 时间戳(当前)
func Timestamp() int64 {
	return time.Now().Unix()
}

// Millisecond 毫秒(当前)
func Millisecond() int64 {
	return time.Now().UnixNano() / 1e6
}

// String 当前时间字符串 "2006-01-02 15:04:05"
func String() string {
	return time.Now().Format(timeLayout)
}

// Ms 当前时间毫秒字符串 "2006-01-02 15:04:05.000"
func Ms() string {
	return time.Now().Format(timeLayoutMs)
}

// Format 格式化当前时间
func Format(format string) string {
	return time.Now().Format(format)
}

// F 格式化时间
func F(format string, t int64) string {
	return time.Unix(t, 0).Format(format)
}

// Week 当前周
func Week() int {
	return int(time.Now().Weekday())
}

// Tomorrow 明天
func Tomorrow() int64 {
	timeStr := time.Now().Format(timeYmd)
	//使用Parse 默认获取为UTC时区 需要获取本地时区 所以使用ParseInLocation
	t2, _ := time.ParseInLocation(timeYmd, timeStr, time.Local)
	return t2.AddDate(0, 0, 1).Unix()
}

// W 指定时间周
func W(t int64) int {
	return int(time.Unix(t, 0).Weekday())
}

// ToTimeStamp 字符时间转时间戳
func ToTimeStamp(str string) int64 {
	stamp, _ := time.ParseInLocation(timeLayout, str, time.Local)
	return stamp.Unix()
}

// —— 清晰别名（P2-1 补充） ——

// Now 返回当前 time.Time（与标准库 time.Now() 对齐）
//
// 用途：让使用方在 import 别名场景下有清晰入口
//
// 示例：
//
//	import zetime "github.com/go-zeus/zeus/utils/time"
//	t := zetime.Now()
func Now() time.Time {
	return time.Now()
}

// NowUnix 返回当前 Unix 秒时间戳（Timestamp 的清晰别名）
//
// 等价于 Timestamp()，但命名与标准库 time.Now().Unix() 对齐
func NowUnix() int64 {
	return time.Now().Unix()
}

// NowMs 返回当前 Unix 毫秒时间戳（Millisecond 的清晰别名）
//
// 等价于 Millisecond()
func NowMs() int64 {
	return time.Now().UnixMilli()
}

// NowString 返回当前时间格式化字符串（String 的清晰别名）
//
// 等价于 String()，但避免在代码中与 type String 混淆
func NowString() string {
	return time.Now().Format(timeLayout)
}

// FormatUnix 按指定 layout 格式化 Unix 时间戳（F 的清晰别名）
//
// 参数：
//   - ts: Unix 秒时间戳
//   - layout: 标准 Go 时间布局（参考 time.Format）
//
// 示例：
//
//	out := FormatUnix(1622534400, "2006-01-02 15:04:05")
func FormatUnix(ts int64, layout string) string {
	return time.Unix(ts, 0).Format(layout)
}

// Weekday 返回当前星期（Week 的清晰别名）
//
// 返回 0 (Sunday) - 6 (Saturday)
func Weekday() int {
	return int(time.Now().Weekday())
}

// WeekdayOf 返回指定 Unix 时间戳的星期（W 的清晰别名）
//
// 参数：ts Unix 秒时间戳
//
// 返回 0 (Sunday) - 6 (Saturday)
func WeekdayOf(ts int64) int {
	return int(time.Unix(ts, 0).Weekday())
}

// ParseLocal 按指定 layout 解析字符串到 time.Time（使用本地时区）
//
// 行为：等价于 time.ParseInLocation(layout, str, time.Local)
//
// 返回：
//   - 解析成功：time.Time + nil
//   - 解析失败：零值 time.Time + error
//
// 示例：
//
//	t, err := ParseLocal("2006-01-02 15:04:05", "2024-01-15 10:30:00")
func ParseLocal(layout, str string) (time.Time, error) {
	return time.ParseInLocation(layout, str, time.Local)
}
