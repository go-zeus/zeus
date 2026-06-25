// Package protobuf 提供基于 google.golang.org/protobuf 的 Codec 实现。
//
// 仅接受实现了 proto.Message 的类型；其他类型返回明确错误。
// 用于 gRPC 序列化、etcd value 编码、消息总线等场景。
//
// 用法：
//
//	codec := protobuf.New()
//	bytes, _ := codec.Marshal(msg)
//	_ = codec.Unmarshal(bytes, &msg)
package protobuf

import (
	"fmt"

	"github.com/go-zeus/zeus/encoding"
	"google.golang.org/protobuf/proto"
)

// 编译期检查 codec 实现 encoding.Codec
var _ encoding.Codec = (*codec)(nil)

type codec struct{}

// New 创建 Protobuf Codec
func New() encoding.Codec { return &codec{} }

// Marshal 把 proto.Message 序列化为字节串。
// v 必须实现 proto.Message（即生成的 pb 消息类型），否则返回明确错误。
func (c *codec) Marshal(v any) ([]byte, error) {
	m, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("protobuf: Marshal: expected proto.Message, got %T", v)
	}
	return proto.Marshal(m)
}

// Unmarshal 把字节串反序列化到 proto.Message。
// v 必须是 proto.Message 的可写指针（如 &MyMessage{}）。
func (c *codec) Unmarshal(data []byte, v any) error {
	m, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("protobuf: Unmarshal: expected proto.Message, got %T", v)
	}
	if len(data) == 0 {
		return nil // 空输入视为 no-op，对齐 json.Unmarshal 行为
	}
	return proto.Unmarshal(data, m)
}

// Name 返回 codec 标识符
func (c *codec) Name() string { return "protobuf" }
