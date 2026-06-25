---
title: 安装
weight: 10
---

## 环境要求

- Go 1.22+（主仓要求，log/slog 等标准库 API 决定）
- 可选：Go 1.25+（如需使用 etcd/otel 等插件）

## 主仓安装

```bash
go get github.com/go-zeus/zeus
```

主仓零第三方依赖，所有第三方实现都在 `plugins/` 下作为独立 module 维护。

## 插件安装

按需引入，互不影响：

```bash
go get github.com/go-zeus/zeus/plugins/registry/etcd

go get github.com/go-zeus/zeus/plugins/cache/redis

go get github.com/go-zeus/zeus/plugins/mq/kafka

go get github.com/go-zeus/zeus/plugins/metrics/prometheus
```

完整插件清单参见 [插件 BOM](../reference/plugin-bom)。

## 验证安装

```go
package main

import (
    "fmt"
    "github.com/go-zeus/zeus/app"
)

func main() {
    fmt.Println("zeus version:", app.Version)
}
```

## 下一步

- [快速开始](quickstart) — 5 行代码启动
- [4 层 API](layered-api) — 选择合适的入口
