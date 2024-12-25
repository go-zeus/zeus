package components

import (
	"errors"
	"sync"
)

var ins = &instances{data: make(map[string]Instance)}

type Instance interface {
	InstanceState
}

type InstanceState interface {
	SetReady()
	IsReady() bool
	Wait()
}

type instances struct {
	data map[string]Instance
	mu   sync.RWMutex
}

func SetInstance(id string, instance Instance) error {
	if id == "" {
		return errors.New("实例id不能为空")
	}
	if instance == nil {
		return errors.New("不能设置空实例")
	}
	ins.mu.Lock()
	defer ins.mu.Unlock()
	ins.data[id] = instance
	return nil
}

func GetInstance[T Instance](id string) (T, error) {
	var zero T
	ins.mu.RLock()
	defer ins.mu.RUnlock()
	instance, exists := ins.data[id]
	if !exists {
		return zero, errors.New("实例不存在")
	}
	return instance.(T), nil
}

func GetWaitInstance(id string) (Instance, error) {
	ins.mu.RLock()
	instance, exists := ins.data[id]
	ins.mu.RUnlock()

	if !exists {
		return nil, errors.New("实例不存在")
	}

	// 调用实例自身的 Wait 方法，确保在持有实例自己的锁的情况下进行等待。
	instance.Wait()

	return instance, nil
}

func DelInstance(id string) {
	ins.mu.Lock()
	defer ins.mu.Unlock()
	delete(ins.data, id)
}

func IsReady() bool {
	ins.mu.RLock()
	defer ins.mu.RUnlock()
	for _, i := range ins.data {
		if !i.IsReady() {
			return false
		}
	}
	return true
}
