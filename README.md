# hostmux

**hostmux** is a small reverse proxy built around **host-based routing**: one local HTTPS listener fronts every app you run, so you stop juggling ad hoc ports and scattered reverse-proxy snippets. Point [**Cloudflare Tunnel**](https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/) (`cloudflared`) at that listener and you can share **real hostnames** (`app.example.com`, `api.example.com`) for local development, so teammates and other devices (e.g. phones for testing) hit the same URLs you do without checking in brittle port numbers or tunnel config per repo.

- **Single static binary** — self-hosted; one executable, no separate proxy stack to install
- **Host-based routing** — one HTTPS entrypoint; route many upstreams by the `Host` header
- **Cloudflare Tunnel** — works well with [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/) (`cloudflared`): point the tunnel at hostmux’s HTTPS listener and serve local development through Cloudflare’s edge and DNS
- **Ephemeral port per process** — hostmux picks a free TCP port, injects `PORT` into your dev command, registers hostnames, streams stdio, and tears down routes on exit (including crash or `kill -9`)
- **Git worktrees** — auto-prefix hostnames so parallel branches of the same repo don’t collide
- **Persistent routes** — optional TOML with hot reload when you prefer config-as-code

## Install

Download the latest binary from [GitHub Releases](https://github.com/Limetric/hostmux/releases/latest), or build from source:

```bash
git clone https://github.com/Limetric/hostmux.git
cd hostmux
go build -o build/hostmux .
```

The examples below assume `hostmux` is on your `PATH`.

## Quick Start

```sh
# Start the daemon with TLS enabled by default on :8443 (hostmux start --foreground to stay attached).
hostmux start

# Run a dev server and register a subdomain for it (starts the daemon in the background if it is not already running).
hostmux run --domain example.com --name myapp -- bun run dev

# Inspect the active routes.
hostmux routes

# Print the URL for a route without starting anything.
hostmux url --domain example.com --name myapp

# Stop the daemon.
hostmux stop
```

Tired of typing `--domain` on every line? Put `domain = "example.com"` in `~/.config/hostmux/hostmux.toml`, restart the daemon, and bare `--name` / `hostmux url` picks that up. Same hostnames, less copy-paste.

## More useful commands

```sh
# Multiple subdomains for the same upstream.
hostmux run --domain example.com --name app --name admin -- bun run dev

# Omit --name to infer from the nearest ancestor package.json name.
hostmux run --domain example.com -- bun run dev

# Full hostnames skip domain expansion, but still use the normal prefix logic unless --no-prefix is set.
hostmux run --name myapp.example.org -- bun run dev

# Print the final URL using the same domain/prefix logic as `run`.
hostmux url --domain example.com --name app
hostmux url --domain example.com --prefix feature-x --name app
hostmux url --domain example.com --name app --name admin
```

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

## Example config with persistent routes

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
hosts = ["admin", "myapp.example.org"]
upstream = "http://127.0.0.1:9000"
```

Run with `hostmux start --config /path/to/hostmux.toml`. The file is hot-reloaded on save.

## How it's built

Most of this codebase was written with LLM agents. The architecture, edge case handling, and test coverage reflect that. It runs in production and the integration test catches regressions, but you should know how it was made.

## License

Apache 2.0. See [LICENSE](LICENSE).
