# Agent development operating model

This inventory records the instructions, responsibilities, and deterministic
controls used by development agents working on Mega Agents. It distinguishes
what the repository enforces today from behavior that is only documented or
still needs a defined mechanism. Keep the always-visible policy concise; put
detailed, reusable workflows in skills and link to them from adapters.

This document describes the target operating model, but it does not itself make
a control effective. A row is green only when the stated capability exists at
the scope claimed. Repository policy gates are catalogued separately in the
[CI guardrail inventory](ci-doc.md) and operated through the
[custom gates guide](custom-gates.md).

Legend: ✅ implemented · 🟡 partially implemented · ⬜ not implemented

## Instructions and development workflow

| Status | Capability or guardrail | Mechanism or source | Current state | Target or gap |
| ---: | --- | --- | --- | --- |
| ✅ | Concise, always-visible policy layer | [`AGENTS.md`](../AGENTS.md) | Defines the repository-wide requirements agents must see before working: skill use, verification, gate placement, worktree isolation, secrets, test policy, hooks, and local/remote suite boundaries. | Keep it short and link detailed procedures rather than duplicating them. |
| ✅ | Canonical feature workflow | [Feature-development skill](../.agents/skills/feature-development/SKILL.md) | One canonical skill defines scope preservation, acceptance criteria, red-green-refactor, independently derived expectations, public behavioral seams, careful mocking, vertical slices, verification, and reporting. Documentation-only and styling-only work are explicitly exempt from test-first development. | Extend the canonical skill when a reusable workflow requirement changes. |
| ✅ | Acceptance criteria before implementation | Canonical feature-development skill | Agents translate requested behavior into observable acceptance criteria and identify the smallest proving test boundary before editing product code. | Preserve this as the entry condition for feature and bug work. |
| ✅ | Test-driven red-green-refactor | Canonical feature-development skill | Feature work must observe a behavioral test fail, make the smallest coherent change pass, and refactor while focused tests remain green; test expectations must not repeat production logic or tautological mocks. | Add deterministic enforcement only if a reliable method can distinguish meaningful red-green history without encouraging performative commits. |
| ✅ | Regression-first bug fixes | [Diagnosing-bugs skill](../.agents/skills/diagnosing-bugs/SKILL.md), canonical feature-development skill | Diagnosis uses a minimized, falsifiable evidence loop and keeps investigation separate from repair. An authorized fix starts with a regression test at a public behavior boundary. | Preserve the diagnosis-versus-repair authority boundary and regression-first default. |
| ✅ | Agent-instruction authoring | [Writing-agent-instructions skill](../.agents/skills/writing-agent-instructions/SKILL.md) | Agent-facing policy uses precise invocation pointers, concise always-loaded instructions, progressive disclosure, one canonical source, observable completion criteria, and deterministic gates only for repository-provable rules. | Audit for conflicts and duplication whenever the instruction architecture changes. |
| ✅ | Focused and final verification | [`AGENTS.md`](../AGENTS.md), canonical skill, [`Makefile`](../Makefile) | Agents run narrow tests while editing and `make verify` before reporting completion; `make smoke` is required by the skill when routing, startup, embedded assets, or the compiled application changes. | Keep verification commands stable and shared by agents, hooks, and CI. |
| ✅ | Prohibition on disabled or placeholder tests | [`AGENTS.md`](../AGENTS.md), [`check-test-policy.sh`](../scripts/gates/check-test-policy.sh) | Focused, skipped, and placeholder test patterns are prohibited and rejected by `make gates`, which runs inside `make verify`. | Expand rejection patterns only with deterministic fixtures and evidence of a real bypass. |
| 🟡 | Completion evidence | Canonical feature-development skill | Reports must name covered acceptance criteria, commands passed, assumptions, and limitations. | Not green because report contents are not structured or machine-checked; define a lightweight report contract if inconsistent handoffs become a problem. |

## Agent compatibility and deterministic enforcement

| Status | Capability or guardrail | Mechanism or source | Current state | Target or gap |
| ---: | --- | --- | --- | --- |
| ✅ | Shared instructions for Claude | [`CLAUDE.md`](../CLAUDE.md), thin adapters under [`.claude/skills/`](../.claude/skills/) | Claude imports `AGENTS.md`; every supported workflow has a same-name Claude adapter that points to its canonical skill rather than copying it. | Add an equally thin adapter family only when another supported agent needs a tool-specific discovery path. |
| 🟡 | Deterministic adapter enforcement | [`check-agent-adapters.sh`](../scripts/gates/check-agent-adapters.sh), `make gates` | The gate owns the exact supported skill inventory and rejects missing or unregistered canonical skills, missing `AGENTS.md` pointers, missing or misnamed Claude adapters, and broken canonical links. | Not green because linkage and inventory checks cannot prove semantic compliance by a running agent. |
| ✅ | Adapter rejection fixtures | [`scripts/test-gates.sh`](../scripts/test-gates.sh), `make gate-self-test` | Local fixtures prove a valid inventory passes and missing or misnamed skills, missing instruction links, missing or misdirected adapters, and unregistered entries fail. Required CI runs the fixture suite. | Add a focused rejection case for every new failure mode promised by the gate. |
| ✅ | Supported-agent instruction parity | Canonical skill inventory plus thin Claude adapters | Canonical workflows remain the source of truth and the gate keeps the currently supported Claude adapter set synchronized by name and reference. | Expand the declared inventory and fixtures whenever support for a canonical workflow or another adapter family changes. |

