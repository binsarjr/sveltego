module github.com/binsarjr/sveltego/playgrounds/blog

go 1.25

require (
	github.com/binsarjr/sveltego/packages/sveltego v0.0.0-00010101000000-000000000000
	github.com/microcosm-cc/bluemonday v1.0.27
	github.com/yuin/goldmark v1.7.8
)

require (
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	golang.org/x/net v0.26.0 // indirect
)

replace github.com/binsarjr/sveltego/packages/sveltego => ../../packages/sveltego
