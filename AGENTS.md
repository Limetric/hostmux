# AGENTS.md

## Commands

**Go code must be formatted before every commit** — CI rejects unformatted code.

```bash
go fmt ./...                   # Format all packages — run before commit
go build -o build/hostmux .    # Build binary
go vet ./...                   # Lint
go test ./... -count=1         # Unit tests (no DB required)
go test -run TestFoo ./...     # Run a single test
```

## Key references

- Integration tests use build tag `//go:build integration && !windows`. They
  rely on `golang.org/x/sys/unix`, so they build and run on Linux/macOS only.
  Run them with `go test -tags=integration ./...`.

## Platform support / CI

- CI runs the same quality gate (gofmt check, `go vet`, `go tool staticcheck`,
  `go test ./...`) on both `ubuntu-latest` and `windows-latest`.
- Default unit tests must stay portable: prefer `t.TempDir()` / `os.MkdirTemp`,
  `filepath.Join`, and avoid asserting Unix-style path literals for values that
  pass through `path/filepath` (those become `\`-separated on Windows).
- Integration tests remain Unix-only by build tag; full Windows E2E (sockets,
  child-process spawning) is a separate follow-up.
