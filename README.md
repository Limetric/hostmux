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

# Inspect the active routes (add --wide for age/pid/labels/command, --json for scripts).
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

# Hold the URL until the dev server is actually accepting requests (avoids a transient 502).
hostmux run --wait --name app -- bun run dev
hostmux run --wait-url /healthz --wait-timeout 60s --name api -- go run ./cmd/api

# Print the final URL using the same domain/prefix logic as `run`.
hostmux url --domain example.com --name app
hostmux url --domain example.com --prefix feature-x --name app
hostmux url --domain example.com --name app --name admin
```

## Routing an app you started yourself

`hostmux run` owns the child process, but some dev servers are managed
elsewhere (an IDE, Docker Compose, a Procfile, a watcher). Use `expose` to
route to an already-running upstream without handing hostmux the process
lifecycle:

```sh
hostmux expose --name api --upstream http://127.0.0.1:3000
hostmux expose --domain example.com --name admin --upstream http://127.0.0.1:9000
hostmux unexpose api
```

Exposed routes persist until you `unexpose` them (or the daemon restarts),
appear in `hostmux routes` under a `manual:NAME` source, and accept the same
`--domain` / `--label` options as `run`. The first `--name` is the route's
identifier for `unexpose`.

## Behind cloudflared

By default, `hostmux start` launches the daemon and it listens on `:8443`, generates a self-signed certificate if needed, and stores it at `~/.hostmux/tls/hostmux.crt` and `~/.hostmux/tls/hostmux.key`.
If that managed certificate expires or you want a fresh one, remove `~/.hostmux/tls/` and restart `hostmux start`.

Generate a matching ingress snippet for your config with `hostmux cloudflare config`
(reads the config file, and live daemon info when reachable):

```sh
hostmux cloudflare config            # prints the ingress block below
hostmux cloudflare config --domain example.com
```

If you want HTTP/2 multiplexing on the tunnel-to-origin hop, point `cloudflared` at the default HTTPS listener:

```yaml
ingress:
  - hostname: "*.example.com"
    service: https://127.0.0.1:8443
    originRequest:
      http2Origin: true
      noTLSVerify: true
```

By default `hostmux url` and `hostmux run` print URLs that include the
real listener port — `https://api.example.com:8443` — which is wrong when
cloudflared terminates the public connection on standard HTTPS and the
public URL has no visible port. Set `hide_port = true` in the daemon
config to drop the port from those printed URLs while the daemon keeps
listening on the unprivileged `:8443`:

```toml
listen = ":8443"
domain = "example.com"
hide_port = true
```

`hostmux url --no-prefix api` then prints `https://api.example.com`.

## Trusting the dev certificate

Browsers warn on hostmux's self-signed certificate until it is trusted by the
OS. Install it once:

```sh
hostmux trust      # add the managed cert to the OS trust store
hostmux untrust    # remove it again
```

Supported on macOS (`security`), Linux (`update-ca-*`, may prompt for sudo),
and Windows (`certutil -user Root`). `trust` is idempotent — it exits 0 if the
cert is already trusted unless you pass `--force`. Restart open browser tabs
after trusting.

To trust automatically on daemon start, set `auto_trust = true` under `[tls]`
or export `HOSTMUX_TLS_AUTO_TRUST=1`. It is off by default to avoid surprising
elevation prompts.

