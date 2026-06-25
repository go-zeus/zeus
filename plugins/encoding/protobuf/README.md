# protobuf 编解码

`encoding.Codec` 的 [Protocol Buffers](https://protobuf.dev/) 实现。把 `proto.Message` 序列化为字节串，用于 gRPC 请求体、etcd value、消息总线 payload 等场景。仅接受实现了 `proto.Message` 的类型，其他类型返回明确错误。

## 安装

主仓零依赖，使用 plugins 需要在 go.mod 中添加：

```bash
go get github.com/go-zeus/zeus/plugins/encoding/protobuf
```

> 插件是独立 module，不会污染主仓依赖。

## 使用

```go
import (
    "github.com/go-zeus/zeus/encoding"
    "github.com/go-zeus/zeus/encoding/json"
    pbimpl "github.com/go-zeus/zeus/plugins/encoding/protobuf"

    pb "your.module/gen/user/v1" // protoc 生成的代码
)

codec := pbimpl.New()

// 序列化
payload, err := codec.Marshal(&pb.User{
    Id:   "u-1",
    Name: "alice",
})

// 反序列化
var u pb.User
if err := codec.Unmarshal(payload, &u); err != nil {
    panic(err)
}
```

注册到全局 codec 表，配合 RPC / MQ 框架按名选择：

```go
encoding.RegisterCodec(codec)        // Name()="protobuf"
encoding.RegisterCodec(json.New())   // Name()="json"

c := encoding.GetCodec("protobuf")   // 按名取出
```

## 选项

本插件无配置选项，`New()` 即装即用。

| 行为 | 说明 |
|------|------|
| `Marshal(v)` | `v` 必须实现 `proto.Message`，否则返回 `expected proto.Message` 错误 |
| `Unmarshal(data, v)` | `v` 必须是 `proto.Message` 的可写指针；空 `data` 视为 no-op（对齐 `json.Unmarshal`） |
| `Name()` | 返回字符串 `"protobuf"`，用于 codec 注册表查找 |

## 依赖

- `google.golang.org/protobuf/proto`（新版 API，反射与序列化）

## 集成

- 与 `encoding.RegisterCodec` 配合：按 `"protobuf"` 名注册到全局表，RPC / MQ 框架可按 Content-Type 或协议字段选择
- 与 gRPC 配合：gRPC 默认使用 protobuf，本插件提供纯 codec 视图，便于跨组件复用同一序列化逻辑
- 与 mq/database 配合：消息或字段为结构化数据时，protobuf 体积比 JSON 小 30%–50%，适合带宽敏感场景
- 主仓内置 `encoding/json` 作为零依赖默认 codec；protobuf 编解码放在 plugins 下，按需引入
- 示例参考仓库 `examples/encoding/`
