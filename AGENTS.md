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

- Integration tests use build tag `//go:build integration`