> Note: hostmux currently trusts the self-signed leaf certificate directly. A
> local-CA model (so renewals don't require re-trusting) is planned.

## HTTPS on port 443

Browsers treat `https://app.localhost` as port **443** by default. Hostmux
ships with `listen = ":8443"` because binding `:443` usually requires
extra privileges — but that means users have to type
`https://app.localhost:8443`, which hurts the `*.localhost` workflow.
If you want port-less URLs that match browser defaults, configure hostmux
to listen on `:443` using one of the patterns below.

When you do this, `hostmux url` and `hostmux run` automatically print
URLs **without** the `:443` suffix, matching the browser's address bar.
Stay on `:8443` and they include `:8443` so the URL is still clickable.

### Linux

The lightest touch is the `CAP_NET_BIND_SERVICE` capability. Grant it to
the binary once; after that hostmux binds `:443` unprivileged:

```sh
sudo setcap cap_net_bind_service=+ep /path/to/hostmux
```

If you re-build or upgrade, re-run `setcap` — capabilities are attached
to inodes, not paths.

For systemd-managed installs, add the capability via unit config instead:

```ini
[Service]
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
```

As a fallback, redirect `443 → 8443` with `iptables` or `nft` and keep
hostmux on `:8443`:

```sh
sudo iptables -t nat -A OUTPUT -p tcp -o lo --dport 443 -j REDIRECT --to-ports 8443
```

### macOS

macOS's packet filter can redirect `127.0.0.1:443 → 127.0.0.1:8443` so
hostmux itself stays unprivileged. Because browsers on modern macOS
usually resolve `*.localhost` to `::1` (IPv6 loopback), the anchor needs
both IPv4 and IPv6 rules. Create `/etc/pf.anchors/hostmux` with:

```
rdr pass on lo0 inet proto tcp from any to 127.0.0.1 port 443 -> 127.0.0.1 port 8443
rdr pass on lo0 inet6 proto tcp from any to ::1 port 443 -> ::1 port 8443
```

Add an anchor hook to `/etc/pf.conf`:

```
rdr-anchor "hostmux"
load anchor "hostmux" from "/etc/pf.anchors/hostmux"
```

And enable it:

```sh
sudo pfctl -e
sudo pfctl -f /etc/pf.conf
```

Alternative: run hostmux with elevated privileges. Not recommended for
daily dev.

### Windows

Binding `:443` typically requires administrator privileges. Run hostmux
from an elevated shell, or set up a local port redirect with `netsh`
interface portproxy. Refer to Windows documentation for the exact
invocation on your version.

## Example config with persistent routes

Create a TOML config (default: `~/.config/hostmux/hostmux.toml`):

```toml
domain = "example.com"
# hide_port = true   # omit the listener port from URLs printed by `hostmux url`/`run`

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

## Proxy hardening

By default hostmux uses Go's standard server and transport settings, which
suit local development. When hostmux fronts apps over a tunnel you can opt
into stricter limits with a `[proxy]` block. Every field is optional and
defaults to Go's behavior, so existing configs are unaffected. Note: the
`[proxy]` block is applied at daemon start, so changes require a restart
(it is not hot-reloaded like routes).

```toml
[proxy]
# Server-side limits.
read_header_timeout = "10s"   # max time to read request headers (anti-Slowloris)
idle_timeout = "120s"         # max idle keep-alive lifetime
max_header_bytes = 1048576    # cap request header size (bytes); 0 = Go default (1 MiB)

# Upstream transport.
dial_timeout = "10s"            # max time to connect to an upstream
response_header_timeout = "30s" # max wait for upstream response headers -> 504 on timeout

# Disable TLS verification for HTTPS upstreams that present self-signed
# certs. Off by default; enable only for trusted local dev servers.
upstream_insecure_skip_verify = false
```

Durations are TOML strings such as `"5s"`, `"500ms"`, or `"2m"`. On an
upstream timeout the proxy returns **504 Gateway Timeout**; on a refused or
unreachable upstream it returns **502 Bad Gateway**.

## Access logs

Enable per-request access logging to diagnose 404s (host routing), 502s
(upstream down), and 504s (upstream too slow). Logs go to the daemon's
stderr, so they appear in the foreground (`hostmux start --foreground`) or
wherever the detached daemon's stderr is captured.

```toml
access_log = true
log_format = "text"   # "text" (default) or "json"
```

Each line records method, host, path, status, latency, upstream, and source
(`config`, `socket:N`, or `manual:NAME`). Request headers and bodies are
never logged, so credentials and payloads stay out of the logs. Example:

```
access GET api.example.com/v1/users -> 200 (4.1ms) http://127.0.0.1:8080 src=socket:3
```

The `json` format emits one object per line for ingestion by log tooling.
Like `[proxy]`, this setting is read at daemon start (not hot-reloaded).

## How it's built

Most of this codebase was written with LLM agents. The architecture, edge case handling, and test coverage reflect that. It runs in production and the integration test catches regressions, but you should know how it was made.

## License

Apache 2.0. See [LICENSE](LICENSE).
