# adapter-docker

Deploy adapter that emits a multi-stage `Dockerfile` and `.dockerignore` next to the user's project. The runtime image is `gcr.io/distroless/static-debian12:nonroot` (~2MB, no shell).

## Usage (programmatic)

```go
import adapterdocker "github.com/binsarjr/sveltego/adapter-docker"

err := adapterdocker.Build(ctx, adapterdocker.BuildContext{
    OutputDir:   "./dist",
    BinaryName:  "myapp",
    MainPackage: "./cmd/myapp",
    Port:        8080,
})
```

## Usage (CLI)

```sh
sveltego-adapter build --target=docker --out . --port 8080
docker build -t myapp:latest .
docker run --rm -p 8080:8080 myapp:latest
```

The generated `HEALTHCHECK` invokes the binary with `--healthcheck`. Either implement that flag in `main.go` or replace the directive with one that hits `/healthz` over HTTP — the framework does not generate a `/healthz` route. Add it via `src/routes/healthz/server.go` if you want one.

Status: pre-alpha. See repo root [`README.md`](../../README.md) and [`STABILITY.md`](./STABILITY.md).
