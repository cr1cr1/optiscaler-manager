---
title: Issue conventions
description: How docs/issues/ records the what of every change in this repository.
---

# Issue conventions

`docs/issues/` records the *what* of changes in this repository:
features, bugs, tasks, and chores. The *why* behind design decisions
lives in `docs/scope.md` (settled decisions) and `docs/architecture.md`
(how the pieces fit); when the docs disagree, the design docs win.

## Rules

- One file per issue, named `NNN-kebab-title.md` (for example
  `001-adopt-docs-issues-convention.md`).
- Numbers are never reused. When an issue is dropped, close it with the
  reason instead of deleting or renumbering it.
- Done issues are never deleted. Set `status: done`, fill in the
  Outcome section with the commit hash, and leave the file in place.
- Any change to the codebase comes with an issue, even a trivial one. A
  trivial change may use a one-line What-and-why section.
- Start a new issue from `TEMPLATE.md`.
- The OKF frontmatter gate scans only `docs/` top-level files, so these
  files are not gated; keep the frontmatter anyway (`title`,
  `description`) for consistency with the rest of the docs.

## Lifecycle

An issue is created with the work it records, states the what, the why,
and the acceptance checks, and closes with the outcome: what actually
happened, the commit hash, and anything left open. Issues track work
after the fact as well as before — a finished change still gets its
issue (see issue 1, closed at adoption time).
