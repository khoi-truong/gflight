Follow the conventions in [AGENTS.md](../AGENTS.md).

This repo is `github.com/khoi-truong/gflight` — an unofficial, scrape-based Go
client library for Google Flights. Library only: no server, no database, no
secrets.

- Toolchain and tasks: **mise only** — `mise run {fmt,lint,vet,test,vuln,ci}`.
  Never put tool versions in CI YAML.
- Go 1.26; `internal/` not `pkg/`; no `cmd/` — the consumer demo is
  `examples/search`.
- Public API is context-first (`ctx context.Context` as the first parameter),
  configured with functional `Option`s, and stdlib-only where possible.
- Errors wrap with `%w` and are compared with `errors.Is/As` against the
  sentinels in `errors.go`. Never match on message text.
- Logging: `log/slog` only, and library code logs nothing by default — take an
  optional `*slog.Logger` via an Option, defaulting to a discard handler.
- Tests are fixture-driven from `internal/testdata/`, served by
  `httptest.Server`. No network in `go test` — ever.
- Conventional-commit PR titles (squash-merge only); agent worktrees under
  `.worktrees/`; no hardcoded secrets.
