# hostmux

`hostmux` is a small Go reverse proxy for hostname-based routing. It gives you one local entrypoint for many apps: you can keep long-lived routes in a TOML config, or use `hostmux run` to start a dev process, allocate a free port, register one or more hostnames, and clean everything up automatically when the process exits.

It is designed to sit behind cloudflared (Argo Tunnel) or run standalone when you want a simple host router without maintaining per-app reverse proxy config.

## Quick Start

```sh
# Build from source.
go build -o build/hostmux .

# Start the daemon on :8080.
./build/hostmux serve

# In another terminal, run a dev server and register a hostname for it.
./build/hostmux run myapp.test -- bun run dev

# Inspect the active routes.
./build/hostmux list

# Stop the daemon.
./build/hostmux stop
```

More useful commands:

```sh
# Multiple hostnames for the same upstream.
./build/hostmux run a.test,b.test -- bun run dev

# Replace a running daemon (e.g. after rebuilding the binary or editing config).
./build/hostmux serve --force
```

`hostmux run` allocates a free TCP port, sets `PORT=<port>` in the child's environment, registers the hostname(s) with the daemon, streams the child's stdio, and automatically deregisters when the child exits — even on crash or `kill -9`.

## Behind cloudflared

`hostmux` can sit behind `cloudflared` on its plain listener:

```yaml
ingress:
  - hostname: "*.example.com"
    service: http://127.0.0.1:8080
```

That origin hop will be HTTP/1.1. Cloudflare's `http2Origin: true` setting requires an HTTPS origin, so h2c on the plain listener is not enough for tunnel-to-origin HTTP/2.

If you want HTTP/2 multiplexing on the tunnel-to-origin hop, enable hostmux's TLS listener and point `cloudflared` at `https://...` instead:

```yaml
ingress:
  - hostname: "*.example.com"
    service: https://127.0.0.1:8443
    originRequest:
      http2Origin: true
      noTLSVerify: true
```

The plain listener still speaks HTTP/1.1 + h2c on the same port for direct local clients that can use cleartext HTTP/2.

## Persistent routes

Create a TOML config (default: `~/.config/hostmux/hostmux.toml`):

```toml
listen = ":8080"

[[app]]
hosts = ["api.local"]
upstream = "http://127.0.0.1:8080"

[[app]]
hosts = ["admin.local", "admin.example.com"]
upstream = "http://127.0.0.1:9000"
```

Run with `hostmux serve --config /path/to/hostmux.toml`. The file is hot-reloaded on save.

## Example TLS Config

If you want hostmux to terminate TLS directly, add a `[tls]` block alongside the plain HTTP listener:

```toml
listen = ":8080"

[tls]
listen = ":8443"
cert = "/Users/alice/certs/dev-cert.pem"
key = "/Users/alice/certs/dev-key.pem"

[[app]]
hosts = ["app.local"]
upstream = "http://127.0.0.1:3000"

[[app]]
hosts = ["api.local"]
upstream = "http://127.0.0.1:4000"
```

Run it with `hostmux serve --config /path/to/hostmux.toml`. `listen` serves plain HTTP/1.1 + h2c, while `tls.listen` serves HTTPS and negotiates HTTP/2 via ALPN.

## Worktrees

`hostmux run` auto-detects non-primary git worktrees and prepends the worktree name to the hostname so two checkouts of the same project don't collide:

| cwd | command | actual hostnames |
|---|---|---|
| `~/proj/main` (primary) | `hostmux run myapp.test -- ...` | `myapp.test` |
| `~/proj/feature-x` (worktree) | `hostmux run myapp.test -- ...` | `feature-x-myapp.test` |

Override with `--prefix NAME` or disable with `--no-prefix`.

## How it's built

Most of this codebase was written with LLM agents. The architecture, edge case handling, and test coverage reflect that. It runs in production and the integration test catches regressions, but you should know how it was made.

## License

Apache 2.0. See [LICENSE](LICENSE).
