package context

import (
	"context"
	"net/http"
)

type Context interface {
	Request() *http.Request
	Response() http.ResponseWriter
	Ctx() context.Context
	ClientIP() string
}

type cxt struct {
	request  *http.Request
	response http.ResponseWriter
	ctx      context.Context
	clientIP string
}

type Option func(c *cxt)

func New(opt ...Option) Context {
	c := &cxt{
		request: &http.Request{},
	}
	for _, o := range opt {
		o(c)
	}
	return c
}

func (c *cxt) Request() *http.Request {
	return c.request
}

func (c *cxt) Response() http.ResponseWriter {
	return c.response
}

func (c *cxt) Ctx() context.Context {
	return c.ctx
}

func (c *cxt) ClientIP() string {
	return c.clientIP
}

func Request(r *http.Request) Option {
	return func(c *cxt) {
		c.request = r
	}
}

func Response(r http.ResponseWriter) Option {
	return func(c *cxt) {
		c.response = r
	}
}

func Ctx(ctx context.Context) Option {
	return func(c *cxt) {
		c.ctx = ctx
	}
}
