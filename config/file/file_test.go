package file

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-zeus/zeus/config"
)

// TestNewFile 测试创建文件加载器
func TestNewFile(t *testing.T) {
	d := NewFile()
	if d == nil {
		t.Fatal("NewFile returned nil")
	}
}

// TestNewFileWithPath 测试创建指定路径的文件加载器
func TestNewFileWithPath(t *testing.T) {
	d := NewFileWithPath("/tmp/test")
	if d == nil {
		t.Fatal("NewFileWithPath returned nil")
	}
}

// TestLoad_EmptyPath 测试空路径返回 nil, nil
func TestLoad_EmptyPath(t *testing.T) {
	d := NewFile()
	kvs, err := d.Load()
	if err != nil {
		t.Fatalf("Load with empty path should not return error, got %v", err)
	}
	if kvs != nil {
		t.Fatalf("Load with empty path should return nil, got %v", kvs)
	}
}

// TestLoad_SingleFile 测试加载单个文件
func TestLoad_SingleFile(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello zeus")
	fname := "app.yaml"
	fpath := filepath.Join(dir, fname)
	if err := os.WriteFile(fpath, content, 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	d := NewFileWithPath(fpath)
	kvs, err := d.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(kvs) != 1 {
		t.Fatalf("expected 1 KeyValue, got %d", len(kvs))
	}
	if kvs[0].Key != fname {
		t.Errorf("Key = %q, want %q", kvs[0].Key, fname)
	}
	if string(kvs[0].Value) != string(content) {
		t.Errorf("Value = %q, want %q", string(kvs[0].Value), string(content))
	}
}

// TestLoad_Directory 测试加载目录下所有文件
func TestLoad_Directory(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"app.yaml":   "name: zeus",
		"db.yaml":    "host: localhost",
		"redis.yaml": "addr: :6379",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatalf("write temp file %s: %v", name, err)
		}
	}

	d := NewFileWithPath(dir)
	kvs, err := d.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(kvs) != len(files) {
		t.Fatalf("expected %d KeyValues, got %d", len(files), len(kvs))
	}

	// 将结果转为 map 方便比较
	got := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		got[kv.Key] = string(kv.Value)
	}
	for name, want := range files {
		if got[name] != want {
			t.Errorf("Key %q: Value = %q, want %q", name, got[name], want)
		}
	}
}

// TestLoad_DirectorySkipsSubDirs 测试加载目录时跳过子目录
func TestLoad_DirectorySkipsSubDirs(t *testing.T) {
	dir := t.TempDir()
	// 创建一个子目录
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// 创建一个文件
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte("test"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	d := NewFileWithPath(dir)
	kvs, err := d.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(kvs) != 1 {
		t.Fatalf("expected 1 KeyValue (subdir skipped), got %d", len(kvs))
	}
}

// TestLoad_NonexistentPath 测试加载不存在的路径返回错误
func TestLoad_NonexistentPath(t *testing.T) {
	d := NewFileWithPath("/nonexistent/path/to/config.yaml")
	_, err := d.Load()
	if err == nil {
		t.Fatal("Load nonexistent path should return error, got nil")
	}
}

// TestWatch_Stop 测试 Watcher 的 Stop 方法
func TestWatch_Stop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), []byte("test"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	d := NewFileWithPath(dir)
	w, err := d.Watch()
	if err != nil {
		t.Fatalf("Watch error: %v", err)
	}
	if w == nil {
		t.Fatal("Watch returned nil watcher")
	}

	// 停止 watcher
	if err := w.Stop(); err != nil {
		t.Fatalf("Stop error: %v", err)
	}

	// Stop 后 Next 应该返回 nil, nil
	kvs, err := w.Next()
	if err != nil {
		t.Fatalf("Next after Stop should not return error, got %v", err)
	}
	if kvs != nil {
		t.Fatalf("Next after Stop should return nil, got %v", kvs)
	}
}

// TestWatch_DoubleStop 测试重复调用 Stop 不会 panic
func TestWatch_DoubleStop(t *testing.T) {
	d := NewFileWithPath(t.TempDir())
	w, err := d.Watch()
	if err != nil {
		t.Fatalf("Watch error: %v", err)
	}

	if err := w.Stop(); err != nil {
		t.Fatalf("first Stop error: %v", err)
	}
	// sync.Once 保证第二次 Stop 不会 panic（不会重复 close channel）
	if err := w.Stop(); err != nil {
		t.Fatalf("second Stop should not error, got %v", err)
	}
}

