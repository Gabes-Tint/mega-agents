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

## Measured feedback time

Measurements from September 15, 2026 establish the current baseline. GitHub's
Verify and Security jobs run in parallel, so the slower Verify job determines
the overall CI duration.

| Environment | Cache state | Result |
| --- | --- | ---: |
| Local staged pre-commit | Warm development environment | **5.9 seconds** |
| Pull-request Verify | Cold cache for new analysis tools | **1 minute 30 seconds** |
| Pull-request Verify | Warm cache | **1 minute 12 seconds** |
| `main` Verify | Warm cache | **1 minute 1 second** |
| `main` Security | Warm cache, parallel with Verify | **17 seconds** |
| `main` overall CI | Warm cache | **1 minute 1 second** |

These are observations, not permanent budgets. Re-measure after materially
changing dependencies, test volume, runners, or cache configuration. Keep the
local hook near its current duration so agents receive feedback before pushing,
while leaving expensive and feed-dependent checks in remote or scheduled CI.

## Build, quality, and tests

| Status | Guardrail | Tools | Where | Current state |
| ---: | --- | --- | --- | --- |
| ✅ | Go formatting | `gofmt` | Both | Enforced locally and remotely |
| ✅ | Svelte/TypeScript diagnostics | `svelte-check` | Both | Enforced locally and remotely |
| ✅ | Go static analysis | `go vet` | Both | Enforced locally and remotely |
| ✅ | Stronger Go analysis | `staticcheck` | Both | Enforced locally and remotely |
| ✅ | Frontend lint and formatting | ESLint, Prettier | Both | Enforced locally and remotely |
| ✅ | Svelte production build | Bun, Vite | Both | Runs locally and remotely |
| ✅ | Embedded Go executable | `go build`, `go:embed` | Both | Produces the static executable |
| ✅ | Go behavioral tests | `go test` | Both | Status API and static asset serving are tested |
| ✅ | Svelte component tests | Vitest, Testing Library | Both | Success and failure states are tested |
| 🟡 | API contract tests | Go and Svelte behavioral tests | Both | Response shape is exercised on both sides; no shared schema exists |
| 🟡 | Compiled-application smoke test | Shell, curl | CI | The real binary and embedded page are probed without a browser |
| ⬜ | Browser smoke tests | Playwright | CI later | Not configured |
| ⬜ | Go fuzz tests | `go test -fuzz` | CI later | Not configured |
| 🟡 | Coverage thresholds | Go coverage, Vitest V8 coverage | Both | 80% applies to backend application and tested frontend source; no changed-code policy |
| ⬜ | Mutation testing | Gremlins, Stryker | Scheduled later | Not configured |
| ✅ | Artifact-size budgets | Custom shell gate | Both | Go binary, frontend JavaScript, and CSS have explicit limits |
| ✅ | Test timeouts | Go, Vitest, GitHub Actions | Both | Go, frontend, and CI jobs have explicit limits |

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
| 🟡 | Network timeout safety | Browser abort signal, Go HTTP server | Both | Current browser request and server headers have timeouts; no general enforcement rule |

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
| ✅ | Local time budget | Timed pre-commit hook | Pre-commit | Complete staged hook measured at 5.9 seconds |
| 🟡 | Worktree isolation | Git worktrees, `AGENTS.md` | Agent workflow | Policy exists; automation does not |
| 🟡 | Guardrail self-tests | Defect fixtures | CI | Test-policy and artifact-size gates have rejection fixtures |
| 🟡 | Machine-readable reports | Coverage JSON and Go profiles | Both | Coverage artifacts exist; no unified gate report |
| ⬜ | Failure-versus-crash reporting | Gate runner | CI later | Not implemented |
| ⬜ | Threshold ratchets | Baselines and scripts | CI later | Not implemented |
| ✅ | Reject focused/skipped tests | Custom shell gate | Both | Frontend and Go disabled-test patterns fail verification |
| ⬜ | Change-aware verification | Git path detection | Both later | Not implemented |
| 🟡 | Local/CI contract test | `actionlint`, shared Make target | Both | Workflows are linted and reuse `make verify`; no structural parity test |
| 🟡 | Generated-file freshness | Vite, Git comparison | Both later | Assets rebuild; no comparison check exists |
