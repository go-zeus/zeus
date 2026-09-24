// Package gwreg 提供 gateway 自注册 HTTP 客户端。
//
// srv 启动时调用 Register 把自己注册到 gateway 的内存注册中心；
// 关闭时调用 Deregister 反注册，保证 gateway 不再路由流量到已下线实例。
package gwreg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-zeus/zeus/examples/20-full-demo/internal/gwapi"
	"github.com/go-zeus/zeus/log"
)

// Client gateway 注册客户端
type Client struct {
	gatewayURL string
	http       *http.Client
}

// New 创建注册客户端
// gatewayURL 形如 "http://gateway:8080"
func New(gatewayURL string) *Client {
	return &Client{
		gatewayURL: gatewayURL,
		http:       &http.Client{Timeout: 5 * time.Second},
	}
}

// keepAliveInterval 心跳重注册周期（gateway 重启后最长经过该时长自动恢复注册）
// 包级变量便于测试缩短周期，生产固定 10s
var keepAliveInterval = 10 * time.Second

// Register 注册实例（带重试，srv 启动时 gateway 可能尚未就绪）
func (c *Client) Register(ctx context.Context, ins gwapi.Instance) error {
	body, _ := json.Marshal(gwapi.RegisterRequest{Instance: ins})
	url := c.gatewayURL + "/internal/register"

	var lastErr error
	for attempt := 0; attempt < 30; attempt++ {
		err := c.registerOnce(ctx, url, body)
		if err == nil {
			log.Info("registered to gateway: %s (%s/%s) at %s:%d",
				ins.ID, ins.Name, ins.Cluster, ins.IP, ins.Port)
			return nil
		}
		lastErr = err
		log.Info("register attempt %d failed: %v (retry in 1s)", attempt+1, err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("register failed after retries: %w", lastErr)
}

// registerOnce 单次注册请求（Register 重试和 KeepAlive 心跳共用）
func (c *Client) registerOnce(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("register http %d", resp.StatusCode)
}

// KeepAlive 启动心跳重注册：周期性向 gateway 重复注册，覆盖以下场景：
//   - gateway Pod 重启导致内嵌 memory registry 状态丢失（单次注册随之蒸发）
//   - 注册请求打到垂死的旧 gateway 实例（换代部署窗口期）
//
// 单次心跳失败仅记录日志，下个周期重试；gateway 端需将重复注册视为幂等成功。
// 返回 stop 函数，调用后停止心跳（应在 Deregister 之前调用）。
func (c *Client) KeepAlive(ins gwapi.Instance) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	body, _ := json.Marshal(gwapi.RegisterRequest{Instance: ins})
	url := c.gatewayURL + "/internal/register"

	go func() {
		ticker := time.NewTicker(keepAliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 心跳失败不退出：gateway 可能正在重启，下个周期重试
				if err := c.registerOnce(ctx, url, body); err != nil {
					log.Warn("keepalive failed (will retry): %v", err)
				}
			}
		}
	}()
	return cancel
}

// Deregister 反注册实例（关闭时调用，失败仅 log 不阻塞关闭流程）
func (c *Client) Deregister(ctx context.Context, id string) {
	url := fmt.Sprintf("%s/internal/register?id=%s", c.gatewayURL, id)
	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		log.Error("deregister failed: %v", err)
		return
	}
	resp.Body.Close()
	log.Info("deregistered from gateway: %s", id)
}
