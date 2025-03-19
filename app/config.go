package app

type Config struct {
	Name    string
	Servers []Server
	Clients []Client
}

type Server struct {
	Name string
	Port string
	Ip   string
}

type Client struct {
	Name     string
	balancer Balancer
}

type Balancer struct {
	Name string
}
