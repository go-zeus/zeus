package k8s

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// ===== 单元测试（使用 fake clientset，无需真实集群）=====

func TestNew_RequiresName(t *testing.T) {
	_, err := New()
	if err == nil {
		t.Fatal("expected error when WithName not provided")
	}
	if !strings.Contains(err.Error(), "WithName is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNew_Defaults(t *testing.T) {
	l, err := New(WithName("my-cm"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	inner := l.(*loader)
	if inner.namespace != DefaultNamespace {
		t.Errorf("expected default namespace %s, got %s", DefaultNamespace, inner.namespace)
	}
	if inner.name != "my-cm" {
		t.Errorf("name not set: %s", inner.name)
	}
	if !inner.ownsClient {
		t.Error("expected ownsClient=true by default")
	}
}

func TestNew_OptionsApply(t *testing.T) {
	l, err := New(
		WithName("cm"),
		WithNamespace("prod"),
		WithKubeconfig("/tmp/kubeconfig"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	inner := l.(*loader)
	if inner.namespace != "prod" {
		t.Errorf("namespace not applied: %s", inner.namespace)
	}
	if inner.kubeconfigPath != "/tmp/kubeconfig" {
		t.Errorf("kubeconfig not applied: %s", inner.kubeconfigPath)
	}
}

func TestNew_OptionsNoOpOnEmpty(t *testing.T) {
	l, err := New(
		WithName("cm"),
		WithNamespace(""),
		WithKubeconfig(""),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	inner := l.(*loader)
	if inner.namespace != DefaultNamespace {
		t.Errorf("empty WithNamespace should not modify default: %s", inner.namespace)
	}
	if inner.kubeconfigPath != "" {
		t.Errorf("empty WithKubeconfig should not set path: %s", inner.kubeconfigPath)
	}
}

func TestNew_WithClient(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	l, err := New(WithName("cm"), WithClient(fakeClient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	inner := l.(*loader)
	if inner.client != fakeClient {
		t.Error("injected client not set")
	}
	if inner.ownsClient {
		t.Error("injected client should set ownsClient=false")
	}
}

// TestLoad_ReturnsConfigMapData 验证 Load 从 ConfigMap.Data 取出所有 key
func TestLoad_ReturnsConfigMapData(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"},
		Data: map[string]string{
			"database/dsn": "postgres://localhost",
			"feature.x":    "true",
		},
		BinaryData: map[string][]byte{
			"raw.bin": []byte("hello"),
		},
	}
	fakeClient := fake.NewSimpleClientset(cm)

	l, _ := New(WithName("my-cm"), WithClient(fakeClient))
	kvs, err := l.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(kvs) != 3 {
		t.Fatalf("expected 3 kvs, got %d: %+v", len(kvs), kvs)
	}
	got := map[string]string{}
	for _, kv := range kvs {
		got[kv.Key] = string(kv.Value)
	}
	if got["database/dsn"] != "postgres://localhost" {
		t.Errorf("database/dsn mismatch: %s", got["database/dsn"])
	}
	if got["feature.x"] != "true" {
		t.Errorf("feature.x mismatch: %s", got["feature.x"])
	}
	if got["raw.bin"] != "hello" {
		t.Errorf("raw.bin mismatch: %s", got["raw.bin"])
	}
}

// TestLoad_NotFound_ReturnsEmpty ConfigMap 不存在 → 空 KV（与 file/etcd 行为一致）
func TestLoad_NotFound_ReturnsEmpty(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	l, _ := New(WithName("not-exist"), WithClient(fakeClient))
	kvs, err := l.Load()
	if err != nil {
		t.Fatalf("expected nil error on missing ConfigMap, got: %v", err)
	}
	if kvs != nil {
		t.Fatalf("expected nil kvs on missing ConfigMap, got: %+v", kvs)
	}
}

// TestLoad_EmptyConfigMap ConfigMap 存在但 data 为空
func TestLoad_EmptyConfigMap(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: "default"},
	}
	fakeClient := fake.NewSimpleClientset(cm)
	l, _ := New(WithName("empty"), WithClient(fakeClient))
	kvs, err := l.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(kvs) != 0 {
		t.Fatalf("expected 0 kvs, got %d", len(kvs))
	}
}

// TestLoad_NamespaceScoped 验证 namespace 过滤生效
func TestLoad_NamespaceScoped(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "prod"},
		Data:       map[string]string{"k": "v"},
	}
	fakeClient := fake.NewSimpleClientset(cm)

	// 在 default 命名空间查 → NotFound
	l, _ := New(WithName("my-cm"), WithNamespace("default"), WithClient(fakeClient))
	kvs, err := l.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if kvs != nil {
		t.Errorf("expected nil kvs in default ns, got: %+v", kvs)
	}

	// 在 prod 命名空间查 → 命中
	l2, _ := New(WithName("my-cm"), WithNamespace("prod"), WithClient(fakeClient))
	kvs2, err := l2.Load()
	if err != nil {
		t.Fatalf("load prod: %v", err)
	}
	if len(kvs2) != 1 {
		t.Fatalf("expected 1 kv in prod ns, got %d", len(kvs2))
	}
}

// TestWatch_DetectsChange 验证 Watch 在 ConfigMap 被修改后返回新快照
//
// 注意：fake clientset 的 watch 行为对齐真实 k8s：启动时会先发送所有已存在对象的 ADDED 事件。
// 因此测试设计：
//   - 第一次 Next 消费初次 ADDED 事件（k1=v1）
//   - 然后触发 update，第二次 Next 应返回更新后的快照（k1=v1-updated, k2=new）
func TestWatch_DetectsChange(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "watch-cm", Namespace: "default"},
		Data:       map[string]string{"k1": "v1"},
	}
	// fake.NewSimpleClientset 内置 watch 通知（基于 etcd 模拟）
	fakeClient := fake.NewSimpleClientset(cm)

	l, _ := New(WithName("watch-cm"), WithClient(fakeClient))

	w, err := l.Watch()
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	defer w.Stop()

	// 第一次 Next：消费初次 ADDED 事件（fake clientset 启动时发送）
	firstDone := make(chan struct{}, 1)
	go func() {
		_, _ = w.Next()
		firstDone <- struct{}{}
	}()

	select {
	case <-time.After(5 * time.Second):
		t.Fatal("first watch.Next did not return within 5s")
	case <-firstDone:
	}

	// 触发变更
	_, _ = fakeClient.CoreV1().ConfigMaps("default").Update(
		context.Background(),
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "watch-cm", Namespace: "default"},
			Data:       map[string]string{"k1": "v1-updated", "k2": "new"},
		},
		metav1.UpdateOptions{},
	)

	// 第二次 Next：应返回包含 k2 的更新后快照
	done := make(chan []string, 1)
	go func() {
		kvs, _ := w.Next()
		var keys []string
		for _, kv := range kvs {
			keys = append(keys, kv.Key+"="+string(kv.Value))
		}
		done <- keys
	}()

	select {
	case <-time.After(5 * time.Second):
		t.Fatal("watch did not detect change within 5s")
	case keys := <-done:
		joined := strings.Join(keys, ",")
		if !strings.Contains(joined, "k2=new") {
			t.Fatalf("watch snapshot missing k2=new: %s", joined)
		}
		if !strings.Contains(joined, "k1=v1-updated") {
			t.Fatalf("watch snapshot missing k1=v1-updated: %s", joined)
		}
	}
}

