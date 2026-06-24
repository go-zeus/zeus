// Package batch 提供错误定义
package batch

import "errors"

// ErrClosed 批处理器已关闭
var ErrClosed = errors.New("batch: closed")
