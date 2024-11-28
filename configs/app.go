package configs

type App struct {
	Name    string
	servers []Server
	clients []Client
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