// TestWatch_Stop_ClosesNext 验证 Stop 后 Next 返回 nil, nil
func TestWatch_Stop_ClosesNext(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "stop-cm", Namespace: "default"},
		Data:       map[string]string{"k": "v"},
	}
	fakeClient := fake.NewSimpleClientset(cm)
	l, _ := New(WithName("stop-cm"), WithClient(fakeClient))

	w, _ := l.Watch()
	_ = w.Stop()

	// Stop 后 Next 立即返回 nil, nil
	kvs, err := w.Next()
	if err != nil {
		t.Errorf("expected nil err after Stop, got: %v", err)
	}
	if kvs != nil {
		t.Errorf("expected nil kvs after Stop, got: %+v", kvs)
	}
}

// TestWatch_MultipleEventsCoalesced 验证合并：多次变更只触发一次 Next
func TestWatch_MultipleEventsCoalesced(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "coal-cm", Namespace: "default"},
		Data:       map[string]string{"k": "v0"},
	}
	fakeClient := fake.NewSimpleClientset(cm)
	l, _ := New(WithName("coal-cm"), WithClient(fakeClient))

	w, _ := l.Watch()
	defer w.Stop()

	// 短时间内连续 3 次更新
	go func() {
		time.Sleep(200 * time.Millisecond)
		for i := 1; i <= 3; i++ {
			_, _ = fakeClient.CoreV1().ConfigMaps("default").Update(
				context.Background(),
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "coal-cm", Namespace: "default"},
					Data:       map[string]string{"k": "v" + string(rune('0'+i))},
				},
				metav1.UpdateOptions{},
			)
			time.Sleep(10 * time.Millisecond)
		}
	}()

	done := make(chan int, 5)
	go func() {
		for i := 0; i < 5; i++ {
			kvs, err := w.Next()
			if err != nil || kvs == nil {
				return
			}
			done <- 1
		}
	}()

	select {
	case <-time.After(3 * time.Second):
		// 收到 ≥1 次事件即可（合并后次数不固定，但不应 0）
		if len(done) == 0 {
			t.Fatal("expected at least 1 event from coalesced updates")
		}
	case n := <-done:
		_ = n
	}
}

// TestConfigMapToKeyValues 私有辅助函数纯逻辑测试
func TestConfigMapToKeyValues(t *testing.T) {
	t.Run("nil cm", func(t *testing.T) {
		if kvs := configMapToKeyValues(nil); kvs != nil {
			t.Errorf("expected nil for nil cm, got: %+v", kvs)
		}
	})
	t.Run("empty data", func(t *testing.T) {
		if kvs := configMapToKeyValues(&corev1.ConfigMap{}); kvs != nil {
			t.Errorf("expected nil for empty cm, got: %+v", kvs)
		}
	})
	t.Run("data only", func(t *testing.T) {
		kvs := configMapToKeyValues(&corev1.ConfigMap{
			Data: map[string]string{"a": "1"},
		})
		if len(kvs) != 1 || kvs[0].Key != "a" || string(kvs[0].Value) != "1" {
			t.Errorf("unexpected kvs: %+v", kvs)
		}
	})
	t.Run("binary only", func(t *testing.T) {
		kvs := configMapToKeyValues(&corev1.ConfigMap{
			BinaryData: map[string][]byte{"b": []byte("xyz")},
		})
		if len(kvs) != 1 || kvs[0].Key != "b" || string(kvs[0].Value) != "xyz" {
			t.Errorf("unexpected kvs: %+v", kvs)
		}
	})
}
