module github.com/snack8310/gmp/management

go 1.24.0

require github.com/go-sql-driver/mysql v1.10.1

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/snack8310/gmp/gmp-core v0.0.0
)

replace github.com/snack8310/gmp/gmp-core => ../gmp-core
