# Plans

One file per plan, kebab-case, no dates in the name. Every plan opens with a
`**Status:**` line: `proposed`, `in progress`, `implemented (<sha>)`, or
`abandoned (<why>)`.

Tasks are checkboxes under milestone headings, ticked in the same PR that
implements them. Amend a plan when reality diverges — never leave it wrong.
Reverse-engineering field maps are not plans; they live in `docs/wire/`.

| Plan | Status | PR | Hook |
| --- | --- | --- | --- |
| [scaffold.md](scaffold.md) | implemented (`3e2f6ae`) | — | Dev-infra spine: mise, golangci-lint v2, hardened Actions, `mise run ci` green |
| [porting.md](porting.md) | closed (M1–M3, M5a–M5c done; M4 abandoned — upstream gated) | [#3](https://github.com/khoi-truong/gflight/pull/3) | Port a real Google Flights client: `FlightsFrontendService` RPC, stdlib-only, fixture-driven |
