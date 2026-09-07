# Plan: scaffold `gflight` — public Go client library for Google Flights

**Status:** implemented in commit `5a7c827` (`feat: scaffold gflight Go client library`).
**Module:** `github.com/khoi-truong/gflight` · Go 1.26 · MIT · public
**Not in scope:** creating the GitHub repo; implementing the actual client.

---

## Context

`gflight` is a **library**: no DB, no server, no API contract, no container
images, no secrets. It talks to the same undocumented endpoints the Google
Flights web UI uses, so every upstream shape is untrusted and versioned by
nobody.

The scaffold exists to make one thing true from the very first commit:
`mise run ci` is green. Everything below is dev-infrastructure spine — mise as
the toolchain single source of truth, golangci-lint v2, pre-commit, hardened
GitHub Actions (`permissions: {}`, SHA-pinned third-party actions, zizmor),
Dependabot, and the conventions in `AGENTS.md`.

---

## Guardrails

### Must have

- `mise.toml` is the only place tool versions appear (no versions in CI YAML).
- `go build ./...` and `go test ./...` pass at commit 1 — stub code + one real test.
- MIT `LICENSE` — a public library must ship one.
- Workflows keep the hardening: top-level `permissions: {}`, per-job least
  privilege, `persist-credentials: false`, third-party actions SHA-pinned with a
  `# vX.Y.Z` comment.
- Public-API surface is `context`-first and dependency-free beyond stdlib at commit 1.

### Must NOT have

- No `.env`, no secrets, no `.env.example`, no `[env]` block in `mise.toml`.
- No `api/`, `db/`, `sqlc.yaml`, `.ko.yaml`, `.air.toml`, `cmd/{api,worker,canary}`.
- No `pkg/` directory.
- No real Google Flights implementation — stub only.
- No `docs/plans/` machinery beyond this file; `docs/` otherwise empty.

---

## Step 1 — Repo skeleton + git init

```sh
git init -b main
```

Files, in order:

| #   | File             | Purpose                                                                         |
| --- | ---------------- | ------------------------------------------------------------------------------- |
| 1   | `LICENSE`        | MIT, `Copyright (c) 2026 Khoi Truong`.                                          |
| 2   | `.gitignore`     | Go library defaults; no `.env*` block.                                          |
| 3   | `.gitattributes` | Standard line-ending + linguist rules.                                          |
| 4   | `.editorconfig`  | Non-Go files: 2-space indent, LF, final newline.                                |
| 5   | `go.mod`         | `module github.com/khoi-truong/gflight` / `go 1.26`. No `go.sum` (stdlib only). |

---

## Step 2 — Toolchain: `mise.toml`

```toml
[tools]
go = "1.26"
golangci-lint = "2.13"
"go:golang.org/x/vuln/cmd/govulncheck" = "1.7.0"
"go:github.com/rhysd/actionlint/cmd/actionlint" = "1.7.12"
"aqua:zizmorcore/zizmor" = "1.30.0"
pre-commit = "4.6.2"

[settings]
go_default_packages_file = ""
idiomatic_version_file_enable_tools = []
```

No `[env]` block — there is no `.env`.

**Task list** (name → run):

| Task           | Run                                                                            |
| -------------- | ------------------------------------------------------------------------------ |
| `tidy`         | `go mod tidy`                                                                  |
| `fmt`          | `golangci-lint fmt`                                                            |
| `lint`         | `golangci-lint run`                                                            |
| `vet`          | `go vet ./...`                                                                 |
| `lint:actions` | `actionlint`, then `zizmor --persona=regular .`                                |
| `test`         | `go test -race -shuffle=on -covermode=atomic -coverprofile=coverage.txt ./...` |
| `vuln`         | `govulncheck ./...`                                                            |
| `build`        | `go build -trimpath ./...`                                                     |
| `cover`        | `go tool cover -html=coverage.txt -o coverage.html`                            |
| `example`      | `go run ./examples/search`                                                     |
| `hooks`        | `pre-commit install --install-hooks`                                           |
| `setup`        | `mise install`, then `mise run hooks`                                          |
| `ci`           | `depends = ["tidy", "vet", "lint", "lint:actions", "test", "vuln"]`            |

---

## Step 3 — Lint + hook config

- **`.golangci.yml`** — golangci-lint v2, with:
  - `goimports.local-prefixes` → `github.com/khoi-truong/gflight`.
  - No `database/sql`-only linters (`rowserrcheck`, `sqlclosecheck`).
  - `_test.go` exclusion for `bodyclose` / `noctx`.
- **`.pre-commit-config.yaml`** — gofmt, `go vet`, golangci-lint on staged Go.

---

## Step 4 — Go scaffold (stub that compiles green)

Root package `gflight`, guts hidden in `internal/`:

| File                         | Purpose                                                                                                      |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------ |
| `doc.go`                     | Package doc + unofficial/scrape disclaimer + usage snippet.                                                  |
| `client.go`                  | `type Client` + `func New(opts ...Option) *Client`; `*http.Client`, user-agent, base URL.                    |
| `options.go`                 | `type Option func(*Client)`; `WithHTTPClient`, `WithUserAgent`, `WithBaseURL`, `WithLogger`.                 |
| `types.go`                   | `SearchRequest`, `Itinerary`, `Segment`, `Price`, `Airport` — exported value types.                          |
| `search.go`                  | `func (c *Client) Search(ctx, SearchRequest) ([]Itinerary, error)` — **stub returning `ErrNotImplemented`**. |
| `errors.go`                  | Sentinels: `ErrNotImplemented`, `ErrBadResponse`, `ErrNoResults`; `*HTTPError` with `Unwrap`.                |
| `client_test.go`             | External `package gflight_test`. Table-driven; real assertions, no `t.Skip`.                                 |
| `internal/encoding/doc.go`   | Placeholder for the base64/protobuf `tfs` request encoding.                                                  |
| `internal/testdata/.gitkeep` | Where recorded HTTP fixtures land.                                                                           |
| `examples/search/main.go`    | ~20-line consumer program; also the README quickstart.                                                       |

