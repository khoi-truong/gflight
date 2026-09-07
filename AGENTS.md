# AGENTS.md

## What this is

**gflight** — an unofficial Go client for Google Flights
(`github.com/khoi-truong/gflight`). MIT, public, library only.

It talks to the same undocumented endpoints the Google Flights web UI uses. It
is **not affiliated with, endorsed by, or supported by Google**; the upstream
surface is undocumented and *will* break. Treat every upstream shape as
untrusted and versioned by nobody.

The stability contract lives in *Versioning & release* below.

No server, no database, no API contract, no container images, no secrets.

## Package map

```text
doc.go                     package doc + disclaimer (the godoc landing page)
client.go                  Client struct + New(opts ...Option)
options.go                 Option func(*Client) — WithHTTPClient, WithUserAgent, …
types.go                   SearchRequest, Itinerary, Segment, Price, Airport
search.go                  Client.Search(ctx, SearchRequest) ([]Itinerary, error)
errors.go                  sentinels (ErrNotImplemented, ErrBadResponse, ErrNoResults) + *HTTPError
client_test.go             external package gflight_test — table-driven
internal/encoding/         base64/protobuf `tfs` request encoding (unexported guts)
internal/testdata/         recorded, scrubbed HTTP fixtures
examples/search/           ~20-line consumer program; also the README quickstart
```

There is deliberately no `cmd/`: a library should not ship a CLI it does not
maintain. `examples/` compiles under `./...`, so CI proves the public API stays
usable.

## Toolchain — mise only

`mise` is the single source of truth for the Go toolchain and every dev tool.
Never add tool versions to CI YAML, build config, or a Makefile — declare them
in `mise.toml`.

- `mise install` — provision the toolchain.
- `mise run setup` — one-shot dev bootstrap (tools + git hooks).
- `mise run ci` — tidy + vet + lint + lint:actions + test + vuln. Must pass
  before pushing.
- `mise run <task>` — `fmt`, `build`, `test`, `cover`, `example`, `hooks`, …
  (`mise tasks` for all).
- `mise run hooks` — install git hooks (once after cloning).

## Go conventions

- Go 1.26. Format with `mise run fmt`. Lint: `.golangci.yml` (golangci-lint v2).
- Every network call takes a `context.Context` as its first parameter.
- Configuration is functional options (`Option func(*Client)`) — additive-first,
  so a new knob never breaks a caller.
- Errors: wrap with `%w`, compare with `errors.Is/As` against the sentinels in
  `errors.go`. Never match on message text. `*HTTPError` unwraps to
  `ErrBadResponse`.
- Logging: `log/slog` only — and library code logs nothing by default. Accept an
  optional `*slog.Logger` via an Option, defaulting to
  `slog.New(slog.DiscardHandler)`. No global loggers.
- No global state, no package-level `http.Client`. `internal/` not `pkg/`.
- Keep the dependency list near-empty; stdlib is the default answer.

## Testing

Google Flights has no sandbox, so tests are **fixture-driven**: recorded
responses in `internal/testdata/`, served by an `httptest.Server`, injected with
`WithHTTPClient` (or `WithBaseURL`).

- **No network in `go test` — ever.**
- Table-driven; external `_test` packages where practical.
- `go test -race -shuffle=on` (what `mise run test` runs).
- Capture and scrub fixtures with the `fixture-capture` skill
  (`.claude/skills/fixture-capture/SKILL.md`). Never commit an unscrubbed
  response.
- Live smoke checks, if ever added, sit behind a build tag and are excluded from
  `mise run ci`.

## Versioning & release

- Semver git tags `vX.Y.Z`; `.github/workflows/release.yml` cuts the release.
- Additive-first: add fields and Options, never repurpose or narrow existing
  ones.
- Pre-1.0, minor bumps may break. That is stated in the README, and consumers
  pin an exact version.
- Anything exported is API — keep the surface small and put godoc on every
  exported symbol.

## Repo-wide

- No secrets of any kind; this library needs none. No `.env`.
- Tracked docs live in `docs/` as Markdown. `.omc/` is gitignored scratch.
- Non-Go files: 2-space indent, LF, final newline (`.editorconfig`).
- Agent skills live in `.claude/skills/<name>/`.
- CI: third-party GitHub Actions are pinned to a commit SHA (with a `# vX.Y.Z`
  comment); `actions/*` and `github/*` may use a major-version tag. Workflows
  keep top-level `permissions: {}`, per-job least privilege, and
  `persist-credentials: false`. Dependabot bumps both ecosystems.

## Git

- Default branch `main`. **Squash-merge only**, so the *PR title* must be a
  conventional commit (`feat:`, `fix:`, `chore:`, `docs:`, `refactor:`, `test:`,
  `perf:`, `build:`, `ci:`, `revert:`) — checked by
  `.github/workflows/check-pr-title.yml`. Individual commit messages are
  unconstrained. `pre-commit` runs gofmt, `go vet`, golangci-lint on staged Go.
- Parallel work: create worktrees under `.worktrees/` (gitignored):
  `git worktree add .worktrees/<branch> -b <branch>`.
- `go.work` is gitignored — never commit it.
