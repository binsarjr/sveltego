package env

import (
	"os"
	"strings"
)

const publicPrefix = "PUBLIC_"

// StaticPrivate returns the value of key from the process environment
// and panics when key is unset. Use for required server-only secrets
// (DATABASE_URL, SESSION_KEY) whose absence should fail startup.
func StaticPrivate(key string) string {
	v, ok := os.LookupEnv(key)
	if !ok {
		panic("env: required key " + quote(key) + " is unset")
	}
	return v
}

// StaticPublic returns the value of key from the process environment
// and panics when key is unset or lacks the PUBLIC_ prefix. The
// prefix is enforced so accidental private values cannot reach the
// client bundle.
func StaticPublic(key string) string {
	if !strings.HasPrefix(key, publicPrefix) {
		panic("env: StaticPublic key " + quote(key) + " must have PUBLIC_ prefix")
	}
	v, ok := os.LookupEnv(key)
	if !ok {
		panic("env: required key " + quote(key) + " is unset")
	}
	return v
}

// DynamicPrivate returns the value of key from the process
// environment, or "" when unset. Use for runtime-toggleable
// server-only settings.
func DynamicPrivate(key string) string {
	return os.Getenv(key)
}

// DynamicPublic returns the value of key from the process
// environment, or "" when unset. Panics when key lacks the PUBLIC_
// prefix.
func DynamicPublic(key string) string {
	if !strings.HasPrefix(key, publicPrefix) {
		panic("env: DynamicPublic key " + quote(key) + " must have PUBLIC_ prefix")
	}
	return os.Getenv(key)
}

func quote(s string) string { return `"` + s + `"` }
