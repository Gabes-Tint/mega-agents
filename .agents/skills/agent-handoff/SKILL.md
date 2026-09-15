---
name: agent-handoff
description: Prepare or consume Mega Agents handoffs for delegated, paused, reviewed, or integrated work with explicit ownership, evidence, repository state, and next action.
---

# Agent handoff

Transfer enough verified context for another agent to continue safely without
reconstructing decisions or assuming authority that was never granted. Small
tasks may combine roles; delegated work must state who owns which role and files.

## Roles and authority

- **Investigator:** reproduces and explains a problem. It does not implement a
  fix unless repair is explicitly assigned.
- **Implementer:** changes only the assigned behavior and file scope, verifies
  it, and reports overlaps or decisions it cannot safely resolve.
- **Reviewer:** independently assesses repository conformance and request
  fidelity. Review alone does not authorize editing the work.
- **Integrator:** owns the destination repository state, resolves overlaps,
  preserves unrelated changes, and runs or confirms final verification. It may
  reject incomplete or conflicting handoffs.

One agent may hold several roles for small work, but should name the role change
when it changes authority, such as moving from diagnosis to an authorized fix.
For concurrent work, assign distinct worktrees and file ownership before edits;
do not share generated output.

## Handoff contract

Provide a concise payload containing:

- outcome and current state, including whether work is complete, continuing,
  blocked, or awaiting a decision;
- task scope, current role, ownership boundaries, and the destination integrator;
- relevant files and links to the request, acceptance criteria, specification,
  or decision record;
- decisions and assumptions that materially shaped the work;
- verification evidence with exact commands, results, and checks not run;
- blockers, unresolved work, known risks, and the smallest required decision or
  missing input;
- repository and worktree state, including commits, uncommitted changes,
  generated artifacts, overlaps, and whether changes are already integrated;
- one explicit next action and its intended owner.

Never include credentials, tokens, private keys, sensitive payloads, or raw logs
that contain them. Redact secrets while retaining only the structure needed to
understand the result.

Before sending, inspect the actual status and diff relevant to the owned files;
do not rely on memory. Before consuming, verify paths, repository state, and
authority boundaries. Preserve unrelated changes and do not treat a handoff as
permission for external mutation, merging, committing, or broader fixes.

Pause and escalate when ownership overlaps, the destination state contradicts
the handoff, a material product decision is missing, or the next action exceeds
the assigned authority. Record the evidence, decision owner, and exact condition
needed to resume.
