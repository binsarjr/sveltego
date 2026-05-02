# Constrained-resource SSR runtime for bench/ssr-constrained.
#
# Pre-built `app` binary and `app.html` are baked in by run.sh — see that
# script for the host-side `sveltego compile` + cross-compile chain. The
# image stays minimal (distroless static, nonroot) so per-container memory
# is dominated by Go runtime, not the OS.

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /srv

COPY app /srv/app
COPY app.html /srv/app.html

EXPOSE 3000
USER nonroot:nonroot
ENTRYPOINT ["/srv/app"]
