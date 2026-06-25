package main

import (
	"github.com/go-zeus/zeus/components"
	logslog "github.com/go-zeus/zeus/log/slog"
	"github.com/go-zeus/zeus/registry/memory"
)

func main() {
	app := components.NewApp(
		components.NewLogComponent(logslog.NewSlog()),
		components.NewRegistryComponent(memory.New()),
		components.NewServerComponent(),
		components.NewServiceComponent(),
	)
	app.Run()
}

// curl http://localhost:8080
// curl http://localhost:8080/health
