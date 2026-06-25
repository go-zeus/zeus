// Package page 提供通用分页与排序辅助。
//
// 设计目标：
//   - 零依赖（仅用标准库）
//   - 与数据库/缓存/MQ 等数据源解耦
//   - 支持泛型 Result[T]
//   - 处理边界：page < 1 自动校正为 1，size 超出 [1, maxSize] 自动校正
//
// 用例：
//   - HTTP API 分页查询（?page=1&size=20）
//   - 数据库 LIMIT/OFFSET 包装
//   - 任意 List 的分片访问
//
// 使用示例：
//
//	req := page.Request{Page: 1, Size: 20, Sort: "created_at:desc"}
//	items := []User{...}  // 全量数据
//	resp := page.Paginate(req, items)
//	// resp.Items = 用户列表
//	// resp.Total = 全量长度
//	// resp.TotalPages = 计算出的总页数
package page

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// —— 常量 ——

const (
	defaultPage    = 1
	defaultSize    = 20
	defaultMaxSize = 100
)

// —— Request ——

// Request 分页请求
//
// 字段：
//   - Page：页码（1-based），< 1 自动校正为 1
//   - Size：每页大小，< 1 自动校正为默认值，> MaxSize 自动校正为 MaxSize
//   - MaxSize：每页最大限制（防止恶意大请求），默认 100
//   - Sort：排序表达式，格式 "field1:asc,field2:desc"
//   - Query：可选的查询字符串（业务自定义解析）
type Request struct {
	Page    int
	Size    int
	MaxSize int
	Sort    string
	Query   string
}

// Normalize 规范化分页参数（返回新的 Request，不修改原值）
//
// 链式调用：r = r.Normalize()
func (r Request) Normalize() Request {
	if r.Page < 1 {
		r.Page = defaultPage
	}
	if r.MaxSize <= 0 {
		r.MaxSize = defaultMaxSize
	}
	if r.Size < 1 {
		r.Size = defaultSize
	}
	if r.Size > r.MaxSize {
		r.Size = r.MaxSize
	}
	return r
}

// Offset 计算 SQL OFFSET（0-based）
//
// 用法：db.Offset(req.Offset()).Limit(req.Size).Find(&items)
func (r *Request) Offset() int {
	return (r.Page - 1) * r.Size
}

// Limit 返回每页大小（与 Size 同义，命名对齐 SQL）
func (r *Request) Limit() int {
	return r.Size
}

// —— Sort ——

// SortField 单个排序字段
type SortField struct {
	Name string
	Desc bool
}

// ParseSort 解析排序表达式
//
// 格式："field1:asc,field2:desc" 或 "field1,field2:desc"（默认 asc）
// 容错：忽略空字段名、忽略非法 token
//
// 示例：
//
//	fields := page.ParseSort("created_at:desc,id:asc")
//	// → [{created_at true}, {id false}]
func ParseSort(expr string) []SortField {
	if expr == "" {
		return nil
	}
	parts := strings.Split(expr, ",")
	fields := make([]SortField, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, ":", 2)
		name := strings.TrimSpace(kv[0])
		if name == "" {
			continue
		}
		desc := false
		if len(kv) == 2 {
			dir := strings.ToLower(strings.TrimSpace(kv[1]))
			if dir == "desc" {
				desc = true
			}
		}
		fields = append(fields, SortField{Name: name, Desc: desc})
	}
	return fields
}

// String 反向序列化（用于日志/cache key）
func (f SortField) String() string {
	dir := "asc"
	if f.Desc {
		dir = "desc"
	}
	return f.Name + ":" + dir
}

// SortString 排序字段列表 → 字符串
func SortString(fields []SortField) string {
	parts := make([]string, len(fields))
	for i, f := range fields {
		parts[i] = f.String()
	}
	return strings.Join(parts, ",")
}

// —— Response ——

// Response 分页响应
//
// 字段：
//   - Items：当前页数据（泛型）
//   - Total：全量数据总数
//   - Page/Size：回显请求参数（已规范化）
//   - TotalPages：总页数
//   - HasPrev/HasNext：是否有上/下页
type Response[T any] struct {
	Items      []T
	Total      int64
	Page       int
	Size       int
	TotalPages int
	HasPrev    bool
	HasNext    bool
}

