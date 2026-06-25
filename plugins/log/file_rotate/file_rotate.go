// Package file_rotate 提供基于 gopkg.in/natefinch/lumberjack.v2 的日志文件轮转 Writer。
//
// 设计目的：
//   - 实现 log.Writer 接口，对接 zeus 主包 log.Logger
//   - 按文件大小自动切分（lumberjack 内置）
//   - 自动备份 + 压缩 + 保留数量
//   - 输出格式：JSON（默认）或 TEXT（可配）
//
// 不做的事：
//   - 不做日志聚合（ELK/Loki 等用 filebeat/promtail 收集 sidecar）
//   - 不做异步缓冲（lumberjack 内部已并发安全，业务侧按需 wrap）
//   - 不实现 zap/logrus 等第三方格式化（用对应 plugin + lumberjack writeSyncer 模式）
//
// 用法：
//
//	import (
//	    "github.com/go-zeus/zeus/log"
//	    "github.com/go-zeus/zeus/plugins/log/file_rotate"
//	)
//
//	w, err := file_rotate.New("/var/log/zeus/app.log",
//	    file_rotate.WithMaxSize(100),      // 100 MB
//	    file_rotate.WithMaxBackups(7),     // 保留 7 份
//	    file_rotate.WithMaxAge(30),        // 保留 30 天
//	    file_rotate.WithCompress(true),    // gzip 压缩
//	)
//	if err != nil { return err }
//	logger := log.NewLogger(w)
//	defer logger.Close()
//
// logger.Info("user logged in", "user_id", 123)
// // 输出（JSON）：{"time":"2026-06-23T10:00:00Z","level":"INFO","msg":"user logged in","user_id":123}
package file_rotate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/go-zeus/zeus/log"
)

// 默认值
const (
	// DefaultMaxSize 单文件最大 MB（达到后切分）
	DefaultMaxSize = 100
	// DefaultMaxBackups 保留旧文件数量
	DefaultMaxBackups = 7
	// DefaultMaxAge 旧文件保留天数（0 = 不限）
	DefaultMaxAge = 30
)

// Format 输出格式
type Format int

const (
	// FormatJSON JSON 格式（默认，机器可读，对接 ELK/Loki 友好）
	FormatJSON Format = iota
	// FormatText 纯文本 key=value 格式（人类可读，调试方便）
	FormatText
)

// Option 函数式选项
type Option func(*writer)

// WithMaxSize 单文件最大 MB（默认 100）
func WithMaxSize(mb int) Option {
	return func(w *writer) {
		if mb > 0 {
			w.lumberjack.MaxSize = mb
		}
	}
}

// WithMaxBackups 保留旧文件数量（默认 7）
func WithMaxBackups(n int) Option {
	return func(w *writer) {
		if n >= 0 {
			w.lumberjack.MaxBackups = n
		}
	}
}

// WithMaxAge 旧文件保留天数（默认 30；0 = 不限）
func WithMaxAge(days int) Option {
	return func(w *writer) {
		if days >= 0 {
			w.lumberjack.MaxAge = days
		}
	}
}

// WithCompress 是否 gzip 压缩旧文件（默认 false，节省磁盘但增加 CPU）
func WithCompress(b bool) Option {
	return func(w *writer) {
		w.lumberjack.Compress = b
	}
}

// WithLocalTime 备份文件名用本地时间而非 UTC（默认 false = UTC）
func WithLocalTime(b bool) Option {
	return func(w *writer) {
		w.lumberjack.LocalTime = b
	}
}

// WithFormat 设置输出格式（默认 JSON）
func WithFormat(f Format) Option {
	return func(w *writer) {
		w.format = f
	}
}

// WithMinLevel 最小日志级别（低于此级别的丢弃，默认 LevelDebug = 全部输出）
func WithMinLevel(l log.Level) Option {
	return func(w *writer) {
		w.minLevel = l
	}
}

// 编译期检查 writer 实现 log.Writer
var _ log.Writer = (*writer)(nil)

// writer 文件轮转日志写入器
//
// 字段：
//   - lumberjack：底层 lumberjack.Logger，负责文件轮转
//   - format：输出格式（JSON/Text）
//   - minLevel：最小日志级别
//   - mu：保护并发 Write（lumberjack 自身并发安全，但 JSON marshal 后的 write 需要原子性）
type writer struct {
	lumberjack *lumberjack.Logger
	format     Format
	minLevel   log.Level
	mu         sync.Mutex
}

