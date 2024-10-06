package main

import (
	"github.com/go-yml/yaml"
	"github.com/go-zeus/zeus/log"
	"os"
)

type App struct {
	Name    string
	Servers map[string]Server
	Clients map[string]Client
}

type Server struct {
	Name string
	Ip   string
	Port int
}

type Client struct {
	Name string
}

func main() {
	config := &App{}
	data, err := os.ReadFile("./app.yml")
	if err != nil {
		log.Fatal(err)
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Fatal(err, "===")
	}
	log.Info("%+v", config)
}
