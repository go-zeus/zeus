module github.com/go-zeus/zeus/examples/17-job-cron

go 1.22

require (
	github.com/go-zeus/zeus v0.0.0
	github.com/go-zeus/zeus/plugins/job/cron v0.0.0-00010101000000-000000000000
)

require github.com/robfig/cron/v3 v3.0.1 // indirect

replace (
	github.com/go-zeus/zeus => ../..
	github.com/go-zeus/zeus/plugins/job/cron => ../../plugins/job/cron
)
