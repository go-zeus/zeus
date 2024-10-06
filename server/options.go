package server

import (
	"net/http"
)

func Ip(ip string) Option {
	return func(s *server) {
		s.ip = ip
	}
}

func Port(port int) Option {
	return func(s *server) {
		s.port = port
	}
}

func Mux(h http.Handler) Option {
	return func(s *server) {
		s.Handler = h
	}
}

func DefaultHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("welcome to zeus!"))
	})
}
