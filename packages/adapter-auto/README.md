# adapter-auto

Dispatcher that auto-detects the deploy target from the environment and forwards to the matching per-target adapter. Also hosts the `sveltego-adapter` standalone CLI.

## Detection rules

| Signal                              | Target       |
| ----------------------------------- | ------------ |
| `SVELTEGO_ADAPTER` set              | (verbatim)   |
| `AWS_LAMBDA_RUNTIME_API` set        | `lambda`     |
| `CF_PAGES` set                      | `cloudflare` |
| (otherwise)                         | `server`     |

Override at any time by setting `SVELTEGO_ADAPTER`.

## Usage (programmatic)

```go
import adapterauto "github.com/binsarjr/sveltego/adapter-auto"

// Auto-detect:
err := adapterauto.Build(ctx, adapterauto.BuildContext{
    BinaryPath: "./app",
    OutputDir:  "./dist",
})

// Or explicit:
err := adapterauto.Build(ctx, adapterauto.BuildContext{
    Target:     "lambda",
    ProjectRoot: ".",
    ModulePath: "github.com/me/app",
})
```

## CLI

The `sveltego-adapter` binary lives at `./cmd/sveltego-adapter/` and drives every target until the main `sveltego` CLI grows a `--target` flag (Phase 0ee follow-up):

```sh
go install github.com/binsarjr/sveltego/adapter-auto/cmd/sveltego-adapter@latest

sveltego-adapter targets
sveltego-adapter doc   --target=docker
sveltego-adapter build --target=server --binary ./app --out ./dist
```

## Targets

| Name         | Status     | Notes                                                     |
| ------------ | ---------- | --------------------------------------------------------- |
| `server`     | shipped    | Single binary, the default                                |
| `docker`     | shipped    | Multi-stage Dockerfile + `.dockerignore` + healthcheck    |
| `lambda`     | shipped    | API Gateway HTTP API → Lambda via aws-lambda-go-api-proxy |
| `static`     | stub (#65) | Blocked on prerender mode                                 |
| `cloudflare` | stub       | Workers Go runtime too restricted                         |

Status: pre-alpha. See repo root [`README.md`](../../README.md) and [`STABILITY.md`](./STABILITY.md).
