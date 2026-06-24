package config

import (
	"fmt"
	"sync"
)

// KeyValue 配置键值对
type KeyValue struct {
	Key   string
	Value []byte
}

// Loader 配置加载器接口（实现者实现此接口）
type Loader interface {
	Load() ([]KeyValue, error)
	Watch() (Watcher, error)
}

// Watcher 配置变更监听接口
type Watcher interface {
	Next() ([]KeyValue, error)
	Stop() error
}

// Decoder 解码器接口
type Decoder interface {
	Decode(src []byte, dst any) error
}

// Config 配置管理器（用户 API）
type Config struct {
	mu      sync.RWMutex
	loader  Loader
	values  map[string][]byte
	watcher Watcher
	closed  bool
}

// NewConfig 从加载器创建 Config
func NewConfig(loader Loader) (*Config, error) {
	c := &Config{loader: loader, values: make(map[string][]byte)}
	kvs, err := loader.Load()
	if err != nil {
		return nil, err
	}
	for _, kv := range kvs {
		c.values[kv.Key] = kv.Value
	}
	return c, nil
}

// Get 获取配置值
func (c *Config) Get(key string) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.values[key]
}

// Unmarshal 反序列化配置
func (c *Config) Unmarshal(key string, dst any, decoder Decoder) error {
	c.mu.RLock()
	val, ok := c.values[key]
	c.mu.RUnlock()
	if !ok {
		return fmt.Errorf("config: key %q not found", key)
	}
	return decoder.Decode(val, dst)
}

// Watch 监听配置变更
func (c *Config) Watch() error {
	c.mu.Lock()
	if c.watcher != nil {
		c.mu.Unlock()
		return nil
	}
	w, err := c.loader.Watch()
	if err != nil {
		c.mu.Unlock()
		return err
	}
	c.watcher = w
	c.mu.Unlock()
	go func() {
		for {
			kvs, err := w.Next()
			if err != nil {
				return
			}
			// watcher 停止后 Next 返回 nil,nil，退出循环避免空转
			if kvs == nil {
				return
			}
			c.mu.Lock()
			for _, kv := range kvs {
				c.values[kv.Key] = kv.Value
			}
			c.mu.Unlock()
		}
	}()
	return nil
}

// Close 关闭配置
func (c *Config) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if c.watcher != nil {
		return c.watcher.Stop()
	}
	return nil
}
