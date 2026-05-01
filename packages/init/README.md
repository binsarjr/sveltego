# init

Standalone scaffolder for fresh sveltego projects.

Status: pre-alpha. The `sveltego-init` binary writes a baseline project
tree (src/routes, src/lib, hooks, sveltego.config, go.mod, README) and
optionally copies the AI-assistant templates embedded under
[`internal/aitemplates`](./internal/aitemplates). Closes
[#131](https://github.com/binsarjr/sveltego/issues/131).

The npm wrapper [`create-sveltego`](../create-sveltego) drives this same
binary for users who prefer `npm create sveltego@latest ./my-app` over
`go run @latest`. Both paths produce identical project trees.

## Use (no clone)

```sh
# npm (Node >= 20). Falls back to `go run @latest` if no release binary.
npm create sveltego@latest ./my-app

# Go-only (Go >= 1.23). Resolves via the Go module proxy.
go run github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@latest ./my-app
```

## Use (from-source build)

```sh
go build -o sveltego-init ./cmd/sveltego-init
sveltego-init ./my-app                # baseline scaffold, prompts on TTY for --ai
sveltego-init --ai ./my-app           # baseline + AI templates, no prompt
sveltego-init --non-interactive ./my-app   # piped/CI: skip prompt, default --ai=false
sveltego-init --force ./my-app        # overwrite existing files
sveltego-init --module example.com/my-app ./my-app
sveltego-init --tailwind=v4 ./my-app  # opt into Tailwind (v4 default, v3 legacy, none)
sveltego-init --service-worker ./my-app    # emit a starter src/service-worker.ts
```

Conflicts skip-by-default; the binary prints the list of skipped paths.
Re-run with `--force` to overwrite.

## Layout

- `cmd/sveltego-init/` — binary entry point.
- `internal/scaffold/` — file writer, baseline templates, AI-template copy.
- `internal/aitemplates/` — embedded AI-assistant rule files. Mirrored
  from `templates/ai/` at the repo root and verified byte-equal in tests
  so the embed and the canonical source never drift.

## STABILITY

The `--ai`, `--force`, `--non-interactive`, `--module`, `--tailwind`, and
`--service-worker` flags are the public CLI surface. The internal
scaffold and aitemplates packages are internal-only.
