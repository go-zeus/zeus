module github.com/go-zeus/zeus/plugins/log/zap

go 1.22

require (
	github.com/go-zeus/zeus v0.0.0
	go.uber.org/zap v1.28.0
)

require go.uber.org/multierr v1.11.0 // indirect

replace github.com/go-zeus/zeus => ../../..
