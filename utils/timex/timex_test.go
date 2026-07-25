package timex

import (
	"testing"
	"time"
)

func TestNowUnix_CloseToStdlib(t *testing.T) {
	before := time.Now().Unix()
	got := NowUnix()
	after := time.Now().Unix()
	if got < before || got > after {
		t.Errorf("NowUnix() = %d, 不在 [%d, %d]", got, before, after)
	}
}

func TestNowMs_CloseToStdlib(t *testing.T) {
	before := time.Now().UnixMilli()
	got := NowMs()
	after := time.Now().UnixMilli()
	if got < before || got > after {
		t.Errorf("NowMs() = %d, 不在 [%d, %d]", got, before, after)
	}
}

func TestNowString_DefaultLayout(t *testing.T) {
	got := NowString()
	if _, err := time.ParseInLocation(DateTime, got, time.Local); err != nil {
		t.Errorf("NowString() = %q 无法按默认布局解析: %v", got, err)
	}
}

func TestFormatUnix_KnownTimestamp(t *testing.T) {
	ts := int64(1622534400) // 2021-06-01 08:00:00 UTC
	if got := FormatUnix(ts); got != time.Unix(ts, 0).Format(DateTime) {
		t.Errorf("FormatUnix 默认布局不匹配: %q", got)
	}
	if got := FormatUnix(ts, DateOnly); got != time.Unix(ts, 0).Format(DateOnly) {
		t.Errorf("FormatUnix DateOnly 不匹配: %q", got)
	}
}

func TestParse_DefaultAndCustomLayout(t *testing.T) {
	if _, err := Parse("2024-01-15 10:30:00"); err != nil {
		t.Errorf("默认布局解析失败: %v", err)
	}
	if _, err := Parse("2024-01-15", DateOnly); err != nil {
		t.Errorf("DateOnly 布局解析失败: %v", err)
	}
	if _, err := Parse("invalid"); err == nil {
		t.Error("非法输入应返回 error")
	}
}

// BeginningOfDay/EndOfDay 构成当天闭区间
func TestBeginningAndEndOfDay(t *testing.T) {
	now := time.Date(2024, 3, 15, 13, 45, 30, 123, time.Local)
	start := BeginningOfDay(now)
	end := EndOfDay(now)
	if start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 {
		t.Errorf("BeginningOfDay 不是 00:00:00, got %v", start)
	}
	if end.Hour() != 23 || end.Minute() != 59 || end.Second() != 59 {
		t.Errorf("EndOfDay 不是 23:59:59, got %v", end)
	}
	if !start.Before(now) || !end.After(now) {
		t.Errorf("当天 %v 不在 [%v, %v] 区间内", now, start, end)
	}
}

// BeginningOfWeek 必须是周一 00:00
func TestBeginningOfWeek_IsMonday(t *testing.T) {
	// 2024-03-15 是周五
	fri := time.Date(2024, 3, 15, 10, 0, 0, 0, time.Local)
	start := BeginningOfWeek(fri)
	if start.Weekday() != time.Monday {
		t.Errorf("BeginningOfWeek 应为周一, got %v", start.Weekday())
	}
	if start.Hour() != 0 {
		t.Errorf("BeginningOfWeek 应为 00:00, got %v", start)
	}
	// 周一到周五相差 4 天
	if diff := fri.Sub(start); diff.Hours() < 4*24 || diff.Hours() >= 5*24 {
		t.Errorf("BeginningOfWeek 到周五间隔异常: %v", diff)
	}
}

// BeginningOfMonth/EndOfMonth 构成本月闭区间
func TestBeginningAndEndOfMonth(t *testing.T) {
	mid := time.Date(2024, 2, 15, 12, 0, 0, 0, time.Local)
	start := BeginningOfMonth(mid)
	end := EndOfMonth(mid)
	if start.Day() != 1 {
		t.Errorf("BeginningOfMonth 应为 1 号, got %v", start.Day())
	}
	// 2024-02 闰年 29 天
	if end.Day() != 29 {
		t.Errorf("EndOfMonth 应为 29 号（2024-02 闰年）, got %v", end.Day())
	}
}

func TestTomorrow(t *testing.T) {
	now := time.Date(2024, 3, 15, 23, 59, 0, 0, time.Local)
	tom := Tomorrow(now)
	if tom.Day() != 16 || tom.Hour() != 0 {
		t.Errorf("Tomorrow 应为次日 00:00, got %v", tom)
	}
}

// 默认参数（不传 t）应取当前时间，且不 panic
func TestRangeHelpers_DefaultArg(t *testing.T) {
	_ = BeginningOfDay()
	_ = EndOfDay()
	_ = BeginningOfWeek()
	_ = BeginningOfMonth()
	_ = EndOfMonth()
	_ = Tomorrow()
}
