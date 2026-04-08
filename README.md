# hostmux

A Go reverse proxy that routes by hostname. Designed to sit behind cloudflared (Argo Tunnel) or run standalone.

## Quickstart

```sh
# Start the daemon (or just run hostmux run — it auto-spawns).
hostmux serve

# Run a dev server, register a hostname for it.
hostmux run myapp.test -- bun run dev

# Multiple hostnames for the same upstream.
hostmux run a.test,b.test -- bun run dev

# What's currently registered?
hostmux list
```

`hostmux run` allocates a free TCP port, sets `PORT=<port>` in the child's environment, registers the hostname(s) with the daemon, streams the child's stdio, and automatically deregisters when the child exits — even on crash or `kill -9`.

## Behind cloudflared

For HTTP/2 multiplexing on the tunnel-to-origin hop, set `http2Origin: true` in your cloudflared config:

```yaml
ingress:
  - hostname: "*.example.com"
    service: http://127.0.0.1:8080
    originRequest:
      http2Origin: true
```

hostmux speaks HTTP/2 cleartext (h2c) on the same port as HTTP/1.1, so cloudflared can negotiate HTTP/2 without certificates.

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
