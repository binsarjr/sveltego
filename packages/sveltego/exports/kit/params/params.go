package params

import "github.com/binsarjr/sveltego/runtime/router"

// DefaultMatchers returns a fresh [router.Matchers] map seeded with the
// built-in matchers (int, uuid, slug). The returned map is owned by the
// caller; mutating it does not affect subsequent calls.
func DefaultMatchers() router.Matchers {
	return router.Matchers{
		"int":  Int,
		"uuid": UUID,
		"slug": Slug,
	}
}
