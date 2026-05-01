// Package version exposes the build-time version string for the sveltego CLI.
package version

// Version is the build-time version string. Override via -ldflags
// "-X github.com/binsarjr/sveltego/packages/sveltego/internal/version.Version=$(git describe ...)".
var Version = "dev"
