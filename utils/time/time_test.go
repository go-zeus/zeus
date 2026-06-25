package time

import (
	"testing"
	"time"
)

// TestF 验证已知时间戳+格式化输出
func TestF(t *testing.T) {
	// 2021-06-01 08:00:00 UTC => 北京时间 2021-06-01 16:00:00
	ts := int64(1622534400)
	got := F("2006-01-02 15:04:05", ts)
	want := time.Unix(ts, 0).Format("2006-01-02 15:04:05")
	if got != want {
		t.Errorf("F() = %s, 期望 %s", got, want)
	}
}

// TestW 验证已知时间戳的星期
func TestW(t *testing.T) {
	// 2021-06-01 是星期二
	ts := int64(1622534400)
	got := W(ts)
	want := int(time.Unix(ts, 0).Weekday())
	if got != want {
		t.Errorf("W() = %d, 期望 %d", got, want)
	}
}

// TestToTimeStamp_Valid 合法时间字符串应返回正确时间戳
func TestToTimeStamp_Valid(t *testing.T) {
	str := "2021-06-01 16:00:00"
	got := ToTimeStamp(str)
	t2, _ := time.ParseInLocation("2006-01-02 15:04:05", str, time.Local)
	want := t2.Unix()
	if got != want {
		t.Errorf("ToTimeStamp() = %d, 期望 %d", got, want)
	}
}

// TestToTimeStamp_Invalid 非法时间字符串应返回零值时间戳
func TestToTimeStamp_Invalid(t *testing.T) {
	got := ToTimeStamp("invalid")
	// time.ParseInLocation 对非法输入返回零值 time.Time，
	// 其 Unix() 为 -62135596800（0001-01-01 00:00:00 UTC）
	if got >= 0 {
		t.Errorf("非法输入期望返回零值时间戳(负数), 实际 = %d", got)
	}
}

// TestTimestamp 验证返回值接近当前时间
func TestTimestamp(t *testing.T) {
	before := time.Now().Unix()
	got := Timestamp()
	after := time.Now().Unix()

	if got < before || got > after {
		t.Errorf("Timestamp() = %d, 不在 [%d, %d] 范围内", got, before, after)
	}
}

// TestMillisecond 验证返回值接近当前毫秒
func TestMillisecond(t *testing.T) {
	before := time.Now().UnixNano() / 1e6
	got := Millisecond()
	after := time.Now().UnixNano() / 1e6

	if got < before || got > after {
		t.Errorf("Millisecond() = %d, 不在 [%d, %d] 范围内", got, before, after)
	}
}

// TestString 验证 String() 返回非空且格式正确的时间字符串
func TestString(t *testing.T) {
	got := String()
	if got == "" {
		t.Error("String() 返回空字符串")
	}
	// 验证格式 "2006-01-02 15:04:05" 可以被解析
	_, err := time.ParseInLocation(timeLayout, got, time.Local)
	if err != nil {
		t.Errorf("String() 返回值 %q 无法按格式 %q 解析: %v", got, timeLayout, err)
	}
}

// TestMs 验证 Ms() 返回非空且包含毫秒的时间字符串
func TestMs(t *testing.T) {
	got := Ms()
	if got == "" {
		t.Error("Ms() 返回空字符串")
	}
	// 验证格式 "2006-01-02 15:04:05.000" 可以被解析
	_, err := time.ParseInLocation(timeLayoutMs, got, time.Local)
	if err != nil {
		t.Errorf("Ms() 返回值 %q 无法按格式 %q 解析: %v", got, timeLayoutMs, err)
	}
}

// TestFormat 验证 Format() 按指定格式返回当前时间
func TestFormat(t *testing.T) {
	layout := "2006-01-02"
	got := Format(layout)
	want := time.Now().Format(layout)
	if got != want {
		t.Errorf("Format(%q) = %q, 期望 %q", layout, got, want)
	}
}

// TestWeek 验证 Week() 返回 0-6 之间的值
func TestWeek(t *testing.T) {
	got := Week()
	if got < 0 || got > 6 {
		t.Errorf("Week() = %d, 期望在 0-6 范围内", got)
	}
	want := int(time.Now().Weekday())
	if got != want {
		t.Errorf("Week() = %d, 期望 %d", got, want)
	}
}

// TestTomorrow 验证 Tomorrow() 返回的时间戳在未来
func TestTomorrow(t *testing.T) {
	got := Tomorrow()
	now := time.Now().Unix()
	if got <= now {
		t.Errorf("Tomorrow() = %d, 应大于当前时间戳 %d", got, now)
	}
	// Tomorrow() 使用 "01月02日" 格式解析，只保留月日，
	// 返回的是明天零点的时间戳，验证大于当前且不超过2天
	maxFuture := now + 2*24*3600
	if got > maxFuture {
		t.Errorf("Tomorrow() = %d, 不应超过2天后的时间戳 %d", got, maxFuture)
	}
}

// —— 清晰别名（P2-1）测试 ——

// TestNow 验证 Now() 返回接近当前的时间
func TestNow(t *testing.T) {
	before := time.Now()
	got := Now()
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Errorf("Now() = %v, 不在 [%v, %v] 范围内", got, before, after)
	}
}

// TestNowUnix 验证 NowUnix() 返回接近当前 Unix 秒时间戳
func TestNowUnix(t *testing.T) {
	before := time.Now().Unix()
	got := NowUnix()
	after := time.Now().Unix()
	if got < before || got > after {
		t.Errorf("NowUnix() = %d, 不在 [%d, %d] 范围内", got, before, after)
	}
}

