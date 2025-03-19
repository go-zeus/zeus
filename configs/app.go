package configs

import (
	"github.com/go-zeus/zeus/components"
	"github.com/go-zeus/zeus/server"
)

type App struct {
	Name       string
	Service    server.Server
	components map[string]*components.Component
}