// TestLoad_DirectoryType 测试验证返回的 KeyValue 结构正确
func TestLoad_DirectoryType(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"k":"v"}`), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	d := NewFileWithPath(dir)
	kvs, err := d.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	var found bool
	for _, kv := range kvs {
		if kv.Key == "a.json" {
			found = true
			if string(kv.Value) != `{"k":"v"}` {
				t.Errorf("Value = %q, want %q", string(kv.Value), `{"k":"v"}`)
			}
		}
	}
	if !found {
		t.Error("a.json not found in loaded KeyValues")
	}
}

// TestWatch_DetectsChange 测试 Watch 能检测到文件变更
func TestWatch_DetectsChange(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "app.yaml")

	// 写入初始内容
	if err := os.WriteFile(fpath, []byte("version: 1"), 0644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	d := NewFileWithPath(dir)
	w, err := d.Watch()
	if err != nil {
		t.Fatalf("Watch error: %v", err)
	}
	defer w.Stop()

	// 在后台等待一小段时间后修改文件
	go func() {
		time.Sleep(500 * time.Millisecond)
		if err := os.WriteFile(fpath, []byte("version: 2"), 0644); err != nil {
			t.Errorf("write updated file: %v", err)
		}
	}()

	// Next 应该能检测到变更并返回更新后的内容
	kvs, err := w.Next()
	if err != nil {
		t.Fatalf("Next error: %v", err)
	}
	if len(kvs) != 1 {
		t.Fatalf("expected 1 KeyValue, got %d", len(kvs))
	}
	if kvs[0].Key != "app.yaml" {
		t.Errorf("Key = %q, want %q", kvs[0].Key, "app.yaml")
	}
	if string(kvs[0].Value) != "version: 2" {
		t.Errorf("Value = %q, want %q", string(kvs[0].Value), "version: 2")
	}
}

// 确认 fileDriver 实现了 config.Loader 接口
var _ config.Loader = (*fileDriver)(nil)

// TestLastModTime_EmptyPath 空路径返回零时间
func TestLastModTime_EmptyPath(t *testing.T) {
	f := &fileDriver{path: ""}
	if got := f.lastModTime(); !got.IsZero() {
		t.Errorf("empty path should return zero time, got %v", got)
	}
}

// TestLastModTime_NonexistentPath 不存在路径返回零时间
func TestLastModTime_NonexistentPath(t *testing.T) {
	f := &fileDriver{path: "/this/path/does/not/exist/zeus_test"}
	if got := f.lastModTime(); !got.IsZero() {
		t.Errorf("nonexistent path should return zero time, got %v", got)
	}
}

// TestLastModTime_Directory 目录场景：取最新文件修改时间
func TestLastModTime_Directory(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{}`), 0644)
	// 稍后再写 b.json 确保 b 比新
	time.Sleep(20 * time.Millisecond)
	_ = os.WriteFile(filepath.Join(dir, "b.json"), []byte(`{}`), 0644)

	f := &fileDriver{path: dir}
	mt := f.lastModTime()
	if mt.IsZero() {
		t.Fatal("directory lastModTime should not be zero")
	}
	// mt 应不早于 b.json 的修改时间
	infoB, _ := os.Stat(filepath.Join(dir, "b.json"))
	if mt.Before(infoB.ModTime()) {
		t.Errorf("modTime %v before b.json %v", mt, infoB.ModTime())
	}
}

// TestLoad_ReadDirError 目录被删除时 ReadDir 失败路径（load()）
func TestLoad_ReadDirError(t *testing.T) {
	dir := t.TempDir()
	// 创建并立即删除
	_ = os.Mkdir(filepath.Join(dir, "sub"), 0755)
	f := &fileDriver{path: dir}
	// 用 chmod 让 ReadDir 失败较难跨平台；改为删除目录后用 NewFileWithPath 残留路径
	// 简化：path 指向已不存在的目录 → Stat 失败 → Load 返回 error
	removed := filepath.Join(dir, "removed")
	_ = os.Mkdir(removed, 0755)
	_ = os.RemoveAll(removed)
	f2 := &fileDriver{path: removed}
	if _, err := f2.Load(); err == nil {
		t.Error("Load on removed dir should return error")
	}
	_ = f
}

// TestLoad_FileReadError 单文件场景读失败
func TestLoad_FileReadError(t *testing.T) {
	// 用一个存在但不可读的文件（仅 Unix 权限模式）
	if os.Geteuid() == 0 {
		t.Skip("running as root, permission test unreliable")
	}
	dir := t.TempDir()
	fpath := filepath.Join(dir, "secret.yaml")
	_ = os.WriteFile(fpath, []byte("k: v"), 0000)
	defer os.Chmod(fpath, 0644)
	f := &fileDriver{path: fpath}
	_, err := f.Load()
	if err == nil {
		t.Error("Load on unreadable file should return error")
	}
}

// TestWatch_NextStopAfterStop Stop 后 Next 立即返回 nil
func TestWatch_NextStopAfterStop(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{}`), 0644)
	d := NewFileWithPath(dir)
	w, _ := d.Watch()
	// 先 Stop，再 Next 应立即返回 (nil, nil)
	if err := w.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	kvs, err := w.Next()
	if err != nil {
		t.Errorf("Next after Stop err: %v", err)
	}
	if kvs != nil {
		t.Errorf("Next after Stop should return nil kvs, got %v", kvs)
	}
}

// TestWatch_LoadDirectoryType directory load through fileWatcher.load()
func TestWatch_LoadDirectoryType(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"k":1}`), 0644)
	_ = os.WriteFile(filepath.Join(dir, "b.yaml"), []byte("k: 2"), 0644)

	d := NewFileWithPath(dir)
	w, err := d.Watch()
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer w.Stop()

	fw := w.(*fileWatcher)
	kvs, err := fw.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(kvs) != 2 {
		t.Errorf("kvs len = %d, want 2", len(kvs))
	}
}

// TestLoad_EmptyDirectory 空目录返回空 KV 切片
func TestLoad_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	f := NewFileWithPath(dir)
	kvs, err := f.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(kvs) != 0 {
		t.Errorf("empty dir kvs len = %d, want 0", len(kvs))
	}
}
