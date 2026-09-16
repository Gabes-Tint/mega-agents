# CI guardrails

This file records the deterministic guardrails used to develop Mega Agents,
where each check runs, and which controls are still planned. Its purpose is to
keep local pre-commit feedback and GitHub CI aligned without putting slow or
feed-dependent work in the commit loop. Update this inventory whenever a
guardrail is added, removed, or changes execution scope.

See the [CI file index](ci-files.md) for the monitored configuration paths and
the [custom gates guide](custom-gates.md) for operating and extending repository
policy gates.

`make gates` runs every repository-specific policy gate. Their executables live
in `scripts/gates/`; generic build, staged-snapshot, and smoke helpers remain in
`scripts/`. `make verify` invokes `make gates`, so the same policies run in the
staged pre-commit snapshot and required CI. Each major verification phase emits
a `✅` success or `❌` failure status so local and CI logs identify progress and
the failing layer at a glance.

Changes to `Makefile`, workflow YAML under `.github/workflows/`, or an executable
in `scripts/gates/` must update this document in the same change. A refactor
with genuinely no documentation impact may instead change `.ci-doc-no-impact`.
That declaration must start with `CI_DOCUMENTATION_NO_IMPACT_V1`, contain a
specific `reason=` of at least 21 characters, and list exactly every changed CI
input as `<sha256><two spaces><path>`. Use `DELETED` in place of the hash for a
deleted input. The gate requires the declaration itself to be part of the
change, so old declarations cannot grant an exemption. This mechanism only
waives the document-touch requirement: the machine-checked facts below are
always validated and cannot be waived. Prefer updating this document whenever
behavior or any documented fact changes.

Run `scripts/gates/check-ci-documentation.sh --write-facts .` after an intentional
configuration change, then review and stage the result. The generated block is
bounded by markers; the gate compares it exactly and separately verifies its
claims against the Makefile and workflows.

Legend: ✅ implemented · 🟡 partial · ⬜ not implemented

The **Where** column names the exact execution point: **Pre-commit + required
CI**, **Pre-commit only**, **Required CI only**, **Weekly + manual CI**, or a
planned location for a guardrail that has not been implemented.

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

The measurements above predate the browser smoke tests joining the Verify job
(September 15, 2026). The three Playwright specs run in about 2 seconds locally
with a cached browser; a cache miss adds the browser download. Re-measure the
Verify duration once this change runs in CI.

## Build, quality, and tests

| Status | Guardrail | Tools | Where | Current state |
| ---: | --- | --- | --- | --- |
| ✅ | Go formatting | `gofmt` | Pre-commit + required CI | Enforced locally and remotely |
| ✅ | Svelte/TypeScript diagnostics | `svelte-check` | Pre-commit + required CI | Enforced locally and remotely |
| ✅ | Go static analysis | `go vet` | Pre-commit + required CI | Enforced locally and remotely |
| ✅ | Stronger Go analysis | `staticcheck` | Pre-commit + required CI | Enforced locally and remotely |
| ✅ | Frontend lint and formatting | ESLint, Prettier | Pre-commit + required CI | Enforced locally and remotely |
| ✅ | Svelte production build | Bun, Vite | Pre-commit + required CI | Runs locally and remotely |
| ✅ | Embedded Go executable | `go build`, `go:embed` | Pre-commit + required CI | Produces the static executable |
| ✅ | Go behavioral tests | `go test` | Pre-commit + required CI | Status API, static asset serving, and the fake-OpenCode agent-evaluation path are tested |
| ✅ | Svelte component tests | Vitest, Testing Library | Pre-commit + required CI | Success and failure states are tested |
| 🟡 | API contract tests | Go and Svelte behavioral tests | Pre-commit + required CI | Not green because each side tests its own response assumptions; there is no shared schema that can detect contract drift automatically |
| 🟡 | Compiled-application smoke test | Shell, curl | Required CI only | Not green because it probes the real binary and embedded page over HTTP but does not exercise them in a browser |
| ✅ | Browser smoke tests | Playwright | Required CI only | Real-mouse drag-and-drop specs (palette drop, node move, scrolled-canvas placement, dialog dismissal) run in the required Verify job; jsdom cannot exercise the HTML5 drag pipeline, so the browser suite covers what component tests cannot. A `make e2e` convenience target runs the same specs locally without touching the pre-commit budget |
| ⬜ | Go fuzz tests | `go test -fuzz` | Planned scheduled CI | Not green because the current API has no complex parser or untrusted structured input that would provide a valuable fuzz target |
| 🟡 | Coverage thresholds | Go coverage, Vitest V8 coverage | Pre-commit + required CI | Not green because the 80% floor covers backend application and tested frontend source, but there is no changed-code coverage policy |
| ⬜ | Mutation testing | Gremlins, Stryker | Planned scheduled CI | Not green because the test suite is still small; mutation runtime and maintenance are not justified until more domain behavior exists |
| ✅ | Artifact-size budgets | Custom shell gate | Pre-commit + required CI | Go binary (16 MiB), frontend JavaScript (110 KiB, re-baselined from 100 KiB when the SVG edge layer landed), and frontend CSS (50 KiB) have explicit limits |
| ✅ | Test timeouts | Go, Vitest, GitHub Actions | Pre-commit + required CI | Go, frontend, and CI jobs have explicit limits |

