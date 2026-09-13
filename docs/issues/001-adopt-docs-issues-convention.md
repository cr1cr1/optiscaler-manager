---
title: Adopt the docs/issues/ convention
description: Track the what of every change in docs/issues/ per the global AGENTS.md rule.
issue: 1
status: done
---

# 1 — Adopt the docs/issues/ convention

## What and why

The global AGENTS.md requires every codebase change to come with an
issue in `docs/issues/` — numbered, never reused, never deleted. This
repository had no such directory, so the rule had nothing to point at.

## Acceptance

- [x] `docs/issues/issues-conventions.md` states the rules.
- [x] `docs/issues/TEMPLATE.md` is the starting point for new issues.
- [x] The repository AGENTS.md points at the convention.
- [x] `docs/index.md` lists `docs/issues/` in the document map.

## Outcome

Adopted with this issue's own commit. Issue 2 records the open
pre-existing gofmt drift found during the v0.16 work.
