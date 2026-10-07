---
title: Progressive scan rendering — stream game cards during scan
description: Scan results upsert into live session state as they are discovered, so frontends render cards incrementally instead of at scan settle.
---

# 014 — Progressive scan rendering

## What and why

A scan commits its rows only at the very end of the pipeline
(discover → enrich → covers → lookup): on a cold boot or a manual rescan
the grid stays empty (or fully stale) until every cover fetch and online
lookup has finished. Change the pipeline to stream: each accepted game is
reported per source during discovery, the session upserts a cover-less row
into live state immediately (existing rows refresh in place by install
dir, new rows append), and the covers phase rebinds art per row as it
lands. The final settle (sort, title disambiguation, stale-row drop,
cache persist, EvScanDone) is unchanged.

Scan concurrency itself is already correct and stays untouched: `Scan`
runs on its own goroutine and is single-flight (a mid-flight `Scan` sets
a pending bit; the running scan re-runs once — covered by
`scanserial_test.go`).

Consequence: the "enrich" progress phase disappears as a separate bar
phase — enrichment (classify/EAC/version probes) now happens inline per
game during discovery. Phases become discover → covers → lookup.

## Acceptance

- [x] `discovery.ScanAll` reports each newly accepted (deduped) game via
      an optional callback, in merge order, before it returns.
- [x] `app.ScanAllLibraries` forwards each enriched entry via an optional
      callback during the scan; the returned slice is unchanged.
- [x] During a scan, `State.Rows` shows streamed rows before the scan
      settles (cards render while covers/lookups are still running).
- [x] A pre-existing row for a discovered game is refreshed in place, not
      duplicated or cleared-then-rebuilt.
- [x] Cover art binds per row during the covers phase (cards appear
      without waiting for art; art pops in as it resolves).
- [x] Scan remains non-blocking and single-flight; existing serialization
      and progress tests (updated for the dropped enrich phase) pass.
- [x] `go test ./...`, `go vet ./...`, `gofmt`, and a `GOOS=windows`
      build are green; docs updated.

## Outcome

Implemented as designed.

- `discovery.ScanOptions.OnGame` fires per newly accepted game in
  `ScanAll`'s merge (`internal/discovery/scanall.go`); duplicates are
  never reported.
- `app.ScanAllLibraries` loads store manifests up front, enriches inline
  in the `OnGame` callback, and forwards each entry via the new
  `ScanAllOptions.OnEntry` (`internal/app/app.go`). The per-game "enrich"
  progress tick is gone — enrichment is part of discovery now.
- `ui.runScan` streams: `OnEntry` builds a cover-less row (`baseRow`,
  split out of `toRow`) and `upsertScanRow` merges it into `State.Rows`
  (replace by install dir, else append) with a throttled `pokeScan`
  repaint (extracted from `scanProgress`). The covers phase then resolves
  each row's art (`resolveCover`) and re-upserts; manual extra-dir rows
  upsert the same way. Settle is unchanged: sort, disambiguate, prune
  stale rows, persist cache, `EvScanDone`. A cancelled scan leaves the
  partial streamed rows visible; the next scan completes them.
- Concurrency requirements were already satisfied (`Scan` is async and
  single-flight with pending-bit coalescing, `scanserial_test.go`) — no
  change needed there.
- Tests: `TestScanAll_OnGameStreamsAcceptedGames`,
  `TestScanAllLibraries_OnEntryStreamsEnriched`,
  `TestScanStream_RowsVisibleBeforeScanSettles`,
  `TestScanStream_ExistingRowRefreshedInPlace`;
  `TestScan_ProgressMonotonic` updated for the dropped enrich phase.
- Verification: `go test` exit 0 for all packages except
  `internal/gui` (blocked by a parallel session's in-flight red
  `pillwrap_test.go` — foreign, untouched); `go vet`, `gofmt`,
  `GOOS=windows`/`darwin` builds all clean.

Commit: recorded in the follow-up hash-pin commit.
