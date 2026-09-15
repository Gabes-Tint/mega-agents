# Mega Agents development

- For feature implementation and bug fixes, read and follow
  [the shared feature-development skill](.agents/skills/feature-development/SKILL.md).
- For diagnosis without an authorized fix, read and follow the
  [diagnosing-bugs skill](.agents/skills/diagnosing-bugs/SKILL.md).
- When writing agent-consumed policy, skills, or adapters, read and follow the
  [writing-agent-instructions skill](.agents/skills/writing-agent-instructions/SKILL.md).
- For code-review requests, read and follow the
  [code-review skill](.agents/skills/code-review/SKILL.md).
- For delegated, paused, reviewed, or integrated work, read and follow the
  [agent-handoff skill](.agents/skills/agent-handoff/SKILL.md).
- Run focused tests while editing and `make verify` before reporting completion.
- Keep repository policy gates in `scripts/gates/`, run them through `make gates`,
  and follow `docs/ci-doc.md` when changing CI-defining inputs.
- Use separate worktrees for concurrent agents. Do not share generated output.
- Never commit credentials. Browser assets are public. Use placeholders in examples.
- Never focus, skip, or leave placeholder tests. Do not lower coverage or artifact
  budgets to make a change pass; justify a deliberate policy change in review.
- Install hooks with `make setup`. Do not bypass a failing hook; explain and fix it.
- Local checks include staged secret scanning, formatting checks, types, Go tests,
  frontend tests, coverage, static analysis, and the embedded build. Keep expensive browser, race, mutation, and broad
  integration suites for remote CI. Measure local runtime before expanding checks.
