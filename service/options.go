package service

import (
	"github.com/go-zeus/zeus/server"
)

const (
	DefaultServiceName = "zeus-service"
	DefaultClusterName = "default"
)

func Name(name string) Option {
	return func(s *Service) {
		s.Name = name
	}
}

func Cluster(name string) Option {
	return func(s *Service) {
		s.Cluster = name
	}
}

func Ip(ip string) Option {
	return func(s *Service) {
		s.Ip = ip
	}
}

func Server(server server.Server) Option {
	return func(s *Service) {
		s.server = server
	}
}
