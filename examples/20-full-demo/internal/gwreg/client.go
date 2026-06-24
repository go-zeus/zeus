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

// Register 注册实例（带重试，srv 启动时 gateway 可能尚未就绪）
func (c *Client) Register(ctx context.Context, ins gwapi.Instance) error {
	body, _ := json.Marshal(gwapi.RegisterRequest{Instance: ins})
	url := c.gatewayURL + "/internal/register"

	var lastErr error
	for attempt := 0; attempt < 30; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			log.Info("register attempt %d failed: %v (retry in 1s)", attempt+1, err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Info("registered to gateway: %s (%s/%s) at %s:%d",
				ins.ID, ins.Name, ins.Cluster, ins.IP, ins.Port)
			return nil
		}
		lastErr = fmt.Errorf("register http %d", resp.StatusCode)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("register failed after retries: %w", lastErr)
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
