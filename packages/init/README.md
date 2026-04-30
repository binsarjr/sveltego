# init

Standalone scaffolder for fresh sveltego projects.

Status: pre-alpha. The `sveltego-init` binary writes a baseline project
tree (src/routes, src/lib, hooks, sveltego.config, go.mod, README) and
optionally copies the AI-assistant templates from
[`templates/ai`](../../templates/ai). Closes
[#131](https://github.com/binsarjr/sveltego/issues/131).

## Build

```sh
go build -o sveltego-init ./cmd/sveltego-init
```

## Use

```sh
sveltego-init ./my-app                # baseline scaffold, prompts on TTY for --ai
sveltego-init --ai ./my-app           # baseline + AI templates, no prompt
sveltego-init --non-interactive ./my-app   # piped/CI: skip prompt, default --ai=false
sveltego-init --force ./my-app        # overwrite existing files
sveltego-init --module example.com/my-app ./my-app
```

Conflicts skip-by-default; the binary prints the list of skipped paths.
Re-run with `--force` to overwrite.

## Layout

- `cmd/sveltego-init/` — binary entry point.
- `internal/scaffold/` — file writer, baseline templates, AI-template copy.

## STABILITY

The `--ai`, `--force`, `--non-interactive`, and `--module` flags are the
public CLI surface. The internal scaffold package is internal-only. AI
templates are sourced from `github.com/binsarjr/sveltego/templates/ai`
and validated byte-equal against `embed.FS` in tests.
