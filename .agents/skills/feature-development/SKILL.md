---
name: feature-development
description: Implement or change Mega Agents product behavior and fix regressions using acceptance criteria, test-driven development, and the repository's verification commands. Do not use for documentation-only or styling-only edits.
---

# Feature development

Deliver one observable vertical slice through the layers it needs. Preserve the
user's scope and avoid infrastructure that the behavior does not require.

## Define the behavior

- Translate the request into observable acceptance criteria before editing code.
- Identify the smallest public behavioral seam that proves each criterion. Test
  through stable interfaces rather than private implementation details.
- Surface a missing product decision only when different answers would materially
  change behavior; otherwise make a narrow, reversible assumption and state it.

## Red, green, refactor

1. Add or change a behavioral test and run it to observe the expected failure.
2. Implement the smallest coherent behavior that makes the test pass.
3. Refactor only while the focused tests remain green.

For a bug, first reproduce it with a regression test. Derive expected values from
the requirement, a fixed fixture, or another independent source; do not calculate
them with the production logic under test. Avoid assertions that merely repeat
the implementation or prove that a mock returned its configured value. Mock only
the external boundaries needed for determinism or isolation, and prefer real,
cheap collaborators when they expose meaningful integration behavior.

Documentation, styling-only work, generated code, and exploratory spikes do not
require a test-first cycle.

## Work across layers

- Prefer a complete thin path over disconnected backend and frontend scaffolding.
- Finish and verify one vertical slice before opening the next.
- Keep domain logic outside UI components and transport handlers when it has its
  own rules or meaningful edge cases.
- Treat browser-visible configuration as public. Keep secrets on the server.
- Reuse existing project structure and commands unless the requested behavior
  demonstrates a reason to change them.

## Verify and report

- Run the narrowest relevant tests during development.
- Run `make verify` after the implementation is complete.
- Run `make smoke` when routing, startup, embedded assets, or the compiled
  application changed.
- Report which acceptance criteria are covered, which commands passed, and any
  deliberate assumption or remaining limitation.
