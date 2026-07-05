# 07-autoapp-multi · L4 多 Server 单 App

同一进程内启动两个 HTTP server（监听不同端口），框架为每个 server 生成一个 Instance 注册到注册中心（带 `protocol` 字段）。

## 演示场景

- 业务 API server（`:9001`）
- 管理 server（`:9002`）

真实场景：把第二个 server 换成 `grpc.NewGRPC(grpc.Port(9002))` 即可实现 **HTTP + gRPC 同进程多协议注册**，无需改其他代码。

## 启动与测试

```bash
cd examples/07-autoapp-multi
go run .
```

```bash
curl http://localhost:9001/hello   # hello from api
curl http://localhost:9002/admin   # admin endpoint
```

注册中心会出现两条 Instance（同名，不同 protocol/port）。

## 核心代码

```go
apiServer := httpdriver.NewHTTP(httpdriver.Mux(apiMux), httpdriver.Port(9001))
adminServer := httpdriver.NewHTTP(httpdriver.Mux(adminMux), httpdriver.Port(9002))

app := components.NewApp(
    components.NewLogComponent(logslog.NewSlog()),
    components.NewRegistryComponent(memory.New()),
    components.NewServerComponent(apiServer, adminServer),   // 多 server 一次传入
    components.NewServiceComponent(),
)
app.Run()
```

## 衔接

- L4 单 server → [06-autoapp-full](../06-autoapp-full/)
- L3 多 server（类型装配）→ [03-typed](../03-typed/)
