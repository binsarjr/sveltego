package codegen

import (
	"fmt"
	"regexp"
	"strconv"
)

// staticPublicEnvRe matches env.StaticPublic("KEY") call expressions in
// raw .svelte source. The match is deliberately text-level so we can
// rewrite the call to a Go string literal before the source reaches the
// parser, baking the value into the binary at build time.
//
// Only StaticPublic is substituted here. env.StaticPrivate in templates
// is rejected by the private-leak guard (checkPrivateEnv / validateExpr)
// because even if inlined as a literal, the value would appear in
// server-rendered HTML that is delivered to the browser.
//
// Capture group: [1] = the key string without quotes.
var staticPublicEnvRe = regexp.MustCompile(
	`\benv\.StaticPublic\("([^"]+)"\)`,
)

// EnvLookup resolves an environment variable by name. The second return
// is true when the variable is present (possibly with an empty value).
// The caller supplies os.LookupEnv in production and a stub in tests.
type EnvLookup func(key string) (string, bool)

// substituteStaticEnv rewrites every env.StaticPublic("X") call in body
// to the Go string literal for the value returned by lookup at build time.
// A call whose key is unset in the build environment is a fatal error —
// StaticPublic panics at runtime when its key is unset, and the build-time
// substitute requires the value to be known.
//
// env.StaticPrivate calls are NOT substituted; they are instead rejected
// by the downstream private-leak guard (checkPrivateEnv) because the
// value would appear in SSR HTML visible to clients.
//
// The function operates on the raw .svelte source text before parsing,
// analogous to rewriteLibImports for $lib paths.
func substituteStaticEnv(body string, lookup EnvLookup) (string, error) {
	if !staticPublicEnvRe.MatchString(body) {
		return body, nil
	}
	var substErr error
	out := staticPublicEnvRe.ReplaceAllStringFunc(body, func(match string) string {
		if substErr != nil {
			return match
		}
		sub := staticPublicEnvRe.FindStringSubmatch(match)
		key := sub[1]
		val, ok := lookup(key)
		if !ok {
			substErr = fmt.Errorf("codegen: env.StaticPublic(%q): key is unset in build environment", key)
			return match
		}
		return strconv.Quote(val)
	})
	if substErr != nil {
		return "", substErr
	}
	return out, nil
}
