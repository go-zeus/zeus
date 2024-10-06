package time

import (
	"time"
)

const timeLayout = "2006-01-02 15:04:05"
const timeLayoutMs = "2006-01-02 15:04:05.000"
const timeYmd = "2006-01-02"
const timeMd = "01月02日"

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
