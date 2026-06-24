# 从零构建（scratch）— 无需任何基础镜像
#
# 用法：
#   1. 本地交叉编译 Linux 二进制：GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/server ./cmd/${SVC}
#   2. 准备 ca-certificates.crt（同目录）
#   3. docker build -f docker/scratch.Dockerfile --build-arg SVC=srv1 -t zeus-srv1 build/
#
# SVC 参数仅用于 ENTRYPOINT 路径（已固定为 /app/server）

FROM scratch

# CA 证书（HTTP 客户端需要，调 gateway HTTPS 时使用）
COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# 静态 Go 二进制（CGO_ENABLED=0 编译，无 libc 依赖）
COPY server /app/server

# 暴露端口（仅文档，实际监听由 PORT 环境变量控制）
EXPOSE 9000

# 时区数据（可选；不需要则可去掉）
# COPY zoneinfo /usr/share/zoneinfo

ENTRYPOINT ["/app/server"]
