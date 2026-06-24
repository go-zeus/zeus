module github.com/go-zeus/zeus/plugins/database/mysql

go 1.22

require (
	github.com/go-sql-driver/mysql v1.8.1
	github.com/go-zeus/zeus v0.0.0
)

require filippo.io/edwards25519 v1.1.0 // indirect

replace github.com/go-zeus/zeus => ../../..
