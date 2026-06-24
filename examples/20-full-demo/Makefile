.PHONY: help build run stop logs k8s-images k8s-apply k8s-delete clean

# 默认目标：显示帮助
help:
	@echo "Zeus full-demo 命令清单"
	@echo ""
	@echo "本地开发："
	@echo "  make build         编译所有服务到 bin/"
	@echo "  make run           本地启动（gateway + api1 + srv1/2/3 多集群 + frontend 后台进程）"
	@echo "  make stop          停止后台进程"
	@echo "  make logs          查看后台日志"
	@echo ""
	@echo "Docker Compose："
	@echo "  make up            构建并启动所有容器"
	@echo "  make down          停止并清理容器"
	@echo "  make logs-dc       跟踪 docker-compose 日志"
	@echo ""
	@echo "K8s："
	@echo "  make k8s-images    构建所有镜像并 load 到 minikube/kind"
	@echo "  make k8s-apply     应用所有 manifests"
	@echo "  make k8s-delete    删除整个 zeus-demo namespace"
	@echo ""
	@echo "测试与清理："
	@echo "  make test          curl 验证 4 条集群路由"
	@echo "  make clean         清理 bin/ 和临时文件"

# === 路径变量 ===
ROOT := ../..
BIN  := bin
DOCKERFILE := docker/service.Dockerfile
COMPOSE := deploy/docker-compose.yml

# === 本地编译 ===
SERVICES := gateway api1 srv1 srv2 srv3

# 集群列表（部署矩阵：不是所有集群都部署所有服务）
CLUSTERS := default user.v1.1 order.v2 batch.v3

build:
	@mkdir -p $(BIN)
	@for svc in $(SERVICES); do \
		echo "→ building $$svc..."; \
		cd $(ROOT) && go build -o examples/20-full-demo/$(BIN)/$$svc ./examples/20-full-demo/cmd/$$svc; \
	done

# === 本地运行（直接启动二进制，无容器）===
PID_DIR := .pids
LOG_DIR := logs

# 本地端口分配：
#   api1:  9000(default) / 9100(其他集群)
#   srv1:  9001(default) / 9101(其他集群)
#   srv2:  9002(default) / 9102(其他集群)
#   srv3:  9003(default) / 9103(其他集群)
# 部署矩阵决定哪些服务在哪些集群启动：
#   api1:  default, batch.v3
#   srv1:  default, user.v1.1, batch.v3
#   srv2:  default, user.v1.1, order.v2
#   srv3:  default, batch.v3

run: build
	@mkdir -p $(PID_DIR) $(LOG_DIR)
	@echo "→ starting gateway..."
	@PORT=8080 ./$(BIN)/gateway > $(LOG_DIR)/gateway.log 2>&1 & echo $$! > $(PID_DIR)/gateway.pid
	@sleep 1
	@echo "→ starting api1 (default + batch.v3)..."
	@CLUSTER=default PORT=9000 GATEWAY_URL=http://localhost:8080 \
		./$(BIN)/api1 > $(LOG_DIR)/api1-default.log 2>&1 & echo $$! > $(PID_DIR)/api1-default.pid
	@CLUSTER=batch.v3 PORT=9100 GATEWAY_URL=http://localhost:8080 \
		./$(BIN)/api1 > $(LOG_DIR)/api1-batch-v3.log 2>&1 & echo $$! > $(PID_DIR)/api1-batch-v3.pid
	@echo "→ starting srv1 (default + user.v1.1 + batch.v3)..."
	@for cluster in default user.v1.1 batch.v3; do \
		if [ "$$cluster" = "default" ]; then port=9001; else port=9101; fi; \
		CLUSTER=$$cluster PORT=$$port GATEWAY_URL=http://localhost:8080 \
			./$(BIN)/srv1 > $(LOG_DIR)/srv1-$$cluster.log 2>&1 & \
		echo $$! > $(PID_DIR)/srv1-$$cluster.pid; \
	done
	@echo "→ starting srv2 (default + user.v1.1 + order.v2)..."
	@for cluster in default user.v1.1 order.v2; do \
		if [ "$$cluster" = "default" ]; then port=9002; else port=9102; fi; \
		CLUSTER=$$cluster PORT=$$port GATEWAY_URL=http://localhost:8080 \
			./$(BIN)/srv2 > $(LOG_DIR)/srv2-$$cluster.log 2>&1 & \
		echo $$! > $(PID_DIR)/srv2-$$cluster.pid; \
	done
	@echo "→ starting srv3 (default + batch.v3)..."
	@for cluster in default batch.v3; do \
		if [ "$$cluster" = "default" ]; then port=9003; else port=9103; fi; \
		CLUSTER=$$cluster PORT=$$port GATEWAY_URL=http://localhost:8080 \
			./$(BIN)/srv3 > $(LOG_DIR)/srv3-$$cluster.log 2>&1 & \
		echo $$! > $(PID_DIR)/srv3-$$cluster.pid; \
	done
	@sleep 2
	@echo ""
	@echo "✓ 服务已启动（4 集群部署矩阵）："
	@echo "  Gateway:       http://localhost:8080"
	@echo "  实例列表:      http://localhost:8080/api/services"
	@echo "  测试 default:   curl http://localhost:8080/login"
	@echo "  测试 user.v1.1: curl -H 'X-Zeus-Cluster: user.v1.1' http://localhost:8080/login"
	@echo "  测试 order.v2:  curl -H 'X-Zeus-Cluster: order.v2' http://localhost:8080/login"
	@echo "  测试 batch.v3:  curl -H 'X-Zeus-Cluster: batch.v3' http://localhost:8080/login"
	@echo ""
	@echo "前端需另起：cd frontend && python3 -m http.server 8088"

