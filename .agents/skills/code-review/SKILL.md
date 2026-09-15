---
name: code-review
description: Review Mega Agents changes for repository conformance and fidelity to the request or acceptance criteria. Use for review requests, not implementation-only work.
---

# Code review

Find consequential defects and give the author evidence they can act on. A
review request authorizes inspection and proportionate verification, not edits,
commits, issue-tracker changes, or expansion of the requested scope.

## Review on two axes

**Repository conformance:** Check the relevant `AGENTS.md`, canonical skills,
guardrails, security and permission boundaries, ownership rules, test policy,
and required verification. Inspect the actual repository state rather than
assuming a reported command covered the current diff.

**Request fidelity:** Compare the implementation with the user's request,
acceptance criteria, specification, and documented decisions. Look for omitted
cases, behavior that contradicts the requirement, and additions that create
unrequested product or operational scope.

Keep the axes separate in reasoning so passing repository checks is not mistaken
for implementing the right behavior, and plausible behavior is not mistaken for
following safety or verification requirements.

## Prioritize useful findings

Prioritize correctness failures, regressions, security or data-exposure risks,
destructive or unauthorized effects, scope creep, and missing verification that
could conceal those problems. Inspect relevant callers, tests, and boundaries
when the changed line alone is insufficient.

Skip preferences and style issues already rejected by deterministic formatters
or linters unless the enforcement is absent, bypassed, or demonstrably misses the
case. Do not require an issue tracker, a particular review ceremony, or parallel
agents.

For each finding, identify the tightest relevant location, the triggering
condition, the observable impact, and why existing checks do not protect against
it. Rank findings by severity. Distinguish verified defects from questions and
residual risks; do not inflate uncertainty into a finding.

Run the smallest useful read-only checks when evidence requires them and state
what was not run. Lead the report with actionable findings. If there are none,
say so and note any material verification gap or untested risk.