## Security and dependencies

| Status | Guardrail | Tools | Where | Current state |
| ---: | --- | --- | --- | --- |
| ✅ | Secret scanning | Gitleaks | Pre-commit + required CI | Staged changes locally; commits/history remotely |
| ✅ | Dependency vulnerability audit | `bun audit`, `govulncheck` | Weekly + manual CI | Weekly or manually triggered |
| ✅ | Dependency updates | Dependabot | Weekly | Checks GitHub Actions weekly |
| ✅ | Reproducible dependencies | Bun lockfile | Pre-commit + required CI | Frozen lockfile used locally and remotely |
| ✅ | Immutable CI actions | Git commit SHAs | Required CI only | Actions are hash-pinned |
| 🟡 | Source security analysis | CodeQL or Semgrep | Planned CI | Not green because only secret scanning is configured; a source analyzer and its false-positive policy have not been selected |
| 🟡 | Dependency/license policy | License scanner | Planned CI | Not green because vulnerabilities are audited, but allowed and prohibited dependency licenses have not been defined |
| 🟡 | Network timeout safety | Browser abort signal, Go HTTP server | Pre-commit + required CI | Not green because current request and server-header timeouts are tested indirectly; there is no repository-wide rule covering every future network operation |

## Agent and CI controls

| Status | Guardrail | Tools | Where | Current state |
| ---: | --- | --- | --- | --- |
| ✅ | Shared verification | Make | Pre-commit + required CI | Both environments call `make verify` |
| ✅ | Staged snapshot verification | Git, shell script | Pre-commit only | Checks exactly what will be committed |
| ✅ | Whitespace validation | `git diff --cached --check` | Pre-commit only | Checks staged content |
| ✅ | Protected main branch | GitHub protection | Required CI only | Verify and Security are required |
| ✅ | Parallel CI lanes | GitHub Actions | Required CI only | Verify and Security run independently |
| ✅ | CI caching | GitHub cache actions | Required CI only | Go, Bun, and security tools are cached |
| ✅ | Superseded-run cancellation | GitHub Actions | Required CI only | Older branch runs are cancelled |
| ✅ | Local time budget | Timed pre-commit hook | Pre-commit only | Complete staged hook measured at 5.9 seconds |
| 🟡 | Worktree isolation | Git worktrees, `AGENTS.md` | Agent workflow | Not green because agents are instructed to isolate work, but no automation verifies worktree creation, cleanup, or shared-output violations |
| ✅ | Deterministic agent scenario evaluation | Go, fake OpenCode | Pre-commit + required CI | One diagnosis-only scenario verifies isolated worktree execution, evidence capture, deterministic scoring, and unauthorized-change detection; real model runs remain manual |
| 🟡 | Guardrail self-tests | Defect fixtures | Required CI only | Rejection fixtures cover test policy, artifact sizes, the supported agent-skill inventory and adapter links, and CI-documentation drift; not every future custom gate is automatically covered |
| 🟡 | Machine-readable reports | Coverage JSON and Go profiles | Pre-commit + required CI | Not green because coverage artifacts exist, but there is no single structured report summarizing all gate outcomes |
| ⬜ | Failure-versus-crash reporting | Gate runner | Planned CI | Not green because there is no unified gate runner to distinguish a detected defect from an infrastructure or tool crash |
| ⬜ | Threshold ratchets | Baselines and scripts | Planned CI | Not green because the project has too little historical data to set meaningful improving baselines without arbitrary limits |
| ✅ | Reject focused/skipped tests | Custom shell gate | Pre-commit + required CI | Frontend and Go disabled-test patterns fail verification |
| ⬜ | Change-aware verification | Git path detection | Planned pre-commit + CI | Not green because the full suite is currently fast and small; path mapping would add complexity before it saves meaningful time |
| ✅ | Local/CI contract test | `actionlint`, CI-documentation gate | Pre-commit + required CI | Workflows are linted, required CI must call `make verify`, and verification must call the shared `make gates` entry point |
| ✅ | CI-documentation drift | Custom shell gate, content-bound declaration | Pre-commit + required CI | CI-defining changes require updated documentation or an exact hashed no-impact declaration; factual checks cannot be waived |
| ✅ | Canonical agent-skill parity | Custom shell gate | Pre-commit + required CI | The declared canonical skill set, concise `AGENTS.md` pointers, and same-name Claude adapters must remain complete; unregistered canonical skills or adapters are rejected |
| 🟡 | Generated-file freshness | Vite, Git comparison | Planned pre-commit + CI | Deferred until generated assets become committed artifacts. Today they are untracked and rebuilt before Go compiles, so a freshness comparison would add no protection; if we commit them later, add a clean-tree comparison proving they match their sources |

## Machine-checked CI facts

<!-- ci-facts:start -->
| Machine-checked fact | Configuration |
| --- | --- |
| Required verification entry point | `make verify` |
| Repository policy entry point | `make gates` (test policy, agent adapters, CI-documentation drift, artifact sizes) |
| Custom gate rejection tests | `make gate-self-test` |
| Compiled application smoke test | `make smoke` |
| Required CI workflows | `CI` (Verify + Security), `Dependency audit` (weekly + manual) |
| Required vulnerability checks | `bun audit`, `govulncheck ./...` |
<!-- ci-facts:end -->
