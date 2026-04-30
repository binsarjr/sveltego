# adapter-server

Reference deploy adapter. Produces a standalone HTTP server binary — the default sveltego target.

## Usage (programmatic)

```go
import adapterserver "github.com/binsarjr/sveltego/adapter-server"

err := adapterserver.Build(ctx, adapterserver.BuildContext{
    BinaryPath: "/path/to/built/binary",
    OutputDir:  "./dist",
    AssetsDir:  "./public",      // optional
    BinaryName: "myapp",         // defaults to "sveltego"
})
```

## Usage (CLI)

```sh
sveltego-adapter build --target=server --binary ./app --out ./dist
```

The binary is single-file, statically linked, and runs on any host that supports the GOOS/GOARCH it was compiled for. No external runtime.

Status: pre-alpha. See repo root [`README.md`](../../README.md) and [`STABILITY.md`](./STABILITY.md).
