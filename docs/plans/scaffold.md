# Plan: scaffold `gflight` — public Go client library for Google Flights

**Status:** implemented in commit `5a7c827` (`feat: scaffold gflight Go client library`).
**Module:** `github.com/khoi-truong/gflight` · Go 1.26 · MIT · public
**Not in scope:** creating the GitHub repo; implementing the actual client; editing plan-bee.

---

## Context

`plan-bee` (sibling repo, `../plan-bee`) is a Cloud Run *service*: Postgres, sqlc/goose,
River, OpenAPI contract, ko images, `cmd/{api,worker,canary}`, `.env` secrets. `gflight`
is a *library*: no DB, no server, no contract, no images, no secrets. The transferable
value from plan-bee is its **dev-infrastructure spine** — mise as toolchain SSoT,
golangci-lint v2, pre-commit, hardened GitHub Actions (`permissions: {}`, SHA-pinned
third-party actions, zizmor), Dependabot, AGENTS.md conventions. That spine was copied;
everything service-shaped was dropped.

Goal state: from the very first commit, `mise run ci` is green.

---

## Guardrails

**Must have**
- `mise.toml` is the only place tool versions appear (no versions in CI YAML).
- `go build ./...` and `go test ./...` pass at commit 1 — stub code + one real test.
- MIT `LICENSE` (plan-bee has none; a public library must).
- Workflows keep plan-bee's hardening: top-level `permissions: {}`, per-job least
  privilege, `persist-credentials: false`, third-party actions SHA-pinned with a
  `# vX.Y.Z` comment.
- Public-API surface is `context`-first and dependency-free beyond stdlib at commit 1.

**Must NOT have**
- No `.env`, no secrets, no `.env.example`, no `[env]` block in `mise.toml`.
- No `api/`, `db/`, `sqlc.yaml`, `.ko.yaml`, `.air.toml`, `cmd/{api,worker,canary}`.
- No `pkg/` directory (plan-bee convention).
- No real Google Flights implementation — stub only.
- No `docs/plans/` machinery beyond this file; `docs/` otherwise empty.

---

## Step 1 — Repo skeleton + git init

```
cd /Users/khoi/Code/maxmia
mkdir gflight && cd gflight
git init -b main
```

Files, in order:

| # | File | Purpose |
|---|------|---------|
| 1 | `LICENSE` | MIT, `Copyright (c) 2026 Khoi Truong`. |
| 2 | `.gitignore` | Adapted (see table below). |
| 3 | `.gitattributes` | **Verbatim** from plan-bee. |
| 4 | `.editorconfig` | **Verbatim** from plan-bee. |
| 5 | `go.mod` | `module github.com/khoi-truong/gflight` / `go 1.26`. No `go.sum` (stdlib only). |

---

## Step 2 — Toolchain: `mise.toml`

`[tools]` — drop goose, sqlc, air, ko, vacuum; keep the rest at plan-bee's pins:

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

| Task | Run |
|------|-----|
| `tidy` | `go mod tidy` |
| `fmt` | `golangci-lint fmt` |
| `lint` | `golangci-lint run` |
| `vet` | `go vet ./...` |
| `lint:actions` | `actionlint`, then `zizmor --persona=regular .` |
| `test` | `go test -race -shuffle=on -covermode=atomic -coverprofile=coverage.txt ./...` |
| `vuln` | `govulncheck ./...` |
| `build` | `go build -trimpath ./...` |
| `cover` | `go tool cover -html=coverage.txt -o coverage.html` |
| `example` | `go run ./examples/search` |
| `hooks` | `pre-commit install --install-hooks` |
| `setup` | `mise install`, then `mise run hooks` (**no** `.env` copy) |
| `ci` | `depends = ["tidy", "vet", "lint", "lint:actions", "test", "vuln"]` |

Dropped tasks: `lint:openapi`, `image:publish`, `sqlc`, `migrate:*`, `run:api`, `dev`.

---

## Step 3 — Lint + hook config

- **`.golangci.yml`** — plan-bee's, with:
  - `goimports.local-prefixes` → `github.com/khoi-truong/gflight`.
  - `rowserrcheck` and `sqlclosecheck` **dropped** (both `database/sql`-only).
  - Everything else kept, including the `_test.go` exclusion for `bodyclose`/`noctx`.
