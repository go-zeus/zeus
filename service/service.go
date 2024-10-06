package service

import (
	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/metadata"
	"github.com/go-zeus/zeus/registry"
	"github.com/go-zeus/zeus/server"
	"github.com/go-zeus/zeus/types"
	"github.com/go-zeus/zeus/utils/ip"
	"github.com/go-zeus/zeus/utils/uuid"
)

// Service 服务
type Service struct {
	types.Instance
	dis    registry.Register
	server server.Server
}

type Option func(s *Service)

func New(opts ...Option) *Service {
	s := &Service{
		Instance: types.Instance{
			Id:       uuid.New(),
			Name:     DefaultServiceName,
			Cluster:  DefaultClusterName,
			Ip:       ip.LocalIP(),
			Metadata: make(metadata.MD),
			Labels:   make(map[string][]string),
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.server == nil {
		s.server = server.DefaultServer
	} else {
		if s.server.GetIp() != "" {
			s.Ip = s.server.GetIp()
		}
		if s.server.GetPort() > 0 {
			s.Port = s.server.GetPort()
		} else {
			s.server.Init(
				server.Port(s.Port),
			)
		}
	}
	return s
}

func (s *Service) Run(close <-chan struct{}) error {
	log.Info("%s %s starting...", s.Name, s.Cluster)
	return s.server.Run(close)
}
