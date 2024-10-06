package main

import (
	"github.com/go-zeus/zeus/app"
	"github.com/go-zeus/zeus/log"
)

func main() {
	log.Fatal(app.New().Run(make(chan struct{})))
}

// curl http://localhost:8080
