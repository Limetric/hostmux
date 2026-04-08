# hostmux

`hostmux` is a small Go reverse proxy for hostname-based routing. It gives you one local entrypoint for many apps: you can keep long-lived routes in a TOML config, or use `hostmux run` to start a dev process, allocate a free port, register one or more hostnames, and clean everything up automatically when the process exits.

It is designed to sit behind cloudflared (Argo Tunnel) or run standalone when you want a simple host router without maintaining per-app reverse proxy config.

## Quick Start

```sh
# Build from source.
go build -o build/hostmux .

# Start the daemon with TLS enabled by default on :8443.
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

By default, `hostmux serve` listens on `:8443`, generates a self-signed certificate if needed, and stores it at `~/.hostmux/tls/hostmux.crt` and `~/.hostmux/tls/hostmux.key`.

If you want HTTP/2 multiplexing on the tunnel-to-origin hop, point `cloudflared` at the default HTTPS listener:

```yaml
ingress:
  - hostname: "*.example.com"
    service: https://127.0.0.1:8443
    originRequest:
      http2Origin: true
      noTLSVerify: true
```

`cloudflared` requires an HTTPS origin for `http2Origin: true`, so the old h2c-only standalone path is not enough for tunnel-to-origin HTTP/2.

## Persistent routes

Create a TOML config (default: `~/.config/hostmux/hostmux.toml`):

```toml
[tls]
listen = ":8443"
# Optional: override the managed self-signed certificate paths.
# cert = "~/certs/hostmux.crt"
# key = "~/certs/hostmux.key"

[[app]]
hosts = ["api.local"]
upstream = "http://127.0.0.1:8080"

[[app]]
hosts = ["admin.local", "admin.example.com"]
upstream = "http://127.0.0.1:9000"
```

Run with `hostmux serve --config /path/to/hostmux.toml`. The file is hot-reloaded on save.

If you already have an older config using top-level `listen = "..."`
without a `[tls]` block, that listen address still applies to the default TLS
listener.

## Worktrees

`hostmux run` auto-detects non-primary git worktrees and prepends the worktree name as a subdomain so two checkouts of the same project don't collide:

| cwd | command | actual hostnames |
|---|---|---|
| `~/proj/main` (primary) | `hostmux run myapp.test -- ...` | `myapp.test` |
| `~/proj/feature-x` (worktree) | `hostmux run myapp.test -- ...` | `feature-x.myapp.test` |

Override with `--prefix NAME` or disable with `--no-prefix`.

## How it's built

Most of this codebase was written with LLM agents. The architecture, edge case handling, and test coverage reflect that. It runs in production and the integration test catches regressions, but you should know how it was made.

## License

Apache 2.0. See [LICENSE](LICENSE).
