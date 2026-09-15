# Mega Agents development

- For feature implementation and bug fixes, read and follow
  [the shared feature-development skill](.agents/skills/feature-development/SKILL.md).
- Run focused tests while editing and `make verify` before reporting completion.
- Use separate worktrees for concurrent agents. Do not share generated output.
- Never commit credentials. Browser assets are public. Use placeholders in examples.
- Never focus, skip, or leave placeholder tests. Do not lower coverage or artifact
  budgets to make a change pass; justify a deliberate policy change in review.
- Install hooks with `make setup`. Do not bypass a failing hook; explain and fix it.
- Local checks include staged secret scanning, formatting checks, types, Go tests,
  frontend tests, coverage, static analysis, and the embedded build. Keep expensive browser, race, mutation, and broad
  integration suites for remote CI. Measure local runtime before expanding checks.
