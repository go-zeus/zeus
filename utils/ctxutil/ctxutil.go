// Package ctxutil 提供标准库 context 缺失的常用操作。
//
// 设计目的：
//   - Merge：合并多个 ctx（任一取消则整体取消）
//   - WithTimeoutFromCtx：从父 ctx 复制 deadline 到新 ctx
//   - IsCanceled / IsDeadline：判定取消原因（避免手写 errors.Is(ctx.Err(), context.Canceled)）
//
// 与标准库的关系：
//   - 标准库 context 提供 WithCancel / WithTimeout / WithDeadline
//   - ctxutil 提供上述复合操作，全部基于标准库实现
//
// 注意：Merge 不是 magic —— Go 无法外部 kill goroutine。Merge 通过启动一个
// goroutine 监听多个 ctx.Done()，任一触发即取消派生 ctx。
package ctxutil

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Merge 合并多个 ctx，返回派生 ctx + cancel。
//
// 行为：
//   - 任一父 ctx 取消 → 派生 ctx 立即取消（继承父 ctx 的 cause 与错误）
//   - 全部父 ctx 都未取消 → 派生 ctx 保持活跃
//   - 调用 cancel() → 派生 ctx 立即取消（即使所有父 ctx 仍活跃）
//
// 用途：当需要等待多个独立 ctx 中的任何一个完成时（如客户端取消 + 服务端超时）
//
// 性能：会启动一个监听 goroutine（N 个父 ctx 时启动 1 + N 个 goroutine）
// 当派生 ctx 取消后，监听 goroutine 自动退出
//
// 示例：
//
//	ctx := ctxutil.Merge(clientCtx, serverTimeoutCtx)
//	defer cancel()
//	select {
//	case <-workDone:
//	case <-ctx.Done():
//	    return ctx.Err()
//	}
func Merge(parents ...context.Context) (context.Context, context.CancelFunc) {
	// 找到最近 deadline，作为派生 ctx 的 base（让 Deadline() 也能传播）
	var base context.Context = context.Background()
	earliest := time.Time{}
	for _, p := range parents {
		if p == nil {
			continue // nil ctx 不能调 Deadline()，会 panic
		}
		if dl, ok := p.Deadline(); ok {
			if earliest.IsZero() || dl.Before(earliest) {
				earliest = dl
				base = p
			}
		}
	}

	merged, cancelCause := context.WithCancelCause(base)
	if len(parents) == 0 {
		return merged, func() { cancelCause(context.Canceled) }
	}

	var once sync.Once

	// 启动监听 goroutine：任一父 ctx 取消 → 取消 merged
	for _, p := range parents {
		if p == nil {
			continue
		}
		go func(parent context.Context) {
			select {
			case <-parent.Done():
				// 继承父 ctx 的 cause（若父 ctx 用 WithCancelCause 创建）
				once.Do(func() {
					cancelCause(context.Cause(parent))
				})
			case <-merged.Done():
				// merged 已被外部 cancel 或其他 goroutine 触发，正常退出
			}
		}(p)
	}

	// 包装 cancel 为 context.CancelFunc（不带 cause 参数）
	wrappedCancel := context.CancelFunc(func() {
		once.Do(func() { cancelCause(context.Canceled) })
	})

	return merged, wrappedCancel
}

// WithTimeoutFromCtx 从 src 继承 deadline（若存在），创建带相同 deadline 的子 ctx。
//
// 行为：
//   - src 无 deadline → 等价于 context.WithCancel(dst)
//   - src 有 deadline → 用 src.Deadline() - time.Now() 作为 dst 的 timeout
//
// 用途：跨服务调用时传递超时（client 超时剩余时间传给下游 server 子调用）
//
// 示例：
//
//	// clientCtx 剩余 2s 超时，让 dbCtx 也只跑 2s
//	dbCtx, cancel := ctxutil.WithTimeoutFromCtx(context.Background(), clientCtx)
//	defer cancel()
func WithTimeoutFromCtx(dst, src context.Context) (context.Context, context.CancelFunc) {
	dl, ok := src.Deadline()
	if !ok {
		// src 无 deadline：返回普通可取消 ctx
		return context.WithCancel(dst)
	}
	timeout := time.Until(dl)
	if timeout <= 0 {
		// src 已超时：立即取消
		ctx, cancel := context.WithCancel(dst)
		cancel()
		return ctx, cancel
	}
	return context.WithTimeout(dst, timeout)
}

// IsCanceled 判断 err 是否为 context.Canceled（主动取消）
func IsCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}

// IsDeadline 判断 err 是否为 context.DeadlineExceeded（超时）
func IsDeadline(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}

// IsCanceledCtx 判断 ctx 是否因主动取消而 Done
//
// 与 IsCanceled(ctx.Err()) 等价，但更易读
func IsCanceledCtx(ctx context.Context) bool {
	return ctx != nil && ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled)
}

// IsDeadlineExceededCtx 判断 ctx 是否因超时而 Done
func IsDeadlineExceededCtx(ctx context.Context) bool {
	return ctx != nil && ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)
}

// DoneOrBlock 阻塞等待 ctx.Done() 或 fn 返回。
//
// 用途：让长时操作能在 ctx 取消时提前返回
//
// 行为：
//   - 若 ctx 已 Done，立即返回 ctx.Err()
//   - 否则阻塞，直到 ctx.Done() 或 fn() 返回（fn 在独立 goroutine 跑）
//
// 注意：fn 在 goroutine 内跑，无法被外部 kill；fn 应主动监听 ctx 并提前返回
func DoneOrBlock[T any](ctx context.Context, fn func() T) (T, error) {
	var zero T
	if ctx.Err() != nil {
		return zero, ctx.Err()
	}

	resultCh := make(chan T, 1)
	go func() {
		resultCh <- fn()
	}()

	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case r := <-resultCh:
		return r, nil
	}
}
