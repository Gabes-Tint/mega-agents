# Mega Agents development

- Use test-driven development for behavior: observe the relevant test fail,
  implement the behavior, then refactor with tests passing.
- Include regression tests for bug fixes. Test observable behavior rather than
  mirroring implementation. Styling and documentation do not require test-first work.
- Run focused tests while editing and `make verify` before reporting completion.
- Use separate worktrees for concurrent agents. Do not share generated output.
- Never commit credentials. Browser assets are public. Use placeholders in examples.
- Install hooks with `make setup`. Do not bypass a failing hook; explain and fix it.
- Local checks include staged secret scanning, formatting checks, types, Go tests,
  and the embedded build. Keep expensive browser, race, mutation, and broad
  integration suites for remote CI. Measure local runtime before expanding checks.
