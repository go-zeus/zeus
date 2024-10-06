package main

import (
	"github.com/go-zeus/zeus/log"
	"github.com/go-zeus/zeus/server"
	"github.com/go-zeus/zeus/service"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/hi", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test server"))
	})
	ser := server.New(server.Mux(mux))

	done := make(<-chan struct{})
	log.Fatal(service.New(service.Server(ser)).Run(done))
}

// curl http://localhost:8080/hi
