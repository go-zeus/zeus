// Package async 提供轻量级 Future / goroutine 抽象。
//
// 设计目的：
//   - 用泛型 Future[T] 包装 go func() 返回值，提供 Await / AwaitCtx / Cancel 能力
//   - 自动 recover panic，转为 error 返回（不污染调用栈）
//   - ExecCtx 接受 context.Context，ctx 取消时通知后台 goroutine 退出（避免泄漏）
//
// 与 errgroup 的差异：
//   - errgroup：多 goroutine 协作 + 第一个 error 取消整组（用 Wait）
//   - async：单 goroutine 异步包装 + 单值返回（用 Await）
//
// 使用建议：
//   - 长时操作（IO / 计算）→ 用 ExecCtx 传 ctx，让上层取消能传播
//   - 短时操作（<100ms）→ 用 Exec 即可，ctx 路径反而增加开销
package async

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
)

// Future 具有await的方法
type Future[T any] interface {
	// Await 阻塞等待结果（不带 ctx，等价于 AwaitCtx(context.Background())）
	Await() (T, error)
	// AwaitCtx 阻塞等待结果，ctx 取消时返回 (zero, ctx.Err())
	//
	// 注意：ctx 取消不会停止后台 goroutine —— Go 无法外部 kill goroutine。
	// 调用方若希望 goroutine 提前退出，应使用 ExecCtx 并让 f() 自行监听 ctx。
	AwaitCtx(ctx context.Context) (T, error)
	// Cancel 主动通知后台 goroutine 退出（仅 ExecCtx 创建的 Future 有效）
	//
	// 行为：取消与该 Future 关联的 ctx；f() 内部应通过 ctx.Done() 感知
	// 重复调用安全；Exec 创建的 Future（无 ctx）是 no-op
	Cancel()
}

type future[T any] struct {
	await  func(ctx context.Context) (T, error)
	cancel context.CancelFunc
}

func (f future[T]) Await() (T, error) {
	return f.await(context.Background())
}

func (f future[T]) AwaitCtx(ctx context.Context) (T, error) {
	return f.await(ctx)
}

func (f future[T]) Cancel() {
	if f.cancel != nil {
		f.cancel()
	}
}

// Exec 执行async函数（后台 goroutine 跑 f，返回 Future 等结果）
//
// 行为：
//   - 立即返回 Future[T]，f 在独立 goroutine 跑
//   - panic 自动 recover 转为 error
//   - Cancel() 是 no-op（无 ctx 可取消）
//
// 注意：若 f 是长时操作，建议改用 ExecCtx —— Exec 路径下 ctx 取消后 f 仍在后台跑直到返回，
// 可能造成 goroutine 泄漏。仅在 f 必然短时（<100ms）或不可中断时使用 Exec。
func Exec[T any](f func() T) Future[T] {
	var (
		result T
		err    error
	)
	c := make(chan struct{})
	go func() {
		defer func() {
			if panicErr := recover(); panicErr != nil {
				err = fmt.Errorf("utils/async: panic recovered: %v\n%s", panicErr, debug.Stack())
			}
			close(c)
		}()
		result = f()
	}()
	return future[T]{
		await: func(ctx context.Context) (T, error) {
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-c:
				return result, err
			}
		},
	}
}

// ExecCtx 执行 async 函数，并把可取消 ctx 传给 f
//
// 行为：
//   - 派生 ctx：AwaitCtx 调用方的 ctx 与 Cancel() 都可取消该 ctx
//   - f 应通过 <-ctx.Done() 主动感知取消，及时返回
//   - panic 自动 recover 转为 error
//
// 与 Exec 的关键差异：
//   - Exec：ctx 取消仅 AwaitCtx 立即返回，f 仍跑完（泄漏风险）
//   - ExecCtx：ctx 取消同时通知 f，f 可主动退出（无泄漏）
//
// 推荐用法：
//
//	fut := async.ExecCtx(ctx, func(ctx context.Context) (Result, error) {
//	    return doSlowIO(ctx)  // 内部应监听 ctx.Done
//	})
//	result, err := fut.Await()  // 或 fut.AwaitCtx(timeoutCtx) + fut.Cancel()
func ExecCtx[T any](ctx context.Context, f func(ctx context.Context) (T, error)) Future[T] {
	// 派生可被 Cancel() + 父 ctx 双向取消的 ctx
	innerCtx, cancel := context.WithCancel(ctx)

	var (
		result T
		err    error
	)
	c := make(chan struct{})

	go func() {
		defer func() {
			if panicErr := recover(); panicErr != nil {
				err = fmt.Errorf("utils/async: panic recovered: %v\n%s", panicErr, debug.Stack())
			}
			close(c)
		}()
		result, err = f(innerCtx)
	}()

	var once sync.Once
	var zero T // ctx 取消路径用 zero value，避免与 goroutine 写 result 竞争
	return future[T]{
		await: func(awaitCtx context.Context) (T, error) {
			select {
			case <-awaitCtx.Done():
				// 调用方取消了等待，同时通知后台 goroutine 退出（防泄漏）
				// 不读 result：避免与后台 goroutine 写入竞争（race detector 触发点）
				once.Do(cancel)
				return zero, awaitCtx.Err()
			case <-c:
				return result, err
			}
		},
		cancel: func() {
			once.Do(cancel)
		},
	}
}