**Decision — `examples/` not `cmd/`.** A library shouldn't ship a CLI it doesn't
maintain; `examples/` compiles under `./...` so CI proves the public API stays
usable. Add `cmd/gflight/` later only if live-debugging the scraper turns painful.

---

## Step 5 — GitHub CI

| File                                       | Purpose                                                                                          |
| ------------------------------------------ | ----------------------------------------------------------------------------------------------- |
| `.github/actions/setup-go/action.yml`      | Composite: provision the toolchain via mise + Go module/build cache.                            |
| `.github/workflows/_detect-changes.yml`    | Reusable path-gate: one source of truth for which jobs run on a PR.                             |
| `.github/workflows/test.yml`               | Build + `go test -race -shuffle=on`, path-gated.                                                |
| `.github/workflows/check-code-quality.yml` | `lint` / `govulncheck` / `workflow-lint` / `pre-commit`, path-gated, behind a single gate job. |
| `.github/workflows/check-pr-title.yml`     | Conventional-commit PR-title check (regex, no token, Dependabot bypass).                        |
| `.github/workflows/release.yml`            | Tag-triggered — see Step 6.                                                                     |
| `.github/dependabot.yml`                   | `gomod` + `github-actions`, weekly, 7d cooldown, `build(deps)` prefix.                          |
| `.github/zizmor.yml`                       | `unpinned-uses` policy; `self-repository.ignore` for workflows using `./.github/actions/...`.   |
| `.github/CODEOWNERS`                       | `* @khoi-truong`.                                                                               |
| `.github/copilot-instructions.md`          | Points at `AGENTS.md`.                                                                          |

---

## Step 6 — Release workflow

One small `.github/workflows/release.yml`:

- Trigger: `push: tags: ['v*.*.*']`.
- `permissions: {}` top-level; the single job gets `contents: write`.
- Steps: checkout (`persist-credentials: false`) → `./.github/actions/setup-go` →
  `mise run ci` → `gh release create "$GITHUB_REF_NAME" --generate-notes`
  (`GH_TOKEN: ${{ github.token }}`, no third-party action).

---

## Step 7 — Docs, agent rules, skills

### `AGENTS.md`

Sections: **What this is** · **Package map** · **Toolchain — mise only** ·
**Go conventions** · **Testing** · **Versioning & release** · **Repo-wide** ·
**Git**.

### `CLAUDE.md`

One line: `See @AGENTS.md for project instructions and conventions.`

### `.claude/skills/` — one skill: `fixture-capture`

Hit live Google Flights, scrub the response (cookies, session ids, timestamps),
write it into `internal/testdata/<case>.json` with a provenance header, re-run the
affected test. The "decode the protobuf/base64 request format" skill is deferred
until `internal/encoding` is real.

### `README.md`

Badges (pkg.go.dev, Tests workflow, Go Report Card) → one-paragraph what-it-is →
**prominent disclaimer** → `go get github.com/khoi-truong/gflight` → quickstart
from `examples/search` → stability note → dev quickstart → MIT line.

---

## Step 8 — Initial commit + green

```sh
git add -A
git commit -m "feat: scaffold gflight Go client library"
```

**"Green"** = `mise run ci` exits 0; `git diff --exit-code -- go.mod go.sum`
clean after `tidy`; `pre-commit run --all-files` passes; `zizmor
--persona=regular .` reports no findings; no network during tests.

---

## Decisions (resolved 2026-09-07)

1. **Name: `gflight`** (singular). Module `github.com/khoi-truong/gflight`.
2. **`examples/search/`** — not a `cmd/` CLI.
3. **Include** the minimal tag-triggered `release.yml`.
4. **Drop** `rowserrcheck` + `sqlclosecheck` from `.golangci.yml` (both
   `database/sql`-only).
5. **One skill** (`fixture-capture`) only.

### Implementation deviations from the plan

- Options `WithBaseURL` and `WithLogger` added beyond the plan's `WithHTTPClient` /
  `WithUserAgent` — `WithBaseURL` makes the fixture/`httptest` testing story work;
  `WithLogger` is required by the AGENTS.md "optional `*slog.Logger`, default discard"
  rule. Plus accessors `HTTPClient` / `BaseURL` / `UserAgent` for the external `_test`
  package.
- `examples/search/main.go` carries two `//nolint:staticcheck` (SA4023) because the stub
  `Search` provably always returns a non-nil error; both are removed with the stub.
  `main` split into `main`/`run` for gocritic's `exitAfterDefer`.
- `mise.toml` pins `go = "1.26"`. A cached 1.26.1 tripped govulncheck `GO-2026-5037`
  (crypto/x509, fixed 1.26.4); resolved by upgrading the local toolchain. CI provisions
  fresh so it picks the latest 1.26.x. Pin a patch version if the guarantee matters.

---

## Out of scope (noted for later)

- Branch protection configuration.
- Implementing the actual Google Flights client.