stop:
	@for f in $(PID_DIR)/*.pid; do \
		[ -f "$$f" ] && kill $$(cat $$f) 2>/dev/null && echo "stopped $$(basename $$f .pid)"; \
	done
	@rm -rf $(PID_DIR)

logs:
	@tail -f $(LOG_DIR)/*.log

# === Docker Compose ===
up:
	docker compose -f $(COMPOSE) up --build -d
	@echo ""
	@echo "✓ 容器已启动："
	@echo "  前端:   http://localhost:8088"
	@echo "  Gateway: http://localhost:8080"

down:
	docker compose -f $(COMPOSE) down -v --remove-orphans

logs-dc:
	docker compose -f $(COMPOSE) logs -f

# === K8s（minikube/kind 场景） ===
# 注意：imagePullPolicy=Never，需先 load 到集群节点
k8s-images:
	@for svc in $(SERVICES) frontend; do \
		echo "→ building zeus-$$svc..."; \
		if [ "$$svc" = "frontend" ]; then \
			docker build -t zeus-$$svc -f docker/frontend.Dockerfile $(ROOT); \
		else \
			docker build -t zeus-$$svc -f $(DOCKERFILE) --build-arg SVC=$$svc $(ROOT); \
		fi; \
	done
	@echo ""
	@echo "→ loading images into cluster..."
	@command -v minikube >/dev/null && for svc in $(SERVICES) frontend; do \
		minikube image load zeus-$$svc:latest; \
	done || echo "(skip minikube load: minikube not found)"
	@command -v kind >/dev/null && for svc in $(SERVICES) frontend; do \
		kind load docker-image zeus-$$svc:latest; \
	done || echo "(skip kind load: kind not found)"

k8s-apply:
	kubectl apply -f deploy/k8s/

k8s-delete:
	kubectl delete namespace zeus-demo --ignore-not-found

# === 测试验证 ===
test:
	@echo "=== Test 1: default 链路 ==="
	@curl -s http://localhost:8080/login | jq . 2>/dev/null || curl -s http://localhost:8080/login
	@echo ""
	@echo "=== Test 2: user.v1.1 链路（缺 api1/srv3 → 降级 default） ==="
	@curl -s -H 'X-Zeus-Cluster: user.v1.1' http://localhost:8080/login | jq . 2>/dev/null || \
		curl -s -H 'X-Zeus-Cluster: user.v1.1' http://localhost:8080/login
	@echo ""
	@echo "=== Test 3: order.v2 链路（仅 srv2 部署，其余降级 default） ==="
	@curl -s -H 'X-Zeus-Cluster: order.v2' http://localhost:8080/login | jq . 2>/dev/null || \
		curl -s -H 'X-Zeus-Cluster: order.v2' http://localhost:8080/login
	@echo ""
	@echo "=== Test 4: batch.v3 链路（缺 srv2 → 降级 default） ==="
	@curl -s -H 'X-Zeus-Cluster: batch.v3' http://localhost:8080/login | jq . 2>/dev/null || \
		curl -s -H 'X-Zeus-Cluster: batch.v3' http://localhost:8080/login
	@echo ""
	@echo "=== 服务实例列表 ==="
	@curl -s http://localhost:8080/api/services | jq . 2>/dev/null

clean:
	rm -rf $(BIN) $(PID_DIR) $(LOG_DIR)
