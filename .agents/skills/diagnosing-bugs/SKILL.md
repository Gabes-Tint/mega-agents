---
name: diagnosing-bugs
description: Diagnose Mega Agents failures, crashes, or unexpected behavior with a deterministic evidence loop. A diagnosis-only request does not authorize implementing a fix.
---

# Diagnosing bugs

Produce an evidence-backed cause or a precise account of what remains unknown.
Keep investigation distinct from repair: when the user asks only for diagnosis,
inspect and report without changing product behavior.

## Establish the failure

- Restate the observable symptom, expected behavior, affected scope, and relevant
  constraints. Inspect repository status before experiments and preserve unrelated
  work.
- Reproduce with the smallest deterministic command or fixture available. Record
  exact inputs, environment facts that matter, expected result, and actual result.
- Tighten the feedback loop before broadening it. Minimize data, components,
  timing, and environment variables until one run can distinguish hypotheses.
- If the failure is intermittent, measure frequency and vary one suspected
  dimension at a time. Do not treat a single passing retry as disproof.

## Test hypotheses

State each hypothesis so evidence could falsify it. Prefer the cheapest next
observation that separates competing explanations, then update or discard the
hypothesis from the result. Trace causes across boundaries only as far as the
evidence leads; a nearby suspicious line is not itself a root cause.

Add targeted, temporary instrumentation only when existing evidence is
insufficient. Capture the narrow values or transitions needed to answer the
current question. Never expose credentials, tokens, personal data, or sensitive
payloads in commands, logs, fixtures, screenshots, or reports; redact before
sharing. Remove temporary probes and generated artifacts when the investigation
ends unless the user asked to retain them.

## Repair boundary

If repair is authorized, read and follow the
[feature-development skill](../feature-development/SKILL.md): first encode the
minimal reproduction as a regression test, observe it fail, implement the
smallest coherent correction, and keep refactoring inside the green loop. Verify
that the regression test fails for the original reason and that its expected
value is independent of the repaired implementation.

If repair is not authorized, stop after identifying the cause and the narrowest
supported remedy. Do not edit product code, tests, configuration, or external
state merely because the likely fix is clear.

## Stop and report

Stop when the cause is supported by a repeatable observation, when the authorized
repair and regression verification pass, or when further progress requires
missing access, unsafe mutation, unavailable data, or a product decision outside
the task. For a nondeterministic failure, use a bounded set of purposeful attempts
rather than open-ended retries.

Report the reproduction, evidence, cause or remaining hypotheses, scope affected,
commands run, cleanup performed, and next action. When blocked, name the exact
missing evidence or authority and the smallest step that would unblock it.
