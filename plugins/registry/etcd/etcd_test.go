package etcd

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-zeus/zeus/types"
)

// 测试目标 etcd 地址；可通过 ZEUS_ETCD_ENDPOINT 环境变量覆盖
// 默认与 DefaultEndpoint 一致：127.0.0.1:2379
func testEndpoint() string {
	if v := os.Getenv("ZEUS_ETCD_ENDPOINT"); v != "" {
		return v
	}
	return DefaultEndpoint
}

// skipIfNoEtcd 当 etcd 不可达时跳过测试，避免 CI 因网络环境失败
func skipIfNoEtcd(t *testing.T) {
	t.Helper()
	r := New(WithEndpoints(testEndpoint()), WithDialTimeout(2*time.Second)).(*etcdRegistry)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cli, err := r.getClient()
	if err != nil {
		t.Skipf("etcd 不可达，跳过集成测试: %v", err)
	}
	// 用一次 Status 探活确认 etcd 真的可用（端点通但服务挂的场景）
	st, err := cli.Status(ctx, testEndpoint())
	if err != nil || st == nil {
		t.Skipf("etcd Status 失败，跳过集成测试: %v", err)
	}
	_ = r.Close()
}

// TestNew_DefaultEndpoint 验证不传 Option 时默认 endpoints 为 127.0.0.1:2379
func TestNew_DefaultEndpoint(t *testing.T) {
	r := New().(*etcdRegistry)
	if len(r.endpoints) != 1 || r.endpoints[0] != DefaultEndpoint {
		t.Errorf("默认 endpoints = %v, want [%s]", r.endpoints, DefaultEndpoint)
	}
	if r.ttl != DefaultTTL {
		t.Errorf("默认 ttl = %v, want %v", r.ttl, DefaultTTL)
	}
	if r.prefix != DefaultPrefix {
		t.Errorf("默认 prefix = %q, want %q", r.prefix, DefaultPrefix)
	}
	if !r.ownsClient {
		t.Error("默认 ownsClient 应为 true（自己创建的 client 自己关）")
	}
}

// TestNew_OptionsApply 验证 Option 生效
func TestNew_OptionsApply(t *testing.T) {
	r := New(
		WithEndpoints("10.0.0.1:2379", "10.0.0.2:2379"),
		WithTTL(60*time.Second),
		WithPrefix("/test/zeus/"),
		WithCredentials("admin", "pass"),
		WithDialTimeout(10*time.Second),
	).(*etcdRegistry)
	if len(r.endpoints) != 2 || r.endpoints[0] != "10.0.0.1:2379" {
		t.Errorf("endpoints = %v", r.endpoints)
	}
	if r.ttl != 60*time.Second {
		t.Errorf("ttl = %v", r.ttl)
	}
	if r.prefix != "/test/zeus/" {
		t.Errorf("prefix = %q", r.prefix)
	}
	if r.username != "admin" || r.password != "pass" {
		t.Errorf("credentials = %s/%s", r.username, r.password)
	}
	if r.dialTimeout != 10*time.Second {
		t.Errorf("dialTimeout = %v", r.dialTimeout)
	}
}

// TestNew_TTLTooSmall 验证 TTL < 5s 被拒绝（保持默认）
func TestNew_TTLTooSmall(t *testing.T) {
	r := New(WithTTL(time.Second)).(*etcdRegistry)
	if r.ttl != DefaultTTL {
		t.Errorf("过小 TTL 应被忽略，got %v want %v", r.ttl, DefaultTTL)
	}
}

// TestInstanceKey 验证 key 拼接格式
func TestInstanceKey(t *testing.T) {
	r := New(WithPrefix("/zeus/services/")).(*etcdRegistry)
	ins := &types.Instance{ID: "abc", Name: "user-svc"}
	got := r.instanceKey(ins)
	want := "/zeus/services/user-svc/abc"
	if got != want {
		t.Errorf("instanceKey = %q, want %q", got, want)
	}
}

// TestServicePrefix 验证 service 前缀格式（必须以 / 结尾供 WithPrefix 查询）
func TestServicePrefix(t *testing.T) {
	r := New().(*etcdRegistry)
	got := r.servicePrefix("user-svc")
	if !strings.HasSuffix(got, "/") {
		t.Errorf("servicePrefix 应以 / 结尾: %q", got)
	}
	if !strings.Contains(got, "user-svc") {
		t.Errorf("servicePrefix 应包含 service 名: %q", got)
	}
}

// TestRegister_Validation 验证输入校验
func TestRegister_Validation(t *testing.T) {
	r := New(WithEndpoints(testEndpoint())).(*etcdRegistry)
	defer r.Close()

	if err := r.Register(context.Background(), nil); err == nil {
		t.Error("nil instance 应返回错误")
	}
	if err := r.Register(context.Background(), &types.Instance{}); err == nil {
		t.Error("空 Id/Name 应返回错误")
	}
}

// ====== 集成测试（需要真实 etcd 连通）======

