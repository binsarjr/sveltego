# create-sveltego

Zero-clone scaffold launcher for sveltego. Run from any terminal with Node >= 20:

```sh
npm create sveltego@latest ./hello
# or, for Go users:
go run github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@latest ./hello
```

`npm create sveltego@latest` is a thin wrapper that delegates to the canonical
`sveltego-init` Go binary. It tries, in order:

1. A cached binary under `~/.cache/sveltego/` (or `$XDG_CACHE_HOME/sveltego/`).
2. A platform-matched release asset from GitHub (pending [#368](https://github.com/binsarjr/sveltego/issues/368) — currently always falls through).
3. `go run github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@latest`, when `go` is on `PATH`.

If none of the three apply the wrapper exits non-zero with a single-line message
pointing the user at either Go install or the release-binary issue.

Status: pre-alpha. See [`STABILITY.md`](./STABILITY.md) for the API tier.

## Flags

The wrapper passes every flag through to `sveltego-init` unchanged, so the flag
surface is identical to [packages/init](../init/README.md):

```sh
npm create sveltego@latest --ai ./hello
npm create sveltego@latest --tailwind=v4 ./hello
npm create sveltego@latest --service-worker ./hello
npm create sveltego@latest --module example.com/x ./hello
```

Wrapper-only flags / env vars:

| Flag | Env | Effect |
|---|---|---|
| `--no-binary-download` | `SVELTEGO_NO_BINARY_DOWNLOAD=1` | Skip cache + release-binary lookup; jump straight to the `go run @latest` fallback. CI uses this. |
| | `SVELTEGO_INIT_LOCAL_PATH=<file>` | Run the named binary directly. CI escape hatch — point at a freshly built `sveltego-init`. |
| | `SVELTEGO_VERSION=<v>` | Override the version slug used in the cache filename. Defaults to `latest`. |
| | `SVELTEGO_CACHE_DIR=<dir>` | Override `~/.cache/sveltego`. |

## Develop

```sh
npm install                       # install dev deps (typescript, @types/node)
npm run build                     # tsc → dist/
npm test                          # node --test test/*.test.mjs
node bin/create-sveltego.js ...   # invoke locally
```

`dist/` is gitignored and produced by `npm run build`. The published tarball
ships `bin/`, `dist/`, and the markdown files only.
