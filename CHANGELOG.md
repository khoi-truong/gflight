# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Pre-1.0, minor bumps may break the API. Two things move with upstream and are
part of the effective contract even though Go's type system does not enforce
them: the `errors.Is` targets a caller matches on, and the fields of a decoded
struct. Both are recorded here whenever they change.

## [Unreleased]

### Removed

- **`Client.BookingOptions` and the `BookingOption` type.** The
  `GetBookingResults` method they called is gated behind a BotGuard token that
  no Go client can mint, so the API could never have succeeded against the live
  endpoint — its tests passed only because the fixture was derived from a
  browser capture. Removed rather than kept as a method that always fails.
  `Itinerary.BookingToken` is still decoded and exposed; nothing in this library
  can spend it.

### Changed

- **Requests now go to `/_/FlightsFrontendUi/data/batchexecute`** with
  `rpcids=LqxFAb` (`GetShoppingResultsPrefetch`) instead of the per-method path
  style. The path style is not retired — it is BotGuard-gated, as is every rpc
  id except this one. No public API changed; callers who pinned `WithBaseURL`
  to a recording will need to re-record.
- **`internal/wire` accepts both response framings.** The live route answers
  unframed unless `&rt=c` is set, and the two length-prefix conventions in
  circulation differ by the newlines they count.
- Godoc and README no longer attribute upstream blocks to TLS fingerprinting.
  A uTLS Chrome handshake was tested against the gate and makes no difference;
  `examples/utls` now says so.

### Added

- **`docs/wire/botguard.md`** — the gate, the evidence for it (header ablation,
  token rebinding, the open-vs-gated status-code test), and the full rpc id
  table for `FlightsFrontendService`.
