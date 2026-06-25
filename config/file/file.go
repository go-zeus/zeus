package file

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-zeus/zeus/config"
)

// 编译期检查 fileDriver 实现了 config.Loader 接口
var _ config.Loader = (*fileDriver)(nil)

type fileDriver struct {
	path string
}

// NewFile 创建文件配置加载器
func NewFile() config.Loader {
	return &fileDriver{}
}

// NewFileWithPath 创建指定路径的文件配置加载器
func NewFileWithPath(path string) config.Loader {
	return &fileDriver{path: path}
}

func (f *fileDriver) Load() ([]config.KeyValue, error) {
	if f.path == "" {
		return nil, nil
	}
	info, err := os.Stat(f.path)
	if err != nil {
		return nil, fmt.Errorf("config/file: stat %s: %w", f.path, err)
	}
	var kvs []config.KeyValue
	if info.IsDir() {
		entries, err := os.ReadDir(f.path)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(f.path, entry.Name()))
			if err != nil {
				continue
			}
			kvs = append(kvs, config.KeyValue{
				Key:   entry.Name(),
				Value: data,
			})
		}
	} else {
		data, err := os.ReadFile(f.path)
		if err != nil {
			return nil, err
		}
		kvs = append(kvs, config.KeyValue{
			Key:   filepath.Base(f.path),
			Value: data,
		})
	}
	return kvs, nil
}

func (f *fileDriver) Watch() (config.Watcher, error) {
	return &fileWatcher{
		path:    f.path,
		stopCh:  make(chan struct{}),
		modTime: f.lastModTime(),
	}, nil
}

// fileWatcher 基于文件修改时间的轮询监听
type fileWatcher struct {
	path     string
	stopCh   chan struct{}
	stopOnce sync.Once
	mu       sync.Mutex
	modTime  time.Time
}

// lastModTime 获取文件/目录的最新修改时间
func (f *fileDriver) lastModTime() time.Time {
	if f.path == "" {
		return time.Time{}
	}
	info, err := os.Stat(f.path)
	if err != nil {
		return time.Time{}
	}
	if info.IsDir() {
		// 目录取最新的文件修改时间
		latest := info.ModTime()
		entries, err := os.ReadDir(f.path)
		if err != nil {
			return latest
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			fi, err := entry.Info()
			if err != nil {
				continue
			}
			if fi.ModTime().After(latest) {
				latest = fi.ModTime()
			}
		}
		return latest
	}
	return info.ModTime()
}

// 编译期检查 fileWatcher 实现了 config.Watcher 接口
var _ config.Watcher = (*fileWatcher)(nil)

func (w *fileWatcher) Next() ([]config.KeyValue, error) {
	// 轮询检测文件变化，每秒检查一次
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-w.stopCh:
			return nil, nil
		case <-ticker.C:
			mt := w.lastModTime()
			if mt.IsZero() {
				continue
			}
			w.mu.Lock()
			changed := !mt.Equal(w.modTime)
			if changed {
				w.modTime = mt
			}
			w.mu.Unlock()
			if changed {
				// 重新加载文件内容
				return w.load()
			}
		}
	}
}

// lastModTime 获取当前最新修改时间
func (w *fileWatcher) lastModTime() time.Time {
	info, err := os.Stat(w.path)
	if err != nil {
		return time.Time{}
	}
	if info.IsDir() {
		latest := info.ModTime()
		entries, err := os.ReadDir(w.path)
		if err != nil {
			return latest
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			fi, err := entry.Info()
			if err != nil {
				continue
			}
			if fi.ModTime().After(latest) {
				latest = fi.ModTime()
			}
		}
		return latest
	}
	return info.ModTime()
}

// load 加载文件内容
func (w *fileWatcher) load() ([]config.KeyValue, error) {
	info, err := os.Stat(w.path)
	if err != nil {
		return nil, fmt.Errorf("config/file: stat %s: %w", w.path, err)
	}
	var kvs []config.KeyValue
	if info.IsDir() {
		entries, err := os.ReadDir(w.path)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(w.path, entry.Name()))
			if err != nil {
				continue
			}
			kvs = append(kvs, config.KeyValue{
				Key:   entry.Name(),
				Value: data,
			})
		}
	} else {
		data, err := os.ReadFile(w.path)
		if err != nil {
			return nil, err
		}
		kvs = append(kvs, config.KeyValue{
			Key:   filepath.Base(w.path),
			Value: data,
		})
	}
	return kvs, nil
}

func (w *fileWatcher) Stop() error {
	w.stopOnce.Do(func() {
		close(w.stopCh)
	})
	return nil
}
