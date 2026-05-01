//go:build sveltego

// Package src wires the cookiesession counter playground's server hooks.
package src

import (
	"github.com/binsarjr/sveltego/cookiesession"
	"github.com/binsarjr/sveltego/playgrounds/cookiesession-counter/src/routes"
)

// counterCodec is the process-wide codec for the counter session.
// In production code, load the key from an environment variable and
// never hard-code it. The 32-byte key here is for the playground only.
var counterCodec = must(cookiesession.NewCodec([]cookiesession.Secret{
	{ID: 1, Key: []byte("example-key-must-be-32-bytes!!!!")},
}))

// Handle installs the cookiesession middleware for the counter session.
var Handle = cookiesession.Handle[routes.CounterSession](counterCodec, "counter",
	cookiesession.WithHTTPOnly(true),
	cookiesession.WithSecure(false), // dev only — TLS not running in playground
	cookiesession.WithSameSite(0),   // defaults to Lax
)

func must[T any](v T, err error) T {
	if err != nil {
		panic("cookiesession-counter: " + err.Error())
	}
	return v
}
