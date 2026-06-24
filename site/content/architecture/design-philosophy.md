---
title: 设计哲学
weight: 10
---

> **内部复杂（灵活）+ 外部简单（默认）。**
>
> 用户 5 行代码启动应用，需要时能挖到底层实现细节。

所有 PR、API 设计、文档组织都必须服从这条哲学。新增功能前先回答："能不能在用户不需要时看不到？"

## 验收指标（可量化）

| 指标 | 目标 | 业界标杆 |
|---|---|---|
| Hello world 代码行数 | ≤ 5 行 | Gin: 7 行 / FastAPI: 5 行 |
| L1 用户需记忆的概念数 | ≤ 2 个（App + Config） | Redis: 5 / Gin: 3 |
| 默认装配数量 | ≥ 8 个组件 | Django: batteries included |
| 从 demo 到生产改动 | 仅改配置 | go-zero: etc/ |
| L1 → L4 切换成本 | 渐进，无需重写 | Spring Boot 分层 |

## 设计参考

不是抄作业，而是借鉴 + 不抄的部分：

| 项目 | 借鉴点 | 不抄 |
|---|---|---|
| Redis | 5 个核心数据结构撑起整个生态 | 不限制接口数量 |
| Gin | 1 个路由 + 1 个中间件链 = 80% 业务 | 不做 router-centric |
| FastAPI | 类型即文档、装饰器即路由 | 不引入 Python 风格 |
| Django | "batteries included" 默认齐全 | 不做 admin 后台 |
| go-zero | 代码生成消灭样板 | 不做 .api DSL |
| gRPC | 一个 .proto → 自动生成 | 不引入 IDL |

## 概念分层

为避免业务 / 运行时 / 注册中心概念混淆，明确各层职责：

| 层 | 概念 | 归属 | 说明 |
|---|---|---|---|
| 业务层 | `UserService` / `OrderService` | 用户自治 | 框架不感知业务 service |
| 运行时层 | `App` | 框架 | 进程级，管信号 + 优雅关闭 |
| 运行时层 | `Server` | 框架 | Endpoint = ip:port，对应一种协议 |
| 注册中心层 | `ServiceEntry` | 框架 | 同名 Instance 的逻辑集合 |
| 注册中心层 | `Instance` | 框架 | 一个 Server 一个 Instance |
| 注册中心层 | `Cluster` | 框架 | 实例按 Cluster 字段聚合 |

**核心规则**：

- 一个 `App` 可持有多个 `Server`（HTTP + gRPC 同进程）
- 每个 `Server` 对应一个注册中心 `Instance`（共享 Name，区分 Protocol）
- 注册/反注册最小单位是 `Instance`（不是业务 service）
- 业务 service 挂到 Server 的 Handler 即可，不参与注册

## 可见性规则

呼应"4 层渐进暴露"，概念分可见性：

- `App` / `Server` / `Instance` / `ServiceEntry` / `Cluster` 属于 **L3 及以上** 可见
- L1/L2 用户不应感知 `Instance` / `ServiceEntry` / `Cluster`，仅看到 `Service` 这个聚合词
- 文档与 API 命名时，"用户感知概念"和"内部实现概念"必须分层标注

## 默认装配清单

未配置时自动启用，用户零感知：

| 默认项 | 内置实现 |
|---|---|
| Server 协议选择 | 按 handler 类型推断 |
| 注册中心 | `registry/memory` |
| 日志 | `log/slog` |
| 中间件 | recovery + requestID + 请求日志 |
| 健康检查 | `/health` `/health/ready` `/health/live` |
| Metrics | `/metrics` |
| 信号处理 | SIGTERM/SIGINT/SIGQUIT → 优雅关闭（10s 超时） |
| 服务名 | `zeus-service`（用户可覆盖） |

**关键规则**：默认装配**不允许失败**。
