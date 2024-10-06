package main

import (
	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/service"
)

func main() {
	log.Fatal(service.New().Run(make(<-chan struct{})))
}

// curl http://localhost:8080
