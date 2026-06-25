module github.com/go-zeus/zeus/plugins/database/postgres

go 1.22

require (
	github.com/go-zeus/zeus v0.0.0
	github.com/jackc/pgx/v5 v5.6.0
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	golang.org/x/crypto v0.23.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/text v0.15.0 // indirect
)

replace github.com/go-zeus/zeus => ../../..
