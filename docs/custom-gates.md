# Custom gates

Custom gates enforce repository-specific policy that ordinary formatters,
compilers, and linters do not cover. Every policy rule has one executable in
[`scripts/gates/`](../scripts/gates/). Generic build, smoke, and staged-snapshot
helpers stay in [`scripts/`](../scripts/).

Run every custom policy rule through the shared entry point:

```sh
make gates
```

`make verify` invokes this target after the standard frontend, Go, and workflow
checks. The staged pre-commit snapshot and GitHub Verify job both call
`make verify`, so they enforce the same gate set.

## Current gates

| Gate | Policy |
| --- | --- |
| [`check-test-policy.sh`](../scripts/gates/check-test-policy.sh) | Rejects focused, skipped, and placeholder frontend tests and skipped Go tests. |
| [`check-agent-adapters.sh`](../scripts/gates/check-agent-adapters.sh) | Keeps the supported canonical skill inventory, `AGENTS.md` pointers, and matching thin Claude adapters synchronized. |
| [`check-ci-documentation.sh`](../scripts/gates/check-ci-documentation.sh) | Requires accountability for changes to CI-defining inputs and always validates the generated CI facts against configuration. |
| [`check-artifact-sizes.sh`](../scripts/gates/check-artifact-sizes.sh) | Enforces explicit size budgets for the Go executable and built JavaScript and CSS. |

The [`Makefile`](../Makefile) owns each gate's target and composes them under
`gates`. Build-dependent gates may depend on the artifact they inspect; policy
logic itself belongs in the gate executable.

## Rejection fixtures

Run the focused fixture suite with:

```sh
make gate-self-test
```

[`scripts/test-gates.sh`](../scripts/test-gates.sh) creates temporary defects and
proves the gates reject them. Agent fixtures cover missing or misnamed canonical
skills, missing `AGENTS.md` and `CLAUDE.md` links, missing or misnamed Claude
adapters, misdirected canonical pointers, and unregistered skills on either side
of the compatibility inventory. The suite also includes valid agent and
CI-documentation cases so the gates do not reject supported workflows. Fixtures
must remain deterministic, local, and independent of network services.

When changing a gate, first add or update a fixture that demonstrates the
observable behavior. Preserve at least one rejection case for each failure mode
that the rule promises to detect. Never weaken a fixture or budget merely to
make verification pass.

## Adding or changing a gate

1. Add one executable `scripts/gates/check-<policy>.sh` with deterministic error
   messages and a nonzero exit status when it detects a violation.
2. Add its Make target and include that target in `make gates`.
3. Add focused acceptance and rejection coverage to `scripts/test-gates.sh`.
4. Update [`ci-doc.md`](ci-doc.md), this guide, and the
   [`CI file index`](ci-files.md) when its inventory or documented behavior
   changes.
5. Run `make gate-self-test`, the new gate directly where useful, and finally
   `make verify`.

Adding or modifying a gate is itself a monitored CI change, so the
CI-documentation gate will enforce step 4.

## CI-documentation drift workflow

The drift gate monitors [`Makefile`](../Makefile), workflow YAML directly under
[`.github/workflows/`](../.github/workflows/), and files directly under
[`scripts/gates/`](../scripts/gates/). The exact current inventory is listed in
[`ci-files.md`](ci-files.md).

For behavior or factual changes, update [`ci-doc.md`](ci-doc.md) in the same
change. If the machine-checked facts change, regenerate their bounded section:

```sh
scripts/gates/check-ci-documentation.sh --write-facts .
```

Review the generated result before staging it. Touching the document is not an
override: factual disagreement between the document, Makefile, workflows, and
gate configuration still fails.

### Declaring no documentation impact

Use `.ci-doc-no-impact` only for a refactor that preserves every documented CI
behavior and fact. The declaration must itself be
changed in the same change and use this exact structure:

```text
CI_DOCUMENTATION_NO_IMPACT_V1
reason=A specific explanation of why documented CI behavior is unchanged.
<sha256>  path/to/changed-ci-input
```

The reason must contain at least 21 characters. List exactly every changed
monitored input in lexical order, using its current SHA-256 and two spaces before
the path. For a deleted input, use `DELETED` instead of a hash. Generate a hash
with `sha256sum path/to/file`; do not copy an earlier declaration.

The gate rejects missing, stale, extra, or incorrectly hashed entries. A valid
declaration waives only the requirement to edit `ci-doc.md`; it never waives the
machine-checked factual comparison. Prefer a normal documentation update if a
reviewer could reasonably learn anything new about CI behavior from the change.