// New 创建文件轮转日志 Writer
//
// filename 是日志文件路径（自动创建父目录）
// 返回的 writer 实现 log.Writer，可直接传给 log.NewLogger
func New(filename string, opts ...Option) (log.Writer, error) {
	if filename == "" {
		return nil, fmt.Errorf("file_rotate: filename is required")
	}

	// 创建父目录
	if dir := filepath.Dir(filename); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("file_rotate: mkdir %q: %w", dir, err)
		}
	}

	w := &writer{
		lumberjack: &lumberjack.Logger{
			Filename:   filename,
			MaxSize:    DefaultMaxSize,
			MaxBackups: DefaultMaxBackups,
			MaxAge:     DefaultMaxAge,
		},
		format:   FormatJSON,
		minLevel: log.LevelDebug,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(w)
		}
	}
	return w, nil
}

// Log 写入一条日志
//
// 实现策略：
//   - 低于 minLevel 的日志直接丢弃
//   - FormatJSON：marshal 成单行 JSON（包含 time/level/msg + 用户 fields）
//   - FormatText：tab 分隔 key=value
//   - 用 mu 保护 marshal+write 原子性（避免多 goroutine 交错输出）
func (w *writer) Log(_ context.Context, level log.Level, msg string, fields ...log.Field) {
	if level < w.minLevel {
		return
	}

	var line string
	switch w.format {
	case FormatJSON:
		line = w.formatJSON(level, msg, fields)
	case FormatText:
		line = w.formatText(level, msg, fields)
	default:
		line = w.formatJSON(level, msg, fields)
	}

	// lumberjack.Write 内部并发安全，但多 goroutine 的两条 Write 可能交错
	// 用 mu 保护一次完整 line 的写入
	w.mu.Lock()
	_, _ = w.lumberjack.Write([]byte(line))
	w.mu.Unlock()
}

// formatJSON 输出 JSON 格式单行日志
//
// 字段顺序：time → level → msg → 用户 fields
func (w *writer) formatJSON(level log.Level, msg string, fields []log.Field) string {
	m := make(map[string]any, len(fields)+3)
	m["time"] = time.Now().UTC().Format(time.RFC3339Nano)
	m["level"] = levelString(level)
	m["msg"] = msg
	for _, f := range fields {
		m[f.Key] = f.Value
	}

	b, err := json.Marshal(m)
	if err != nil {
		// marshal 失败兜底：输出 error + 原始 msg
		return fmt.Sprintf(`{"time":"%s","level":"ERROR","msg":"file_rotate: json marshal failed: %v","orig_msg":%q}`+"\n",
			time.Now().UTC().Format(time.RFC3339Nano), err, msg)
	}
	return string(b) + "\n"
}

// formatText 输出 TEXT 格式单行日志
//
// 格式：TIME\tLEVEL\tMSG\tkey1=val1\tkey2=val2
func (w *writer) formatText(level log.Level, msg string, fields []log.Field) string {
	var b []byte
	b = append(b, time.Now().Format(time.RFC3339)...)
	b = append(b, '\t')
	b = append(b, levelString(level)...)
	b = append(b, '\t')
	b = append(b, msg...)
	for _, f := range fields {
		b = append(b, '\t')
		b = append(b, f.Key...)
		b = append(b, '=')
		b = append(b, fmt.Sprint(f.Value)...)
	}
	b = append(b, '\n')
	return string(b)
}

// levelString 级别字符串（与 slog 对齐）
func levelString(l log.Level) string {
	switch l {
	case log.LevelDebug:
		return "DEBUG"
	case log.LevelInfo:
		return "INFO"
	case log.LevelWarn:
		return "WARN"
	case log.LevelError:
		return "ERROR"
	case log.LevelFatal:
		return "FATAL"
	default:
		return "INFO"
	}
}

// Close flush + 关闭 lumberjack（触发文件关闭）
// 重复调用安全
func (w *writer) Close() error {
	if w.lumberjack == nil {
		return nil
	}
	return w.lumberjack.Close()
}