- **`.pre-commit-config.yaml`** — **verbatim** from plan-bee.

---

## Step 4 — Go scaffold (stub that compiles green)

Root package `gflight`, guts hidden in `internal/`:

| File | Purpose |
|------|---------|
| `doc.go` | Package doc + unofficial/scrape disclaimer + usage snippet. |
| `client.go` | `type Client` + `func New(opts ...Option) *Client`; `*http.Client`, user-agent, base URL. |
| `options.go` | `type Option func(*Client)`; `WithHTTPClient`, `WithUserAgent`, `WithBaseURL`, `WithLogger`. |
| `types.go` | `SearchRequest`, `Itinerary`, `Segment`, `Price`, `Airport` — exported value types. |
| `search.go` | `func (c *Client) Search(ctx, SearchRequest) ([]Itinerary, error)` — **stub returning `ErrNotImplemented`**. |
| `errors.go` | Sentinels: `ErrNotImplemented`, `ErrBadResponse`, `ErrNoResults`; `*HTTPError` with `Unwrap`. |
| `client_test.go` | External `package gflight_test`. Table-driven; real assertions, no `t.Skip`. |
| `internal/encoding/doc.go` | Placeholder for the base64/protobuf `tfs` request encoding. |
| `internal/testdata/.gitkeep` | Where recorded HTTP fixtures land. |
| `examples/search/main.go` | ~20-line consumer program; also the README quickstart. |

**Decision — `examples/` not `cmd/`.** A library shouldn't ship a CLI it doesn't maintain;
`examples/` compiles under `./...` so CI proves the public API stays usable. Add
`cmd/gflight/` later only if live-debugging the scraper turns painful.

---

## Step 5 — GitHub CI

| File | Treatment |
|------|-----------|
| `.github/actions/setup-go/action.yml` | **Verbatim** (mise-action SHA-pinned v2.4.4 + `actions/cache@v4`). |
| `.github/workflows/test.yml` | **Verbatim**. |
| `.github/workflows/check-code-quality.yml` | **Adapt**: delete the `openapi` job, filter entry, and output. Keep `changes` / `lint` / `govulncheck` / `actions` / `pre-commit`. |
| `.github/workflows/pr-title.yml` | **Verbatim** (regex, no token, dependabot bypass). |
| `.github/workflows/release.yml` | **New** — see Step 6. |
| `.github/dependabot.yml` | **Verbatim** — `gomod` + `github-actions`, weekly, 7d cooldown, `build(deps)` prefix. |
| `.github/zizmor.yml` | **Adapt**: same `unpinned-uses` policies; `self-repository.ignore` = the workflows using `./.github/actions/...` (`check-code-quality.yml`, `test.yml`, `release.yml`). |
| `.github/CODEOWNERS` | **Adapt** → `* @khoi-truong`. |
| `.github/copilot-instructions.md` | **Rewrite** for the library. |

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

### `AGENTS.md` — sections

1. **What this is** — unofficial, scrape-based Google Flights client. MIT, public. Not
   affiliated with Google; the upstream surface is undocumented and *will* break.
   Primary consumer: `plan-bee`'s `internal/provider` `gflight` adapter (`../plan-bee`).
2. **Package map** — the Step-4 table as a fenced tree.
3. **Toolchain — mise only**.
4. **Go conventions** — Go 1.26; `mise run fmt`; golangci-lint v2; `log/slog` only and
   **never from library code by default** (optional `*slog.Logger` via `WithLogger`,
   default `slog.New(slog.DiscardHandler)`); errors wrapped `%w`, compared with
   `errors.Is/As` against the `errors.go` sentinels; no global state, no package-level
   `http.Client`; `context.Context` first on every network call; `internal/` not `pkg/`.
5. **Testing** — fixture-driven: recorded responses in `internal/testdata/`, served by
   `httptest.Server`, injected via `WithHTTPClient` / `WithBaseURL`. No network in
   `go test` — ever. Table-driven, external `_test` packages, `-race -shuffle=on`.
