package config

import (
	"encoding/json"
	"sync"
	"testing"
)

// ---- mock loader ----

type mockLoader struct {
	kvs []KeyValue
}

func (m *mockLoader) Load() ([]KeyValue, error) {
	return m.kvs, nil
}

func (m *mockLoader) Watch() (Watcher, error) {
	return &noopWatcher{}, nil
}

type noopWatcher struct{}

func (w *noopWatcher) Next() ([]KeyValue, error) { return nil, nil }
func (w *noopWatcher) Stop() error               { return nil }

// ---- tests ----

func TestNewConfig(t *testing.T) {
	d := &mockLoader{
		kvs: []KeyValue{
			{Key: "app.name", Value: []byte("zeus")},
		},
	}
	c, err := NewConfig(d)
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}
	if c == nil {
		t.Fatal("NewConfig returned nil")
	}
}

func TestGet(t *testing.T) {
	d := &mockLoader{
		kvs: []KeyValue{
			{Key: "app.name", Value: []byte("zeus")},
			{Key: "app.port", Value: []byte("8080")},
		},
	}
	c, err := NewConfig(d)
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	val := c.Get("app.name")
	if string(val) != "zeus" {
		t.Errorf("Get(%q) = %q, want %q", "app.name", string(val), "zeus")
	}

	val = c.Get("app.port")
	if string(val) != "8080" {
		t.Errorf("Get(%q) = %q, want %q", "app.port", string(val), "8080")
	}

	val = c.Get("nonexistent")
	if val != nil {
		t.Errorf("Get(nonexistent) = %v, want nil", val)
	}
}

func TestUnmarshal(t *testing.T) {
	type appConfig struct {
		Name string `json:"name"`
		Port int    `json:"port"`
	}

	raw, _ := json.Marshal(appConfig{Name: "zeus", Port: 9090})

	d := &mockLoader{
		kvs: []KeyValue{
			{Key: "app", Value: raw},
		},
	}
	c, err := NewConfig(d)
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	var cfg appConfig
	err = c.Unmarshal("app", &cfg, jsonDecoder{})
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if cfg.Name != "zeus" {
		t.Errorf("Name = %q, want %q", cfg.Name, "zeus")
	}
	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want %d", cfg.Port, 9090)
	}
}

// jsonDecoder 使用标准库 json 做 Decode
type jsonDecoder struct{}

func (jsonDecoder) Decode(src []byte, dst any) error {
	return json.Unmarshal(src, dst)
}

// mockWatcherWithChannel 支持推送变更的 mock watcher
type mockWatcherWithChannel struct {
	ch      chan []KeyValue
	stopped bool
}

func newMockWatcher() *mockWatcherWithChannel {
	return &mockWatcherWithChannel{ch: make(chan []KeyValue, 10)}
}

func (w *mockWatcherWithChannel) Next() ([]KeyValue, error) {
	kvs, ok := <-w.ch
	if !ok {
		return nil, nil
	}
	return kvs, nil
}

func (w *mockWatcherWithChannel) Stop() error {
	if !w.stopped {
		w.stopped = true
		close(w.ch)
	}
	return nil
}

// mockLoaderWithWatcher 支持返回可推送 watcher 的 mock loader
type mockLoaderWithWatcher struct {
	kvs     []KeyValue
	watcher *mockWatcherWithChannel
}

func (m *mockLoaderWithWatcher) Load() ([]KeyValue, error) {
	return m.kvs, nil
}

func (m *mockLoaderWithWatcher) Watch() (Watcher, error) {
	return m.watcher, nil
}

func TestConfig_ConcurrentGetSet(t *testing.T) {
	w := newMockWatcher()
	d := &mockLoaderWithWatcher{
		kvs: []KeyValue{
			{Key: "app.name", Value: []byte("zeus")},
		},
		watcher: w,
	}
	c, err := NewConfig(d)
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	// 启动 Watch
	if err := c.Watch(); err != nil {
		t.Fatalf("Watch error: %v", err)
	}

	// 并发推送配置变更和读取
	var wg sync.WaitGroup
	const goroutines = 20

	// 多个 goroutine 并发 Get
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				val := c.Get("app.name")
				_ = val
			}
		}()
	}

	// 同时通过 watcher 推送配置变更
	for i := 0; i < 50; i++ {
		w.ch <- []KeyValue{
			{Key: "app.name", Value: []byte("updated")},
		}
	}

	wg.Wait()
	// 关闭 watcher
	_ = c.Close()
}

func TestConfig_Close_Twice(t *testing.T) {
	d := &mockLoader{
		kvs: []KeyValue{
			{Key: "app.name", Value: []byte("zeus")},
		},
	}
	c, err := NewConfig(d)
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	// 第一次 Close 不应报错
	if err := c.Close(); err != nil {
		t.Fatalf("first Close error: %v", err)
	}

	// 第二次 Close 也不应 panic，应返回 nil
	if err := c.Close(); err != nil {
		t.Fatalf("second Close error: %v", err)
	}
}
