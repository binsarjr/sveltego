# init

Standalone scaffolder for fresh sveltego projects.

Status: pre-alpha. The `sveltego-init` binary writes a baseline project
tree (`src/routes`, `src/lib`, `hooks.server.go`, `sveltego.config.go`,
`cmd/app/main.go`, `app.html`, `package.json`, `vite.config.js`,
`go.mod`, `README.md`) and optionally copies the AI-assistant templates
mirrored under `internal/aitemplates/files`. Closes
[#131](https://github.com/binsarjr/sveltego/issues/131).

## Run via `go run @latest` (canonical, zero clone)

```sh
go run github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@latest ./my-app
```

The Go module proxy resolves the binary, runs it, and discards the
checkout — no clone, no `go install` ceremony. All flags below pass
through unchanged.

## Build from source

```sh
go build -o sveltego-init ./cmd/sveltego-init
```

## Flags

```sh
sveltego-init ./my-app                       # baseline; prompts on TTY for --ai
sveltego-init --ai ./my-app                  # baseline + AI templates, no prompt
sveltego-init --non-interactive ./my-app     # piped / CI: skip prompt, default --ai=false
sveltego-init --tailwind=v4 ./my-app         # bare flag also accepts =v3 or =none
sveltego-init --service-worker ./my-app      # emit src/service-worker.ts (#89)
sveltego-init --module example.com/x ./my-app
sveltego-init --force ./my-app               # overwrite existing files
```

Conflicts skip-by-default; the binary prints the list of skipped paths.
Re-run with `--force` to overwrite.

## Layout

- `cmd/sveltego-init/` — binary entry point.
- `internal/scaffold/` — file writer, baseline templates, AI-template copy.
- `internal/aitemplates/` — embedded mirror of `templates/ai/` with a
  drift test guard. Living here keeps `packages/init` self-contained so
  `go run @latest` resolves with no `replace` directive.

## STABILITY

The `--ai`, `--force`, `--non-interactive`, `--module`, `--tailwind`,
and `--service-worker` flags are the public CLI surface. The internal
scaffold and aitemplates packages are internal-only. The canonical,
human-edited copies of the AI templates still live at
[`templates/ai`](../../templates/ai); a drift test in
`internal/aitemplates` keeps the two trees byte-equal.
