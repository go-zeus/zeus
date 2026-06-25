package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/go-zeus/zeus/config"
	"github.com/go-zeus/zeus/config/file"
)

type AppConfig struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func main() {
	// 写入示例配置文件
	os.WriteFile("app.json", []byte(`{"name":"zeus-demo","version":"1.0.0"}`), 0644)
	defer os.Remove("app.json")

	// 使用 file 加载器
	cfg, err := config.NewConfig(file.NewFileWithPath("app.json"))
	if err != nil {
		log.Fatal(err)
	}

	// 直接读取原始值
	raw := cfg.Get("app.json")
	fmt.Printf("raw app.json: %s\n", raw)

	// 使用标准 json 解码
	var appCfg AppConfig
	if err := json.Unmarshal(raw, &appCfg); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("decoded: name=%s version=%s\n", appCfg.Name, appCfg.Version)
}
