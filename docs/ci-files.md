# CI file index

This index maps the files that define Mega Agents CI behavior. The
CI-documentation drift gate monitors the three path groups below. A change in
any group must also update [`ci-doc.md`](ci-doc.md) or include the validated
no-impact declaration described in [`custom-gates.md`](custom-gates.md).

## Monitored CI-defining inputs

| Monitored path or pattern | Current files | Role |
| --- | --- | --- |
| [`Makefile`](../Makefile) | [`Makefile`](../Makefile) | Defines local installation, verification, gate, build, smoke, and staged-check entry points reused by CI. |
| `.github/workflows/*.yml` | [`ci.yml`](../.github/workflows/ci.yml), [`dependency-audit.yml`](../.github/workflows/dependency-audit.yml) | Defines required push/pull-request verification and security jobs, plus the scheduled and manual dependency audit. Any workflow YAML added directly to this directory is monitored automatically. |
| `scripts/gates/*` | [`check-agent-adapters.sh`](../scripts/gates/check-agent-adapters.sh), [`check-artifact-sizes.sh`](../scripts/gates/check-artifact-sizes.sh), [`check-ci-documentation.sh`](../scripts/gates/check-ci-documentation.sh), [`check-test-policy.sh`](../scripts/gates/check-test-policy.sh) | Contains one executable for each repository-specific policy rule. The adapter gate owns the supported canonical-skill inventory and its Claude parity checks. Any file added directly to this directory is monitored automatically. |

The patterns intentionally cover only direct files in the workflow and gate
directories. If either area gains nested directories, update the drift gate and
this index together so the intended coverage remains explicit.

## Related CI files

These files support CI but do not themselves trigger the drift gate:

- [`scripts/test-gates.sh`](../scripts/test-gates.sh) owns positive and rejection
  fixtures for custom gates and runs through `make gate-self-test`.
- [`scripts/verify-staged.sh`](../scripts/verify-staged.sh) builds an isolated
  snapshot of the Git index and passes its changed-file list into verification.
- [`README.md`](../README.md) gives contributors the short local and remote CI
  workflow.
- [`AGENTS.md`](../AGENTS.md) gives coding agents the required gate workflow.
- [`ci-doc.md`](ci-doc.md) is the authoritative guardrail inventory and contains
  the generated, machine-checked factual section.
- [`custom-gates.md`](custom-gates.md) explains how to operate and extend custom
  policy gates.

Generic helpers such as [`scripts/smoke.sh`](../scripts/smoke.sh) remain outside
`scripts/gates/` because they perform build or runtime verification rather than
enforce a repository-specific policy rule.