// Paginate 对切片进行分页
//
// 行为：
//   - 自动规范化 Request
//   - 切片越界时返回空 Items（不 panic）
//   - Total = len(items)
//   - TotalPages = ceil(Total / Size)
func Paginate[T any](req Request, items []T) Response[T] {
	req = req.Normalize()
	total := int64(len(items))
	start := req.Offset()
	if start > len(items) {
		start = len(items)
	}
	end := start + req.Size
	if end > len(items) {
		end = len(items)
	}
	var pageItems []T
	if start < end {
		pageItems = items[start:end]
	} else {
		pageItems = []T{}
	}
	return Response[T]{
		Items:      pageItems,
		Total:      total,
		Page:       req.Page,
		Size:       req.Size,
		TotalPages: totalPages(total, req.Size),
		HasPrev:    req.Page > 1,
		HasNext:    int64(req.Offset()+req.Size) < total,
	}
}

// PaginateSorted 对切片分页前先按 fields 排序
//
// less 函数接收两个 SortField 上下文（用于决定按哪个字段排序）
// 用户应自行实现 less 函数，对字段名进行 switch
//
// 用法：
//
//	resp := page.PaginateSorted(req, items, func(a, b User, field string) bool {
//	    switch field {
//	    case "id": return a.ID < b.ID
//	    case "name": return a.Name < b.Name
//	    }
//	    return false
//	})
func PaginateSorted[T any](req Request, items []T, less func(a, b T, field string) bool) Response[T] {
	fields := ParseSort(req.Sort)
	if len(fields) > 0 {
		SortBy(items, fields, less)
	}
	return Paginate(req, items)
}

// SortBy 对切片按多个字段排序（稳定）
//
// less 函数：返回 true 表示 a < b（升序）
// 多个字段时按顺序优先级（最后一个字段优先级最低）
func SortBy[T any](items []T, fields []SortField, less func(a, b T, field string) bool) {
	if len(fields) == 0 || less == nil {
		return
	}
	// 从最低优先级字段开始排序（稳定排序）
	for i := len(fields) - 1; i >= 0; i-- {
		f := fields[i]
		sort.SliceStable(items, func(a, b int) bool {
			lessAB := less(items[a], items[b], f.Name)
			lessBA := less(items[b], items[a], f.Name)
			// 处理相等（lessAB 和 lessBA 都 false 时）
			if !lessAB && !lessBA {
				return false
			}
			if f.Desc {
				return lessBA
			}
			return lessAB
		})
	}
}

// —— Cursor / Keyset 分页 ——

// CursorRequest 基于 cursor（keyset）的分页请求
//
// 适用：大数据集、深翻页（OFFSET 性能差）
// 用法：cursor 通常是上一页最后一项的 ID
type CursorRequest struct {
	Size    int
	MaxSize int
	Cursor  string // 上一页最后一项的 ID（首页传空）
}

// Normalize 规范化 cursor 分页参数（返回新的 CursorRequest，不修改原值）
func (r CursorRequest) Normalize() CursorRequest {
	if r.MaxSize <= 0 {
		r.MaxSize = defaultMaxSize
	}
	if r.Size < 1 {
		r.Size = defaultSize
	}
	if r.Size > r.MaxSize {
		r.Size = r.MaxSize
	}
	return r
}

// CursorResponse cursor 分页响应
type CursorResponse[T any] struct {
	Items      []T
	NextCursor string // 下一页 cursor（空表示无更多数据）
	HasMore    bool
}

// —— 错误 ——

// ErrInvalidPage 非法分页参数（在 Validate 严格模式下返回）
var ErrInvalidPage = errors.New("page: invalid pagination parameters")

// Validate 严格校验分页参数（不自动校正），失败返回 ErrInvalidPage。
//
// 典型场景：HTTP API 入口显式拒绝非法参数（而非静默校正），让客户端
// 收到明确的错误而不是被悄悄改值。
func (r *Request) Validate() error {
	if r.Page < 0 {
		return fmt.Errorf("%w: page < 0", ErrInvalidPage)
	}
	if r.Size < 0 {
		return fmt.Errorf("%w: size < 0", ErrInvalidPage)
	}
	if r.MaxSize > 0 && r.Size > r.MaxSize {
		return fmt.Errorf("%w: size > MaxSize", ErrInvalidPage)
	}
	return nil
}

// —— 内部辅助 ——

func totalPages(total int64, size int) int {
	if size <= 0 {
		return 0
	}
	return int((total + int64(size) - 1) / int64(size))
}