## Ownership, collaboration, and review

| Status | Capability or guardrail | Mechanism or source | Current state | Target or gap |
| ---: | --- | --- | --- | --- |
| ✅ | Task ownership and scope | Canonical feature-development and [agent-handoff](../.agents/skills/agent-handoff/SKILL.md) skills | Agents preserve user scope and a handoff names role, file and worktree ownership, destination integrator, current state, and next owner. | Keep ownership explicit whenever work is delegated or integrated. |
| ✅ | Agent roles and responsibilities | Canonical agent-handoff skill | Investigator, implementer, reviewer, and integrator have pragmatic authority boundaries. One agent may combine roles for small work but must identify authority changes. | Add roles only when a distinct authority boundary is demonstrated. |
| 🟡 | Collaboration and delegation | [`AGENTS.md`](../AGENTS.md), canonical agent-handoff skill | Concurrent agents use separate worktrees and generated output; delegated work identifies roles, file ownership, integrator, overlaps, and next action. | Not green because repository state cannot prove that every delegation declared ownership before editing. |
| 🟡 | Worktree isolation | Git worktrees, [`AGENTS.md`](../AGENTS.md) | Isolation is required in prose and verification caps local concurrency, but no automation proves worktree creation, cleanup, or generated-output separation. | Add lifecycle guidance and a deterministic check only where repository state can reliably demonstrate a violation. |
| ✅ | Handoff protocol | Canonical agent-handoff skill | Handoffs capture outcome, scope and ownership, relevant references and files, decisions, verification, blockers, repository state, and one owned next action. | Keep payloads concise and evidence-based. |
| ✅ | Context transfer | Canonical agent-handoff skill | Delegation and resumption include the request or acceptance references, constraints, assumptions, relevant files, verification evidence, and redacted repository state. | Never transfer credentials or treat context as expanded authority. |
| ✅ | Review responsibilities | [Code-review skill](../.agents/skills/code-review/SKILL.md), canonical agent-handoff skill | Reviews separately assess repository conformance and request fidelity; reviewers remain read-only unless edits are assigned, while integrators own overlap resolution and final state. | Require independent review only when risk or repository policy justifies it, not as universal ceremony. |
| ✅ | Escalation and user decisions | Diagnosing-bugs and agent-handoff skills | Work pauses for missing authority, unsafe mutation, ownership conflicts, contradictory state, unavailable evidence, or material product decisions; the handoff records evidence, decision owner, and resume condition. | Keep escalation proportional and avoid open-ended retry loops. |

## Safety, integrity, and operational controls

| Status | Capability or guardrail | Mechanism or source | Current state | Target or gap |
| ---: | --- | --- | --- | --- |
| ✅ | Secrets and public browser assets | [`AGENTS.md`](../AGENTS.md), canonical skill, Gitleaks through `make pre-commit` and required CI | Credentials must never be committed, examples use placeholders, browser-visible configuration is treated as public, staged changes are scanned locally, and commits/history are scanned remotely. | Keep sensitive values server-side and extend scanning policy when new secret-bearing systems are introduced. |
| ✅ | Guardrail integrity | [`AGENTS.md`](../AGENTS.md), `make verify` | Agents may not bypass failing hooks or weaken tests, coverage, or artifact budgets merely to pass; deliberate policy changes require review justification. | Preserve explicit review evidence for any intentional policy change. |
| 🟡 | Safe handling of unrelated changes | Worktree isolation and canonical agent-handoff skill | Separate worktrees reduce overlap; investigators inspect status and integrators verify the actual diff, preserve unrelated work, and report overlaps and repository state. | Not green because no deterministic check can prove that pre-existing dirty-tree changes were identified and preserved. |
| 🟡 | Local resource discipline | [`Makefile`](../Makefile), [`AGENTS.md`](../AGENTS.md) | Verification caps parallel Make work at two jobs and reserves expensive browser, race, mutation, and broad integration suites for remote CI. | Add measurable resource or duration budgets only when observed contention justifies them. |
| ⬜ | Machine-readable agent outcome | No repository mechanism | Verification artifacts exist, but agent decisions, ownership, handoffs, and completion evidence have no common structured record. | Introduce a small durable format only if it materially improves orchestration, auditability, or automated review. |
| ⬜ | Agent-behavior conformance evaluation | No repository mechanism | Gates validate repository state and adapter wiring, not whether an agent actually followed acceptance, test-first, reporting, or escalation instructions. | Add scenario-based evaluations for high-risk instruction failures before claiming behavioral enforcement. |

## Updating this inventory

Update this page when agent instructions, adapters, collaboration conventions, or
their enforcement change. A status should advance only when the mechanism covers
the full claim in that row. Changes to CI-defining files still follow the
separate update rules in [`ci-doc.md`](ci-doc.md); editing this inventory does
not satisfy the CI-documentation drift gate.
