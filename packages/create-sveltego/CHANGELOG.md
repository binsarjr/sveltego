# Changelog — create-sveltego

All notable changes to this package will be documented in this file. Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

### Features

- npm wrapper that exec's `sveltego-init` for `npm create sveltego@latest`
  with a `go run @latest` fallback when no release binary is available
  (#374).
