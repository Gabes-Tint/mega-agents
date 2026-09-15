---
name: feature-development
description: Implement or change Mega Agents product behavior and fix regressions using acceptance criteria, test-driven development, and the repository's verification commands. Do not use for documentation-only or styling-only edits.
---

# Feature development

Deliver one observable vertical slice through the layers it needs. Preserve the
user's scope and avoid infrastructure that the behavior does not require.

## Define the behavior

- Translate the request into observable acceptance criteria before editing code.
- Identify the smallest test boundary that proves each criterion.
- Surface a missing product decision only when different answers would materially
  change behavior; otherwise make a narrow, reversible assumption and state it.

## Red, green, refactor

1. Add or change a behavioral test and run it to observe the expected failure.
2. Implement the smallest coherent behavior that makes the test pass.
3. Refactor only while the focused tests remain green.

For a bug, first reproduce it with a regression test. Test public behavior rather
than private implementation details. Documentation, styling-only work, generated
code, and exploratory spikes do not require a test-first cycle.

## Work across layers

- Prefer a complete thin path over disconnected backend and frontend scaffolding.
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
