module github.com/binsarjr/sveltego/playgrounds/dashboard

go 1.22

require (
	github.com/binsarjr/sveltego/packages/sveltego v0.0.0-00010101000000-000000000000
	golang.org/x/crypto v0.31.0
)

replace github.com/binsarjr/sveltego/packages/sveltego => ../../packages/sveltego

require golang.org/x/sys v0.28.0 // indirect
