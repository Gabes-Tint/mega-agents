# Mega Agents

![Mega Agents visual workflow banner](docs/assets/mega-agents-social-preview.png)

Mega Agents is an early-stage, local-first platform for designing and running
bounded agent-development workflows against existing repositories. Its goal is
to combine a Go orchestration engine, a visual block editor, and portable YAML
workflows so the same flow can run from the web interface, CLI, or CI—with live
progress, explicit scope, and repository-native quality gates.

The current repository is the foundation of that product: a Go server with an
embedded Svelte frontend, agent-development policy and evaluation tooling, and
the product proposal that guides the next implementation stages. See the
[product foundation](docs/product/agent-development-platform.md) for the full
vision and constraints.

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

- [`docs/product/agent-development-platform.md`](docs/product/agent-development-platform.md)
  — product foundation for the proposed successor agent-development platform.
- [`docs/agent-development.md`](docs/agent-development.md) — development-agent
  operating model, agent evaluation, controls, and planned gaps.
- [`docs/ci-doc.md`](docs/ci-doc.md) — guardrail status and CI facts.
- [`docs/ci-files.md`](docs/ci-files.md) — index of CI-defining files.
- [`docs/custom-gates.md`](docs/custom-gates.md) — gate conventions and
  extension steps.
