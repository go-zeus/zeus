---
title: Zeus
weight: 1
description: 零依赖、可插拔的 Go 微服务框架
---

零依赖、可插拔的 Go 微服务框架。现代构造器注入模式 + 内置默认装配 + 4 层渐进暴露 API。

## 设计哲学

> **内部复杂（灵活）+ 外部简单（默认）。**
>
> 用户 5 行代码启动应用，需要时能挖到底层实现细节。

## 5 行启动

```go
package main

import (
    "net/http"

    "github.com/go-zeus/zeus/app"
)

func main() {
    app.Run(&app.Config{Port: 8080}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("hello from zeus"))
    }))
}
```

零配置自动启用 slog logger / requestid+accesslog+recovery 中间件 / memory 注册中心 / 优雅关闭。

## 4 层渐进暴露

| 层 | 用户 | 入口 |
|---|---|---|
| **L1** | 学习者 / 单进程 demo | `app.Run(cfg, handler)` |
| **L2** | 个人开发者 / 配置驱动 | `app.Run(cfgWithRegistry, handler)` URL scheme |
| **L3** | 小团队 / 代码定制 | `app.NewApp(opts ...AppOption)` |
| **L4** | 定制需求 / 完全控制 | `components.NewApp(comps ...any)` |

详见 [4 层 API](getting-started/layered-api)。

## 下一步

- [安装](getting-started/installation)
- [快速开始](getting-started/quickstart)
- [设计哲学](architecture/design-philosophy)
- [功能域指南](guide/_index)
