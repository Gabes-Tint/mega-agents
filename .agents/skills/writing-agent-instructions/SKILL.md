---
name: writing-agent-instructions
description: Create or revise Mega Agents instructions consumed by coding agents, including AGENTS.md, canonical skills, adapters, and enforceable workflow policy. Do not use for ordinary user documentation.
---

# Writing agent instructions

Make the correct workflow easy to discover without loading irrelevant detail or
creating competing sources of truth.

## Place instructions deliberately

- Keep `AGENTS.md` concise and always applicable. Add a discriminating pointer
  that says when to open a detailed skill and, where confusion is likely, when
  not to use it.
- Keep each reusable workflow in one canonical skill under `.agents/skills/`.
  Tool-specific adapters should identify the same invocation boundary and point
  to the canonical source instead of copying its body.
- Use the skill name and frontmatter description as the first routing layer,
  `SKILL.md` for the shared workflow and invariants, and references only for
  substantial conditional detail. Link every reference where an agent can decide
  whether it is needed.
- Avoid a new file or layer when a narrow addition to an existing canonical
  instruction is clearer.

## Write operational contracts

- State the desired outcome, authority boundary, important invariants, and
  observable completion criteria. Use exact repository paths and commands where
  precision changes behavior.
- Prefer decision criteria over scripts of generic steps. Use absolute language
  only for safety, correctness, permissions, or fragile repository mechanics.
- Preserve the requester's chosen scope. Do not turn a past example or local
  preference into a universal requirement without evidence that it generalizes.
- Audit nearby instructions for duplicated, contradictory, or stale guidance.
  Resolve conflict in the canonical source and keep pointers thin.

## Enforce only provable rules

Add a deterministic gate only when repository state can prove the promised
invariant. Put gate logic under `scripts/gates/`, run it through `make gates`, and
follow `docs/custom-gates.md` for focused acceptance and rejection fixtures.
Do not claim that a text or linkage check proves an agent followed a behavioral
workflow.

Before completion, validate each new or substantially revised skill with the
available skill validator, run the focused policy fixtures for any gate change,
and run the repository checks required by `AGENTS.md`. Report what is enforced,
what remains guidance, and any invocation or compatibility limitation.
