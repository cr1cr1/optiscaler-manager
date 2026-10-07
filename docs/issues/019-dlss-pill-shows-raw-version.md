---
title: DLSS pill shows the raw dll version, not the marketing name
description: Switching DLSS versions (e.g. restoring an older backup) was invisible in the pill because the label was a coarse vendored marketing name; the pill now shows the raw dll version in tag form.
---

# 019 — DLSS pill shows the raw dll version

## What and why

Switching DLSS versions (e.g. restoring an older backup from the restore
menu) was not reflected in the pill version label. Root cause: the pill
label came from the vendored version→marketing-name map
(`pever.MarketingName`), but NVIDIA reuses one marketing name for many dll
versions (310.5.0, 310.5.3 and 310.6.0 are all "DLSS 4.5") and the vendored
table tops out at 310.6.0 while current releases are 310.9.x+ (tier-4
nearest-below renders every modern dll as "DLSS 4.5"). The refresh
machinery worked fine — the label simply could not change.

The DLSS surface already speaks raw versions everywhere else: the restore
menu labels snapshots by raw dll version and the update target is the raw
release tag. The pill now does the same: `DLSS 310.5.3` (one trailing ".0"
trimmed from the 4-part PE version). FSR/XeSS labels are unchanged.

## Acceptance

- [x] Two dll versions in one marketing bucket (310.5.3.0 / 310.6.0.0)
      produce distinct pill labels.
- [x] Labels match NVIDIA's tag form ("310.5.3", "3.7.20"); a version
      without a trailing ".0" is shown verbatim.
- [x] After update or restore, the pill reflects the applied version
      (existing refresh path, now with a label that can change).
- [x] `go test ./...`, `go vet`, `gofmt` green; docs updated.

## Outcome

Implemented as designed.

- `app.ComponentVersions` labels DLSS via the new `dlssLabel` (raw,
  tag-form) instead of `pever.MarketingName`; FSR/XeSS keep the marketing
  maps. TDD red witnessed: 310.5.3.0 and 310.6.0.0 both rendered "DLSS
  4.5".
- The vendored DLSS map is now prod-unused; it stays as the tier-lookup's
  test dataset with a comment saying so (deleting it would only churn two
  pever test files).
- Assertions updated to raw labels: `TestUpdateDLSSAndRestoreRoundTrip`
  (ui), `TestTUIDetailUpdateDLSS` (tui); the 3.7.20.0 fixtures keep their
  label ("DLSS 3.7.20") because trailing-".0" trimming lands on the same
  string. New: `TestComponentVersionsDLSSLabelIsRawVersion` (app).
- `docs/scope.md`, `docs/architecture.md` version-display notes updated.

Commit: TBD.
