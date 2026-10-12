---
title: Shadow DLSS set warns in the details pane
description: When a complete NVIDIA set sits stranded outside the managed dir, the details pane says so instead of staying silent.
---

# 083 — Shadow DLSS set warns in the details pane

## What and why

081/082 split the writer from the display: the updater manages the
OptiScaler-dir set while the pill shows the present set. That leaves
a silent hazard — a **shadow set**: a complete three-file NVIDIA set
in the injection root while the managed set lives in `OptiScaler/`
(or the reverse). The pill shows one copy, the updater maintains one
copy, but the game loader may prefer the other, nullifying updates
with no visible sign.

Which copy the loader prefers is still an open question (needs a
real-machine probe, e.g. Witcher 3 NR layout) — the warning must name
both locations regardless, so the user can judge.

## Acceptance

- [ ] Detection: the session knows when a complete NVIDIA set exists
  outside the presence dir (root vs `OptiScaler/`, either direction).
- [ ] The details pane surfaces the conflict next to the Game dir
  truth line — which copy is managed, which copy shadows.
- [ ] No automatic deletion or move of the stranded files: game files
  need explicit consent (076/077 precedent). A consent-gated cleanup
  action is future work, not this issue.
- [ ] Red-first test; `go test ./...`, `go vet`, gofmt clean.

## Outcome

Filled in when the issue closes: what actually happened, the commit
hash, anything left open.
