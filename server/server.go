package server

import (
	"context"
	"fmt"
	"github.com/go-zeus/zeus/log"
	"net/http"
)

const (
	DefaultPort = 8080
)

var DefaultServer = New()

type Server interface {
	Init(opts ...Option)
	GetIp() string
	GetPort() int
	Run(close <-chan struct{}) error
}

type server struct {
	ip   string
	port int
	*http.Server
}

type Option func(s *server)

func New(opts ...Option) Server {
	s := &server{
		Server: &http.Server{},
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.port == 0 {
		s.port = DefaultPort
	}
	if s.Handler == nil {
		s.Handler = DefaultHandler()
	}
	return s
}

func (s *server) GetIp() string {
	return s.ip
}

func (s *server) GetPort() int {
	return s.port
}

func (s *server) Init(opts ...Option) {
	for _, opt := range opts {
		opt(s)
	}
}

func (s *server) address() string {
	return fmt.Sprintf("%s:%d", s.ip, s.port)
}

func (s *server) Run(close <-chan struct{}) error {
	log.Info("server start is %s", s.address())
	s.Addr = s.address()
	go func() {
		<-close
		s.Shutdown(context.TODO())
	}()
	return s.ListenAndServe()
}
