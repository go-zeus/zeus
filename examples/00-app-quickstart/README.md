# 00-app-quickstart · L4 手动装配（app.New + WithServer）

Zeus 最底层的手动装配入口。新用户推荐从 [01-hello](../01-hello/)（L1）开始；本示例演示需要完全控制 server 装配的场景。

## 与各层的关系

| 层 | 入口 | 心智 |
|---|---|---|
| L1 | `app.Run(cfg, handler)` | 5 行启动 |
| L4 | `app.New(app.WithServer(s))` ← 本示例 | 完全控制 server，但不引入 Component 概念 |

L4 的 `components.NewApp(...)`（见 [05-autoapp](../05-autoapp/)）比本示例更高一层（引入 Component 装配），本示例是最薄的 server 装配。

## 启动与测试

```bash
cd examples/00-app-quickstart
go run .
```

```bash
curl http://localhost:8080
# hello from zeus L4
```

## 核心代码

```go
srv := zeushttp.NewHTTP(
    zeushttp.Port(8080),
    zeushttp.Mux(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _, _ = w.Write([]byte("hello from zeus L4"))
    })),
)
a := app.New(app.WithServer(srv))
a.Run(make(chan struct{}))
```

## 衔接

- L1 5 行启动 → [01-hello](../01-hello/)
- L4 完整组件装配 → [05-autoapp](../05-autoapp/)
