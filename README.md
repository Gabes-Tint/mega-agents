# Mega Agents

A Go server with an embedded Svelte frontend (built into `internal/web/dist`
via `go:embed`).

## Setup

Install Go, Bun, Make, and Gitleaks, then:

```sh
make install  # after dependency or pinned-tool changes
make setup    # enables the tracked pre-commit hook
```

## Run

```sh
make build
./bin/mega-agents  # PORT=3000 to override the default 8080
```

Open <http://localhost:8080> (status endpoint: `/api/status`).

For frontend development with hot reload: `make dev-frontend`. Re-run
`make build` after frontend changes and restart the server.

## Checks

- `make verify` — types, Go and frontend tests with coverage, formatting,
  lint, static analysis, CI-doc sync, size budgets, and the embedded build.
- `make gates` — repository policy executables under `scripts/gates/`.
- `make smoke` — launches the app and probes the status endpoint.
- `make gate-self-test` — proves the gates reject bad fixtures.

When `Makefile`, `.github/workflows/*.yml`, or `scripts/gates/*` changes,
update `docs/ci-doc.md`. A behavior-preserving refactor may instead include
the content-bound `.ci-doc-no-impact` declaration described there.

## Agent evaluation

One deterministic, diagnosis-only scenario runs the selected agent tool in a
disposable Git worktree:

```sh
make agent-eval TOOL=opencode MODEL=opencode-go/glm-5.3-flash
```

The full per-tool examples and pass criteria are documented in
[`docs/agent-development.md`](docs/agent-development.md#agent-evaluation).

## Documentation

- [`docs/agent-development.md`](docs/agent-development.md) — development-agent
  operating model, agent evaluation, controls, and planned gaps.
- [`docs/ci-doc.md`](docs/ci-doc.md) — guardrail status and CI facts.
- [`docs/ci-files.md`](docs/ci-files.md) — index of CI-defining files.
- [`docs/custom-gates.md`](docs/custom-gates.md) — gate conventions and
  extension steps.