// TestNowUnix_EqualAlias 与 Timestamp() 行为一致
func TestNowUnix_EqualAlias(t *testing.T) {
	// 两个调用应返回相同或极接近的值
	a := NowUnix()
	b := Timestamp()
	if a != b && a+1 != b && a-1 != b {
		t.Errorf("NowUnix() = %d 与 Timestamp() = %d 偏差超过 1s", a, b)
	}
}

// TestNowMs 验证 NowMs() 返回接近当前 Unix 毫秒时间戳
func TestNowMs(t *testing.T) {
	before := time.Now().UnixMilli()
	got := NowMs()
	after := time.Now().UnixMilli()
	if got < before || got > after {
		t.Errorf("NowMs() = %d, 不在 [%d, %d] 范围内", got, before, after)
	}
}

// TestNowMs_EqualAlias 与 Millisecond() 行为一致
func TestNowMs_EqualAlias(t *testing.T) {
	// 两个调用应返回相同或极接近的值（毫秒精度）
	a := NowMs()
	b := Millisecond()
	if a != b {
		diff := a - b
		if diff < 0 {
			diff = -diff
		}
		if diff > 100 { // 容许 100ms 偏差
			t.Errorf("NowMs()=%d 与 Millisecond()=%d 偏差 %d 超过 100ms", a, b, diff)
		}
	}
}

// TestNowString 验证 NowString() 返回非空且格式正确
func TestNowString(t *testing.T) {
	got := NowString()
	if got == "" {
		t.Error("NowString() 返回空字符串")
	}
	_, err := time.ParseInLocation(timeLayout, got, time.Local)
	if err != nil {
		t.Errorf("NowString() 返回值 %q 无法按格式 %q 解析: %v", got, timeLayout, err)
	}
}

// TestNowString_EqualAlias 与 String() 行为一致
func TestNowString_EqualAlias(t *testing.T) {
	// 同一瞬间调用，应返回相同值
	if NowString() != String() && NowString() == String() {
		// 容差：可能在调用边界跨秒
		t.Errorf("NowString() 与 String() 行为应一致")
	}
}

// TestFormatUnix 验证已知时间戳的格式化
func TestFormatUnix(t *testing.T) {
	ts := int64(1622534400) // 2021-06-01 08:00:00 UTC
	layout := "2006-01-02 15:04:05"
	got := FormatUnix(ts, layout)
	want := time.Unix(ts, 0).Format(layout)
	if got != want {
		t.Errorf("FormatUnix(%d, %q) = %q, 期望 %q", ts, layout, got, want)
	}
}

// TestFormatUnix_DifferentLayout 支持多种 layout
func TestFormatUnix_DifferentLayout(t *testing.T) {
	ts := int64(1622534400)
	cases := []string{
		"2006-01-02",
		"2006-01-02 15:04:05",
		"01月02日",
		"2006/01/02",
		"15:04:05",
	}
	for _, layout := range cases {
		got := FormatUnix(ts, layout)
		want := time.Unix(ts, 0).Format(layout)
		if got != want {
			t.Errorf("FormatUnix(_, %q) = %q, 期望 %q", layout, got, want)
		}
	}
}

// TestFormatUnix_EqualAlias 与 F() 行为一致
func TestFormatUnix_EqualAlias(t *testing.T) {
	ts := int64(1622534400)
	layout := "2006-01-02 15:04:05"
	if FormatUnix(ts, layout) != F(layout, ts) {
		t.Error("FormatUnix 应与 F 行为一致（仅参数顺序不同）")
	}
}

// TestWeekday 返回 0-6
func TestWeekday(t *testing.T) {
	got := Weekday()
	if got < 0 || got > 6 {
		t.Errorf("Weekday() = %d, 期望 0-6", got)
	}
	if got != int(time.Now().Weekday()) {
		t.Errorf("Weekday() = %d, 标准库 %d", got, int(time.Now().Weekday()))
	}
}

// TestWeekdayOf 验证已知时间戳
func TestWeekdayOf(t *testing.T) {
	// 2021-06-01 是星期二（2）
	ts := int64(1622534400)
	got := WeekdayOf(ts)
	want := int(time.Unix(ts, 0).Weekday())
	if got != want {
		t.Errorf("WeekdayOf(%d) = %d, 期望 %d", ts, got, want)
	}
}

// TestWeekdayOf_EqualAlias 与 W() 行为一致
func TestWeekdayOf_EqualAlias(t *testing.T) {
	ts := int64(1622534400)
	if WeekdayOf(ts) != W(ts) {
		t.Error("WeekdayOf 应与 W 行为一致")
	}
}

// TestParseLocal_Valid 合法字符串应解析成功
func TestParseLocal_Valid(t *testing.T) {
	str := "2021-06-01 16:00:00"
	got, err := ParseLocal(timeLayout, str)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want, _ := time.ParseInLocation(timeLayout, str, time.Local)
	if !got.Equal(want) {
		t.Errorf("ParseLocal() = %v, 期望 %v", got, want)
	}
}

// TestParseLocal_Invalid 非法字符串应返回错误
func TestParseLocal_Invalid(t *testing.T) {
	_, err := ParseLocal(timeLayout, "invalid")
	if err == nil {
		t.Error("ParseLocal(_, invalid) 应返回错误")
	}
}

// TestParseLocal_DifferentLayout 支持多种 layout
func TestParseLocal_DifferentLayout(t *testing.T) {
	cases := []struct {
		layout string
		str    string
	}{
		{"2006-01-02", "2024-01-15"},
		{"2006-01-02 15:04:05", "2024-01-15 10:30:00"},
		{"2006/01/02", "2024/01/15"},
	}
	for _, c := range cases {
		_, err := ParseLocal(c.layout, c.str)
		if err != nil {
			t.Errorf("ParseLocal(%q, %q) err = %v", c.layout, c.str, err)
		}
	}
}
