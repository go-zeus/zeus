package async

import (
	"context"
	"fmt"
	"runtime/debug"
)

// Future 具有await的方法
type Future[T any] interface {
	Await() (T, error)
	AwaitCtx(ctx context.Context) (T, error)
}

type future[T any] struct {
	await func(ctx context.Context) (T, error)
}

func (f future[T]) Await() (T, error) {
	return f.await(context.Background())
}

func (f future[T]) AwaitCtx(ctx context.Context) (T, error) {
	return f.await(ctx)
}

// Exec 执行async函数
func Exec[T any](f func() T) Future[T] {
	var (
		result T
		err    error
	)
	c := make(chan struct{})
	go func() {
		defer func() {
			if panicErr := recover(); panicErr != nil {
				err = fmt.Errorf("Go async panic:%v \n%s", panicErr, debug.Stack())
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
