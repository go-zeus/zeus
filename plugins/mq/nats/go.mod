module github.com/go-zeus/zeus/plugins/mq/nats

go 1.22

require (
	github.com/go-zeus/zeus v0.0.0
	github.com/nats-io/nats.go v1.36.0
)

require (
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/nats-io/nkeys v0.4.7 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.23.0 // indirect
	golang.org/x/sys v0.20.0 // indirect
)

replace github.com/go-zeus/zeus => ../../..
