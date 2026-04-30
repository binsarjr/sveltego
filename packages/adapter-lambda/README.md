# adapter-lambda

Deploy adapter targeting AWS Lambda. Emits a generated `main.go` under `<root>/.gen/lambda/` that wraps the user's HTTP handler with [`aws-lambda-go-api-proxy/httpadapter`](https://github.com/awslabs/aws-lambda-go-api-proxy), plus a SAM `template.yaml` stub.

## Dependencies

The adapter itself does NOT pull AWS modules. Add them to your project's `go.mod` before building the lambda artefact:

```sh
go get github.com/aws/aws-lambda-go
go get github.com/awslabs/aws-lambda-go-api-proxy
```

## Usage (programmatic)

```go
import adapterlambda "github.com/binsarjr/sveltego/adapter-lambda"

err := adapterlambda.Build(ctx, adapterlambda.BuildContext{
    ProjectRoot:    ".",
    ModulePath:     "github.com/me/myapp",
    HandlerName:    "MyApp",
    MemoryMB:       1024,
    TimeoutSeconds: 30,
})
```

## Usage (CLI)

```sh
sveltego-adapter build --target=lambda --module github.com/me/myapp --root .
cd .gen/lambda
GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o bootstrap .
zip lambda.zip bootstrap
sam deploy --template-file template.yaml --stack-name myapp --resolve-s3
```

The wrapper expects the user's runtime to expose the routes/hooks generators on the `<ModulePath>/.gen` package — the same shape the `sveltego` build emits today. Adjust the generated `main.go` if your handler lives elsewhere.

Status: pre-alpha. See repo root [`README.md`](../../README.md) and [`STABILITY.md`](./STABILITY.md).
