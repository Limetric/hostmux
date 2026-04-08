# Default TLS For `hostmux serve`

## Goal

Change `hostmux serve` so TLS is enabled by default and a self-signed
certificate is created automatically when the user has not configured one.

## Current Behavior

- `hostmux serve` defaults to plain HTTP on `:8080`.
- TLS is only enabled when the config file includes a `[tls]` block.
- The `[tls]` block currently requires explicit `listen`, `cert`, and `key`
  values to be useful.

## Desired Behavior

- `hostmux serve` should default to TLS-only on `:8443`.
- When no config file exists, or when the config omits `[tls]`, the daemon
  should still start with TLS enabled.
- The daemon should automatically create and persist a self-signed
  certificate and private key if the effective TLS cert/key paths do not
  exist yet.
- The generated certificate should be reused across restarts.
- An explicit `[tls]` block remains authoritative:
  - `tls.listen` overrides the default `:8443`.
  - `tls.cert` and `tls.key` override the managed certificate paths.
  - If `[tls]` exists but omits `cert` and `key`, hostmux should use the
    managed paths and generate the self-signed pair there if needed.

## Non-Goals

- Reintroducing plain HTTP as a default listener.
- Designing a broader TLS mode system such as `auto`, `manual`, or `off`.
- Implementing hot-reload of TLS listener settings or certificate material.
  The existing config watcher continues to reload router entries only.

## Effective TLS Resolution

At startup, `serve` should compute one effective TLS config before starting
the HTTP server:

- Default listen address: `:8443`
- Default cert path: `~/.hostmux/tls/hostmux.crt`
- Default key path: `~/.hostmux/tls/hostmux.key`

Resolution order:

1. Start from the defaults above.
2. If a config file was loaded and contains a `[tls]` block:
   - Override `listen` if `tls.listen` is non-empty.
   - Override `cert` if `tls.cert` is non-empty.
   - Override `key` if `tls.key` is non-empty.
3. Expand `~/...` paths for configured cert/key values before use.

There is no default plain listener after this change.

## Managed Certificate Storage

Managed certificate material should live under the existing hostmux state
directory so it follows the same per-user storage convention already used by
the discovery socket file:

- Directory: `~/.hostmux/tls`
- Certificate: `~/.hostmux/tls/hostmux.crt`
- Private key: `~/.hostmux/tls/hostmux.key`

Creation expectations:

- Create the TLS directory with mode `0700`.
- Write the certificate with mode `0644`.
- Write the private key with mode `0600`.

## Generation Rules

Before starting the TLS listener:

- If both effective cert and key files exist, reuse them as-is.
- If neither file exists, generate a new self-signed certificate and key.
- If only one of the two files exists, fail startup with a clear error rather
  than overwriting the partial state.
- If the files exist but cannot be loaded as a valid keypair, fail startup
  with a clear error rather than silently regenerating them.

This keeps startup deterministic and avoids replacing user-managed files or
masking broken filesystem state.

## Implementation Shape

Add a small internal package dedicated to managed TLS material. Its
responsibilities should be:

- Resolve managed default cert/key paths.
- Expand configured paths when needed.
- Ensure the effective cert/key pair exists.
- Generate a self-signed certificate and key when both effective files are
  absent.
- Validate that the resulting pair can be loaded by Go's TLS stack.

`serve.go` should remain orchestration code:

1. Load config if present.
2. Resolve the effective TLS config from defaults plus optional config
   overrides.
3. Ensure the cert/key pair exists and is valid.
4. Start the existing TLS listener with the resolved paths.

The existing listener package can stay structurally the same, but it will now
always receive a non-nil TLS config from `serve`.

## Certificate Characteristics

The generated certificate only needs to satisfy local development use:

- Self-signed leaf certificate.
- Suitable for server authentication.
- Includes SANs that make local use practical.

The exact SAN set can be kept minimal for this change. The implementation
should cover localhost development without introducing extra configuration
surface.

## Logging

Startup logs should reflect the new default behavior:

- Log the effective TLS listen address.
- When a managed self-signed certificate is generated, log that generation
  occurred and where the files were written.

Avoid noisy logs on normal reuse of existing managed certs.

## Testing

### Unit Tests

- Config/default resolution returns TLS `:8443` when no `[tls]` block exists.
- Config override precedence for `tls.listen`, `tls.cert`, and `tls.key`.
- Managed cert generation creates both files with the expected permissions.
- Existing valid keypair is reused.
- Partial state (only cert or only key) returns an error.
- Invalid existing keypair returns an error.

### Integration Test Updates

- The end-to-end daemon test should connect to the default HTTPS listener
  instead of the previous plain HTTP listener.
- The client should use a TLS transport that skips certificate verification
  for the self-signed cert during the test.

## Docs Changes

Update the README so it reflects the new defaults:

- `hostmux serve` now starts TLS by default.
- The default listen address is `:8443`.
- A self-signed certificate is auto-generated and persisted under
  `~/.hostmux/tls/`.
- The `[tls]` block can still override listen/cert/key paths.

## Risks And Constraints

- Changing the default port from `:8080` to `:8443` is a behavioral breaking
  change for users who relied on the old default with no config.
- Self-signed certificates require clients to trust or explicitly bypass
  verification during local development.
- The config watcher does not currently rebind listeners, so TLS settings are
  startup-only configuration.
