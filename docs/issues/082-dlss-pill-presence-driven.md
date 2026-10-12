---
title: DLSS pill reflects the present set, OptiScaler-independent
description: Readiness/version probe the present NVIDIA runtime (OptiScaler dir first, then the bin dir) whether or not OptiScaler is installed.
---

# 082 — DLSS pill reflects the present set, OptiScaler-independent

## What and why

Follow-up to 081, user directive: OptiScaler is not a precondition for
the DLSS libraries — they can coexist or not. The pill must reflect
whatever DLSS version is currently present: the OptiScaler dir first
(the subdir, where applicable), else the game bin dir.

081 coupled the probe to the updater's destination: a game whose set
never moved next to its `dlssg_to_fsr3` read not-ready with no
version, hiding a perfectly present (and updatable) set. It also
dropped the pre-081 nested discovery (a versioned set below the
injection top level read empty instead of versioned-but-not-ready).

Fix: `app.DLSSPresenceDir` — the anchor dir first when it holds any
runtime member, else the injection dir when it holds any, else none —
and both probes (scan `enrichVersions`, post-op UI refresh) read
version/readiness from that dir, keeping the recursive-walk version as
the fallback. The updater still installs into `ResolveDLSSDir`; the
pill describes what is there, not what the updater manages.

## Acceptance

- [x] NR layout: ready + subdir version (root decoy ignored).
- [x] Set stranded in the root while dlssg lives in `OptiScaler/`: pill shows the root set, ready.
- [x] No OptiScaler, versioned set nested below the top level: version shown, not ready (pre-081 display preserved).
- [x] `go test ./...`, `go vet`, gofmt clean.

## Outcome

Shipped: `app.DLSSPresenceDir` (anchor-with-members first, then the
injection dir with members, else none) + both probes (scan
`enrichVersions`, post-op UI refresh) read version (top-level PE,
walk fallback kept) and readiness from it. The 081 probe test's
shadow expectations flipped to the presence contract and a nested
no-OptiScaler case pins the pre-081 display. Full suite, vet,
gofmt clean.
Commit: dceb308.
