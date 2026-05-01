---
title: Deploy
order: 85
summary: Single binary plus static assets — Docker, systemd, Cloudflare Workers.
---

# Deploy

A built sveltego app is one Go binary plus a `dist/` directory of static assets. Deploy as you would any Go HTTP server.

## Single binary

```sh
sveltego build
go build -o app ./cmd/app
./app
```

Set `PORT`, log level via `-v`/`-vv`/`-vvv`, and any environment your `Init` hook expects.

## Docker

```dockerfile
FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go install github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego@latest \
    && sveltego build \
    && go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/app

FROM gcr.io/distroless/base-debian12
COPY --from=build /out/app /app
COPY --from=build /src/dist /dist
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app"]
```

The image is small because the binary is statically linked and the Vite output is plain static files.

## systemd

```ini
[Unit]
Description=sveltego app
After=network.target

[Service]
ExecStart=/srv/app
Environment=PORT=8080
Restart=on-failure
User=app

[Install]
WantedBy=multi-user.target
```

## Cloudflare Workers

The Cloudflare Workers adapter is in scope (issue #92) but not yet implemented. Vercel and Netlify Functions adapters are explicitly out of scope; see [non-goals](/guide/faq#non-goals).

## Reverse proxy

Behind nginx or Caddy, terminate TLS at the proxy and forward to the Go binary. Honor `X-Forwarded-*` headers as you would any Go server. sveltego does not assume direct internet exposure.

## Static assets

The Vite output lives in `dist/`. Serve it with whatever static handler you prefer; `http.FileServer` works. Long-lived `Cache-Control` is safe because filenames are content-hashed.