6. **Versioning & release** — semver git tags `vX.Y.Z`; additive-first; pre-1.0 means
   minor bumps may break (stated in README); anything exported is API; godoc on every
   exported symbol; consumers pin an exact version.
7. **Repo-wide** — no secrets; tracked docs in `docs/`; `.omc/` gitignored; skills in
   `.claude/skills/<name>/`; non-Go files 2-space/LF/final newline; third-party Actions
   SHA-pinned with a version comment.
8. **Git** — default `main`; squash-merge only, **PR title** = conventional commit
   (`pr-title.yml`); `pre-commit` runs gofmt/vet/lint on staged Go; worktrees under
   `.worktrees/<branch>`.

### `CLAUDE.md`
One line: `See @AGENTS.md for project instructions and conventions.`

### `.claude/skills/` — one skill: `fixture-capture`

Hit live Google Flights, scrub the response (cookies, session ids, timestamps), write it
into `internal/testdata/<case>.json` with a provenance header, re-run the affected test.
The "decode the protobuf/base64 request format" skill is deferred until `internal/encoding`
is real. plan-bee's `html-reports` skill is not copied.

### `README.md`
Badges (pkg.go.dev, Tests workflow, Go Report Card) → one-paragraph what-it-is →
**prominent disclaimer** → `go get github.com/khoi-truong/gflight` → quickstart from
`examples/search` → stability note → dev quickstart → MIT line.

---

## Step 8 — Initial commit + green

Single commit on `main`:

```
git add -A
git commit -m "feat: scaffold gflight Go client library"
```

**"Green"** = `mise run ci` exits 0; `git diff --exit-code -- go.mod go.sum` clean after
`tidy`; `pre-commit run --all-files` passes; `zizmor --persona=regular .` reports no
findings; no network during tests.

---

## Copy / adapt / drop — full table

| plan-bee file | gflight |
|---|---|
| `.editorconfig` | **verbatim** |
| `.gitattributes` | **verbatim** |
| `.pre-commit-config.yaml` | **verbatim** |
| `.github/actions/setup-go/action.yml` | **verbatim** |
| `.github/workflows/test.yml` | **verbatim** |
| `.github/workflows/pr-title.yml` | **verbatim** |
| `.github/dependabot.yml` | **verbatim** |
| `.gitignore` | **adapt** — drop the `.env*` block and plan-bee `go.work` wording |
| `mise.toml` | **adapt** — Step 2 |
| `.golangci.yml` | **adapt** — local-prefix; drop `rowserrcheck`, `sqlclosecheck` |
| `.github/workflows/check-code-quality.yml` | **adapt** — remove `openapi` job + filter + output |
| `.github/zizmor.yml` | **adapt** |
| `.github/CODEOWNERS` | **adapt** — `* @khoi-truong` |
| `.github/copilot-instructions.md` | **rewrite** |
| `AGENTS.md` | **rewrite** |
| `CLAUDE.md` | **adapt** (same one-liner) |
| `README.md` | **rewrite** (library README) |
| — | **new**: `LICENSE` (MIT) |
| — | **new**: `.github/workflows/release.yml` |
| — | **new**: `.claude/skills/fixture-capture/SKILL.md` |
| `.env`, `.env.example` | **dropped** |
| `.ko.yaml`, `.air.toml`, `sqlc.yaml` | **dropped** |
| `api/`, `db/`, `cmd/`, `docs/plans/` (machinery) | **dropped** |
| `.claude/skills/html-reports/` | **dropped** |

---

## Decisions (resolved 2026-09-07)

1. **Name: `gflight`** (singular). Module `github.com/khoi-truong/gflight`. plan-bee's
   `AGENTS.md` / `README.md` / provider references updated to match in a follow-up.
2. **`examples/search/`** — not a `cmd/` CLI.
3. **Include** the minimal tag-triggered `release.yml`.
4. **Drop** `rowserrcheck` + `sqlclosecheck` from `.golangci.yml`.
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

- Creating the GitHub repo / pushing / branch protection.
- Implementing the actual Google Flights client.
- Adding `github.com/khoi-truong/gflight` to plan-bee's `go.work` (gitignored — local-dev
  step) and writing `plan-bee/internal/provider/gflight`.
