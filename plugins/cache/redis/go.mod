module github.com/go-zeus/zeus/plugins/cache/redis

go 1.22

require (
	github.com/alicebob/miniredis/v2 v2.38.0
	github.com/go-zeus/zeus v0.0.0
	github.com/redis/go-redis/v9 v9.6.1
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
)

replace github.com/go-zeus/zeus => ../../..
