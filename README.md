# hostmux

`hostmux` is a small Go reverse proxy for host-based routing. It gives you one local entrypoint for many apps: you can keep long-lived routes in a TOML config, or use `hostmux run` to start a dev process, allocate a free port, register one or more subdomains or hostnames, and clean everything up automatically when the process exits.

It is designed to sit behind cloudflared (Argo Tunnel) or run standalone when you want a simple host router without maintaining per-app reverse proxy config.

## Quick Start

```sh
# Build from source.
go build -o build/hostmux .

# Start the daemon with TLS enabled by default on :8443.
./build/hostmux start

# In another terminal, run a dev server and register a subdomain for it.
./build/hostmux run --domain example.com --name myapp -- bun run dev

# Inspect the active routes.
./build/hostmux routes

# Print the URL for a route without starting anything.
./build/hostmux url --domain example.com --name myapp

# Stop the daemon.
./build/hostmux stop
```

More useful commands:

```sh
# Multiple subdomains for the same upstream.
./build/hostmux run --domain example.com --name app --name admin -- bun run dev

# Omit --name to infer from the nearest ancestor package.json name.
./build/hostmux run --domain example.com -- bun run dev

# Full hostnames skip domain expansion, but still use the normal prefix logic unless --no-prefix is set.
./build/hostmux run --name admin.other.test -- bun run dev

# Print the final URL using the same domain/prefix logic as `run`.
./build/hostmux url --domain example.com --name app
./build/hostmux url --domain example.com --prefix feature-x --name app
./build/hostmux url --domain example.com --name app --name admin

# Replace a running daemon (e.g. after rebuilding the binary or editing config).
./build/hostmux start --force

# Run the daemon in the foreground.
./build/hostmux start --foreground
```

`hostmux run` allocates a free TCP port, sets `PORT=<port>` in the child's environment, expands bare subdomains using `--domain` (or the daemon's configured `domain`), registers the resulting hostnames with the daemon, streams the child's stdio, and automatically deregisters when the child exits - even on crash or `kill -9`.

Pass one or more `--name` flags to register explicit bare labels, full hostnames, or IP literals:

```sh
./build/hostmux run --domain example.com --name app --name admin -- bun run dev
```

If you omit `--name`, `hostmux run` infers one name in this order:

1. The nearest ancestor `package.json` `name`, walking upward from the current working directory until the git repo root.
2. The git repo root directory basename.
3. The current working directory basename.

Inferred names are normalized to lowercase DNS-safe labels. Without `--domain`, bare names expand using the daemon's configured domain when a daemon is available (the config file defaults `domain` to `localhost` when the field is omitted). When neither `--domain` nor a daemon domain is available—for example no daemon is running—bare names pass through unchanged.

If the child process takes its own flags (arguments starting with `-` or `--`), keep a `--` separator before the child so hostmux does not parse them (for example `./build/hostmux run -- vite dev --host 0.0.0.0`).

`hostmux url` prints one `https://<hostname>` line per requested name using the same `--name`, `--domain`, `--prefix`, and `--no-prefix` resolution path as `hostmux run`. If `--name` is omitted, it uses the same inference order. If `--domain` is omitted and a daemon is available, it also reuses the daemon's configured domain for bare names.

## Behind cloudflared

By default, `hostmux start` launches the daemon and it listens on `:8443`, generates a self-signed certificate if needed, and stores it at `~/.hostmux/tls/hostmux.crt` and `~/.hostmux/tls/hostmux.key`.
If that managed certificate expires or you want a fresh one, remove `~/.hostmux/tls/` and restart `hostmux start`.

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
domain = "example.com"

[tls]
listen = ":8443"
# Optional: override the managed self-signed certificate paths.
# cert = "~/certs/hostmux.crt"
# key = "~/certs/hostmux.key"

[[app]]
hosts = ["api"]
upstream = "http://127.0.0.1:8080"

[[app]]
hosts = ["admin", "admin.other.test"]
upstream = "http://127.0.0.1:9000"
```

Bare `hosts` entries expand against the top-level `domain` (default `localhost` when omitted); entries that already contain a dot are treated as full hostnames and kept unchanged. An explicit `domain = ""` in TOML is treated the same as omitting `domain` and still defaults to `localhost`.

Run with `hostmux start --config /path/to/hostmux.toml`. The file is hot-reloaded on save.

If you already have an older config using top-level `listen = "..."`
without a `[tls]` block, that listen address still applies to the default TLS
listener.

## Worktrees

`hostmux run` auto-detects non-primary git worktrees and prepends the worktree name to the requested subdomain or hostname so two checkouts of the same project don't collide:

| cwd | command | actual hostnames |
|---|---|---|
| `~/proj/main` (primary) | `hostmux run --domain example.com --name myapp -- ...` | `myapp.example.com` |
| `~/proj/feature-x` (worktree) | `hostmux run --domain example.com --name myapp -- ...` | `feature-x-myapp.example.com` |

Override with `--prefix NAME` or disable with `--no-prefix`.

## How it's built

Most of this codebase was written with LLM agents. The architecture, edge case handling, and test coverage reflect that. It runs in production and the integration test catches regressions, but you should know how it was made.

## License

Apache 2.0. See [LICENSE](LICENSE).
