package client

import (
	"fmt"
	"github.com/go-zeus/zeus/balancer"
	"github.com/go-zeus/zeus/balancer/random"
	"github.com/go-zeus/zeus/registry"
	"net/http"
	"net/url"
)

type Client interface {
	Do(r *http.Request) (*http.Response, error)
}

type client struct {
	name     string //服务名称
	dis      registry.Discovery
	lb       balancer.LoadBalance
	clusters map[string]balancer.LoadBalance
	cc       *http.Client
}

type Option func(c *client)

func NewClient(name string, opts ...Option) Client {
	c := &client{
		name:     name,
		cc:       http.DefaultClient,
		lb:       random.NewRandom(),
		clusters: make(map[string]balancer.LoadBalance),
	}
	for _, opt := range opts {
		opt(c)
	}
	c.load()
	go c.watcher()

	return c
}

func (c *client) watcher() {
	ch := c.dis.Watch(c.name)
	for {
		<-ch
		c.load()
	}
}

func (c *client) load() {
	srv := c.dis.GetService(c.name)
	for name, cl := range srv.Clusters {
		c.clusters[name] = c.lb.Reload(cl.GetInstances())
	}
}

func (c *client) Do(r *http.Request) (*http.Response, error) {
	color := r.Header.Get("X-color")
	if color == "" {
		color = "default"
	}
	ins, _ := c.clusters[color].Next()
	r.URL, _ = url.Parse(fmt.Sprintf("%s://%s:%d", r.URL.Scheme, ins.Ip, ins.Port))
	return c.cc.Do(r)
}
