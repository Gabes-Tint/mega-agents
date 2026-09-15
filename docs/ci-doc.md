# CI guardrails

This file records the deterministic guardrails used to develop Mega Agents,
where each check runs, and which controls are still planned. Its purpose is to
keep local pre-commit feedback and GitHub CI aligned without putting slow or
feed-dependent work in the commit loop. Update this inventory whenever a
guardrail is added, removed, or changes execution scope.

Legend: ✅ implemented · 🟡 partial · ⬜ not implemented

The **Where** column uses **Both** for pre-commit and required CI, **CI** for
required GitHub CI, **Scheduled** for weekly CI, and **Later** for the intended
placement of a guardrail that has not been implemented.

## Build, quality, and tests

| Status | Guardrail | Tools | Where | Current state |
| ---: | --- | --- | --- | --- |
| ✅ | Go formatting | `gofmt` | Both | Enforced locally and remotely |
| ✅ | Svelte/TypeScript diagnostics | `svelte-check` | Both | Enforced locally and remotely |
| ✅ | Go static analysis | `go vet` | Both | Enforced locally and remotely |
| 🟡 | Stronger Go analysis | `staticcheck` | Both later | Not configured |
| 🟡 | Frontend lint and formatting | ESLint, Prettier | Both later | Type checking exists; these tools do not |
| ✅ | Svelte production build | Bun, Vite | Both | Runs locally and remotely |
| ✅ | Embedded Go executable | `go build`, `go:embed` | Both | Produces the static executable |
| ⬜ | Go behavioral tests | `go test` | Both | Command runs, but no tests exist |
| ⬜ | Svelte component tests | Vitest, Testing Library | Both later | Not configured |
| ⬜ | API contract tests | OpenAPI or shared schema | Both later | Not configured |
| ⬜ | Browser smoke tests | Playwright | CI later | Not configured |
| ⬜ | Go fuzz tests | `go test -fuzz` | CI later | Not configured |
| ⬜ | Coverage thresholds | Go/Vitest coverage | CI later | Not configured |
| ⬜ | Mutation testing | Gremlins, Stryker | Scheduled later | Not configured |
| ⬜ | Artifact-size budgets | Custom script | CI later | Not configured |
| 🟡 | Test timeouts | GitHub Actions, test runners | CI | Job timeouts exist; test-level controls do not |

## Security and dependencies

| Status | Guardrail | Tools | Where | Current state |
| ---: | --- | --- | --- | --- |
| ✅ | Secret scanning | Gitleaks | Both | Staged changes locally; commits/history remotely |
| ✅ | Dependency vulnerability audit | `bun audit`, `govulncheck` | Scheduled | Weekly or manually triggered |
| ✅ | Dependency updates | Dependabot | Scheduled | Checks GitHub Actions weekly |
| ✅ | Reproducible dependencies | Bun lockfile | Both | Frozen lockfile used locally and remotely |
| ✅ | Immutable CI actions | Git commit SHAs | CI | Actions are hash-pinned |
| 🟡 | Source security analysis | CodeQL or Semgrep | CI later | Not configured beyond secret scanning |
| 🟡 | Dependency/license policy | License scanner | CI later | Vulnerabilities checked; licenses are not |
| ⬜ | Outbound-call safety | Static analysis/custom rules | Both later | Timeouts and cancellation are not enforced |

## Agent and CI controls

| Status | Guardrail | Tools | Where | Current state |
| ---: | --- | --- | --- | --- |
| ✅ | Shared verification | Make | Both | Both environments call `make verify` |
| ✅ | Staged snapshot verification | Git, shell script | Pre-commit | Checks exactly what will be committed |
| ✅ | Whitespace validation | `git diff --cached --check` | Pre-commit | Checks staged content |
| ✅ | Protected main branch | GitHub protection | CI | Verify and Security are required |
| ✅ | Parallel CI lanes | GitHub Actions | CI | Verify and Security run independently |
| ✅ | CI caching | GitHub cache actions | CI | Go, Bun, and security tools are cached |
| ✅ | Superseded-run cancellation | GitHub Actions | CI | Older branch runs are cancelled |
| ✅ | Local time budget | Timed pre-commit hook | Pre-commit | Complete hook currently runs in a few seconds |
| 🟡 | Worktree isolation | Git worktrees, `AGENTS.md` | Agent workflow | Policy exists; automation does not |
| ⬜ | Guardrail self-tests | Defect fixtures | CI later | Not implemented |
| ⬜ | Machine-readable reports | JSON gate runner | CI later | Not implemented |
| ⬜ | Failure-versus-crash reporting | Gate runner | CI later | Not implemented |
| ⬜ | Threshold ratchets | Baselines and scripts | CI later | Not implemented |
| ⬜ | Reject focused/skipped tests | Test-policy check | Both later | Add with test infrastructure |
| ⬜ | Change-aware verification | Git path detection | Both later | Not implemented |
| ⬜ | Local/CI contract test | `actionlint`, custom script | CI later | Not implemented |
| 🟡 | Generated-file freshness | Vite, Git comparison | Both later | Assets rebuild; no comparison check exists |
