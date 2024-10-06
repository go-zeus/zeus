package client

import (
	"github.com/go-zeus/zeus/balancer"
	"github.com/go-zeus/zeus/registry"
)

func Discovery(dis registry.Discovery) Option {
	return func(c *client) {
		c.dis = dis
	}
}

func LoadBalance(lb balancer.LoadBalance) Option {
	return func(c *client) {
		c.lb = lb
	}
}
