# 02-with-registry · L2 配置驱动 + 集群路由

通过 `app.Config` 字段定制服务名、集群、端口，演示灰度发布场景下双实例的注册与路由。

## 演示场景

启动两个实例（不同 cluster），观察：
- 两个实例都注册到 memory registry（同名 `demo-svc`，不同 cluster）
- client 带 `X-Zeus-Cluster` Header 时路由到对应 cluster 实例

## 启动与测试

两个终端分别启动：

```bash
# 终端 1：default 集群实例
cd examples/02-with-registry
CLUSTER=default PORT=9001 go run .

# 终端 2：canary 集群实例
cd examples/02-with-registry
CLUSTER=canary PORT=9002 go run .
```

```bash
curl http://localhost:9001/ping                              # default 链路
curl http://localhost:9002/ping                              # canary 链路
curl -H "X-Zeus-Cluster: canary" http://localhost:9001/ping # Header 路由到 canary 实例
```

## 核心代码

```go
cfg := &app.Config{
    Name:    "demo-svc",
    Port:    envInt("PORT", 9001),
    Cluster: envStr("CLUSTER", "default"),   // 通过环境变量切 cluster
}
app.Run(cfg, handler)
```

## 衔接

- L1 零配置 → [01-hello](../01-hello/)
- L2 URL scheme 切 cache/mq → [04-config-driven](../04-config-driven/)
- 集群路由端到端（gateway + 多 srv） → [12-cluster-routing](../12-cluster-routing/)