// TestIntegration_RegisterAndGetService 端到端：注册 → 查询 → 反注册
func TestIntegration_RegisterAndGetService(t *testing.T) {
	skipIfNoEtcd(t)

	r := New(WithEndpoints(testEndpoint()), WithTTL(10*time.Second)).(*etcdRegistry)
	defer func() {
		_ = r.Close()
	}()

	ctx := context.Background()
	ins := &types.Instance{
		ID:       "test-integration-1",
		Name:     "zeus-test-svc",
		Cluster:  "default",
		Protocol: "http",
		IP:       "127.0.0.1",
		Port:     8080,
	}
	defer r.Deregister(ctx, ins)

	if err := r.Register(ctx, ins); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// 立即查询应该能查到
	got, err := r.GetService(ctx, ins.Name)
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	found := false
	for _, i := range got.Instances {
		if i.ID == ins.ID {
			found = true
			if i.IP != ins.IP || i.Port != ins.Port {
				t.Errorf("instance 字段不一致: got %+v", i)
			}
		}
	}
	if !found {
		t.Error("注册后未在 GetService 结果中找到该实例")
	}
}

// TestIntegration_Deregister 端到端：反注册后 GetService 应找不到
func TestIntegration_Deregister(t *testing.T) {
	skipIfNoEtcd(t)

	r := New(WithEndpoints(testEndpoint()), WithTTL(10*time.Second)).(*etcdRegistry)
	defer r.Close()

	ctx := context.Background()
	ins := &types.Instance{
		ID:   "test-dereg-1",
		Name: "zeus-test-dereg-svc",
		IP:   "127.0.0.1", Port: 9000,
	}
	if err := r.Register(ctx, ins); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Deregister(ctx, ins); err != nil {
		t.Fatalf("Deregister: %v", err)
	}
	// 反注册后 GetService 应返回 not found 错误（前提是该 service 下无其他实例）
	_, err := r.GetService(ctx, ins.Name)
	if err == nil {
		t.Error("Deregister 后应查不到该实例")
	}
}

// TestIntegration_Deregister_Idempotent 多次 Deregister 不报错
func TestIntegration_Deregister_Idempotent(t *testing.T) {
	skipIfNoEtcd(t)
	r := New(WithEndpoints(testEndpoint())).(*etcdRegistry)
	defer r.Close()

	ins := &types.Instance{ID: "idem-1", Name: "zeus-idem-svc", IP: "127.0.0.1", Port: 9001}
	for i := 0; i < 3; i++ {
		if err := r.Deregister(context.Background(), ins); err != nil {
			t.Errorf("第 %d 次 Deregister 失败: %v", i+1, err)
		}
	}
}

// TestIntegration_GetService_NotFound 查询不存在的 service 应返回错误
func TestIntegration_GetService_NotFound(t *testing.T) {
	skipIfNoEtcd(t)
	r := New(WithEndpoints(testEndpoint())).(*etcdRegistry)
	defer r.Close()

	_, err := r.GetService(context.Background(), "zeus-nonexistent-svc-xyz123")
	if err == nil {
		t.Error("查询不存在的 service 应返回错误")
	}
}

// TestIntegration_Watch 验证 Watch 在注册时能收到事件
func TestIntegration_Watch(t *testing.T) {
	skipIfNoEtcd(t)
	r := New(WithEndpoints(testEndpoint()), WithTTL(10*time.Second)).(*etcdRegistry)
	defer r.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := r.Watch(ctx, "zeus-watch-svc")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// 等收到首次推送
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("未收到 Watch 首次推送")
	}

	// 触发变更
	ins := &types.Instance{ID: "watch-1", Name: "zeus-watch-svc", IP: "127.0.0.1", Port: 9100}
	if err := r.Register(ctx, ins); err != nil {
		t.Fatalf("Register: %v", err)
	}
	defer r.Deregister(ctx, ins)

	select {
	case <-ch:
		// 收到变更事件
	case <-time.After(2 * time.Second):
		t.Fatal("Register 后未收到 Watch 事件")
	}
}

// TestIntegration_Register_MultiInstance_ClusterAggregation 验证多个 Instance 注册后按 cluster 聚合
func TestIntegration_Register_MultiInstance_ClusterAggregation(t *testing.T) {
	skipIfNoEtcd(t)
	r := New(WithEndpoints(testEndpoint()), WithTTL(10*time.Second)).(*etcdRegistry)
	defer r.Close()

	ctx := context.Background()
	name := "zeus-cluster-svc"
	ins1 := &types.Instance{ID: "c-1", Name: name, Cluster: "default", IP: "10.0.0.1", Port: 8080}
	ins2 := &types.Instance{ID: "c-2", Name: name, Cluster: "canary", IP: "10.0.0.2", Port: 8080}
	defer func() {
		r.Deregister(ctx, ins1)
		r.Deregister(ctx, ins2)
	}()

	if err := r.Register(ctx, ins1); err != nil {
		t.Fatalf("Register ins1: %v", err)
	}
	if err := r.Register(ctx, ins2); err != nil {
		t.Fatalf("Register ins2: %v", err)
	}

	got, err := r.GetService(ctx, name)
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if len(got.Instances) < 2 {
		t.Errorf("应至少有 2 个实例，实际 %d", len(got.Instances))
	}
	if _, ok := got.Clusters["default"]; !ok {
		t.Error("missing cluster: default")
	}
	if _, ok := got.Clusters["canary"]; !ok {
		t.Error("missing cluster: canary")
	}
}

// TestIntegration_GetService_EmptyName 空 service 名应返回错误
func TestIntegration_GetService_EmptyName(t *testing.T) {
	skipIfNoEtcd(t)
	r := New(WithEndpoints(testEndpoint())).(*etcdRegistry)
	defer r.Close()

	_, err := r.GetService(context.Background(), "")
	if err == nil {
		t.Error("空 service 名应返回错误")
	}
}
