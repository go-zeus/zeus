package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-zeus/zeus/components"
	"github.com/go-zeus/zeus/server"
	_ "github.com/go-zeus/zeus/utils/banner"
	"github.com/go-zeus/zeus/utils/errgroup"
	"os"
	"os/signal"
	"syscall"
)

type App interface {
	Run(close <-chan struct{}) error
}

func New(opts ...Option) App {
	a := &app{}
	for _, opt := range opts {
		opt(a)
	}
	if len(a.servers) < 1 {
		a.servers = append(a.servers, server.DefaultServer)
	}
	return a
}

func NewForConfig(c *Config) App {
	a := &app{c: c}
	return a
}

type Option func(s *app)

type app struct {
	c       *Config
	servers []server.Server
	components.BaseInstance
}

func (a *app) Run(close <-chan struct{}) error {
	eg, cancelCtx := errgroup.WithContext(context.TODO())
	for _, s := range a.servers {
		eg.Go(func() error {
			return s.Run(cancelCtx.Done())
		})
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGKILL)
	eg.Go(func() error {
		for {
			select {
			case <-close:
				fmt.Println("signal ctx done")
				return nil
			case sig := <-ch:
				return errors.New("get signal " + sig.String() + ", application will shutdown\n")
			}
		}
	})
	if err := eg.Wait(); err != nil {
		return err
	}
	return nil
}
