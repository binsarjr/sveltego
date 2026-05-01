module github.com/binsarjr/sveltego/adapter-auto

go 1.25

require (
	github.com/binsarjr/sveltego/adapter-cloudflare v0.0.0-00010101000000-000000000000
	github.com/binsarjr/sveltego/adapter-docker v0.0.0-00010101000000-000000000000
	github.com/binsarjr/sveltego/adapter-lambda v0.0.0-00010101000000-000000000000
	github.com/binsarjr/sveltego/adapter-server v0.0.0-00010101000000-000000000000
	github.com/binsarjr/sveltego/adapter-static v0.0.0-00010101000000-000000000000
)

replace (
	github.com/binsarjr/sveltego/adapter-cloudflare => ../adapter-cloudflare
	github.com/binsarjr/sveltego/adapter-docker => ../adapter-docker
	github.com/binsarjr/sveltego/adapter-lambda => ../adapter-lambda
	github.com/binsarjr/sveltego/adapter-server => ../adapter-server
	github.com/binsarjr/sveltego/adapter-static => ../adapter-static
)
