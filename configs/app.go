package configs

import (
	"github.com/go-zeus/zeus/client"
	"github.com/go-zeus/zeus/server"
)

type App struct {
	Name    string
	servers []server.Server
	clients []client.Client
}
