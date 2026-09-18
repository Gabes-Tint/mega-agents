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

## Command line

The same binary runs workflows headless and inspects recorded runs:

```sh
./bin/mega-agents run workflow.yaml     # a workflow exported from the editor
./bin/mega-agents run graph.json        # or the editor's JSON graph
./bin/mega-agents workflows             # workflows saved from the editor
./bin/mega-agents run <name>            # run a saved workflow by name
./bin/mega-agents templates             # ready-made workflows
./bin/mega-agents init <template> [name]  # save a new workflow from one
./bin/mega-agents runs                  # recorded runs, newest first
./bin/mega-agents runs show <run-id>    # each step's status, error and details
./bin/mega-agents logs <run-id> [step]  # every step's log, or one by id or name
./bin/mega-agents retry <run-id>        # rerun what failed, reusing what succeeded
```

The editor's **Save** writes the workflow as YAML to
`$MEGA_AGENTS_HOME/workflows/<name>.yaml`; **Open…** loads one back and
**Import YAML** loads any exported file, and **Templates…** starts from a
ready-made workflow: issue to pull request, review and route, gate and fix,
or the **Fit_ development flow**, the fitflow driver drawn as blocks: pick a
story, whose call, slice, failing tests in a bounded loop, the capability
rung, implementation loops that escalate mechanic → builder → solver, the
pull request with review and CI fix rounds, merge, deploy and cleanup, with
every stop handed to Gabriel or marked blocked. The graph in progress is also kept in
the browser, so reloading the page does not lose it.

As the graph changes, the editor's **Problems** panel lists what to fix
before running: errors a run would stop on (a missing prompt, an arrow
without a route, no starting point) and warnings such as blocks no starting
point reaches. Choosing a problem selects its block; the status bar counts
them. Select a block and choose **Delete block**, or press Delete, to remove
it with everything inside it.

To connect two blocks, hover one and drag an arrow handle from any of its
sides onto the other; the block under the pointer turns green where the arrow
may go and red, along with the arrow, where it may not, and Escape cancels. An
arrow from a block with several outputs (a command, schema check, router or
loop) asks which output it takes. An arrow leaves each block on the side
facing the other one and runs in horizontal and vertical segments with
rounded corners, going around the blocks between them instead of cutting
across them, and its output label rides its longest straight run. Blocks
moved around the canvas are given way to as they go. Click an arrow to
select it, then press
Delete or choose **Delete arrow**. Hovering or selecting an arrow shows its
ends; drag either end onto another block to move it. Deleting an arrow
between Git actions does not rejoin their sequence: the actions after it stop
running until an arrow joins them again.

Every run, whether started from the editor or the command line, keeps its
record and the log of each step under `$MEGA_AGENTS_HOME/runs` (default
`~/.mega-agents/runs`). In the editor, select a block after a run and choose
**Show logs**, or use **Logs** beside a step in the run result. **Cancel run**
stops the run in progress (its agent CLIs included), **Runs…** reopens any
recorded run on the canvas, and Ctrl-C stops a command-line run; either way
the run is recorded as cancelled. **Retry from failure** (or `mega-agents
retry`) starts a new run of a failed, cancelled or interrupted run's workflow in
which the steps that already succeeded give back their recorded outputs
instead of running again.

## Blocks that run

Steps run as soon as everything they need has finished, up to four at a
time, so independent agents or gates work in parallel. Fetch, worktree
creation and push of one repository take turns, since they update its shared
refs.

- **GitHub** flagged as a starting point runs its **Actions** in order:
  *Fetch*, *Create worktree* (outputs a `workspace`), *Rebase*, *Read issue*
  (the issue's title, body, labels and comments via `gh`; issue number 0, the
  default, reads the next available issue: the oldest open one without a
  label to ignore and not assigned to anyone but the `gh` user),
  *Commit*, *Push*
  and *Open pull request* (via `gh`). Without actions it fetches. Runs use the
  machine's existing Git and `gh` credentials. Agents and commands that
  receive a workspace pass it on, so worktree → agent → Commit → Push → Open
  pull request is one chain; blocks beside a GitHub block may connect into its
  actions.
- **Agent** runs one conversation with a coding-agent CLI (Claude Code, Codex,
  Grok or OpenCode) in the workspace connected to it, or in its project's
  folder. Its prompt may use `{{workspace.path}}`, `{{workspace.branch}}`,
  `{{workspace.base}}`, `{{workspace.repository}}`, and the results of
  connected agents as `{{result}}` or `{{results.<agent>}}`. With an
  **Output schema** (JSON Schema, strict subset) the reply is validated and
  repaired on the same session up to **Retries** times, unless its
  **Max cost (USD)** is already spent. Each agent step reports the tokens it
  used and, where the CLI prices it, its cost; run results and
  `mega-agents runs show` add them up. **Continue the connected agent's
  conversation** resumes a copy (fork) of the connected agent's session, so a
  fixer keeps everything the coder learned. An agent flagged as a starting point
  starts a run on its own.

- **JSON Schema** (`output/json-schema@v1`) sits inside an agent and validates
  that agent's reply against any JSON Schema, with no arrow into it. A valid
  value leaves on its `valid` output; an invalid one leaves on `invalid` with
  the value and field-level errors when an arrow takes that branch, and
  otherwise fails the run. Its arrows lead to the blocks beside the agent;
  choose each arrow's output in the block's properties.

- **Command** runs a shell command (a test suite, a linter, `make verify`) in
  the workspace connected to it or its project's folder, with
  `MEGA_AGENTS_WORKSPACE_PATH|BRANCH|BASE|REPOSITORY` set. Exit 0 leaves on
  `passed`; anything else leaves on `failed` with `{exitCode, output}` when an
  arrow takes it (for example to a fixer agent), and otherwise fails the run.
- **Loop** repeats the blocks inside it (agents with their schema checks,
  commands, routers) until the block chosen under **Ends when** takes the chosen
  output, at most **Repeat at most** times (3 by default, up to 20). The
  blocks inside that no block inside feeds receive the arrows into the loop
  on every repeat, so a loop holding *Gate* → `failed` → *Fixer* ends when
  *Gate* takes `passed`. The loop sends that value on `done`, or the last
  value on `exhausted` (failing the run when no arrow takes it), and passes a
  workspace on. An agent inside that continues a conversation continues the
  agent feeding the loop's on the first repeat and its own afterwards. The
  canvas shows which repeat a running loop is on, and each block's log marks
  every repeat.
- **Run when any arrow arrives** (agents, commands and loops) joins
  exclusive branches again: the block runs as soon as its incoming blocks
  have settled if any of them arrived, with `{{result}}` naming the one that
  did, instead of being skipped because a branch was not taken.
- **Router** sends the one value connected to it down the first case whose
  CEL condition over `value` holds (for example `value.verdict == "approve"`),
  or down `default`, as `{"case": ..., "value": ...}`. Expressions are
  type-checked when the run is planned and evaluated under a cost limit;
  arrows on routes not taken are skipped.

The **Command**, **Prompt**, **Output schema** and **Schema** fields are code
editors rather than plain text boxes. They take as many lines as the text
needs, highlight a command as shell and the two schema fields as JSON, and mark
`{{placeholders}}` out of the syntax around them in all four. Typing `{{`, or
`$` in a command, or pressing Ctrl-Space offers the variables that actually
reach the selected block: the workspace fields when a worktree reaches it,
`{{result}}` and `{{results.<block>}}` for the blocks connected to it, and the
`MEGA_AGENTS_WORKSPACE_*` environment variables in a command. Enter or Tab
accepts the highlighted variable and Escape closes the list; with no list open
Tab still moves to the next field rather than indenting.

| Backend | CLI | Schema | Verified live |
| --- | --- | --- | --- |
| `claude` | `claude --print --output-format stream-json` | `--json-schema` | ✅ haiku |
| `codex` | `codex exec --json` | `--output-schema` file | ✅ default model |
| `opencode` | `opencode run --format json` | in the prompt, validated | ✅ `opencode-go/glm-5.3-flash` |
| `grok` | `grok --output-format json` | `--json-schema` | ⬜ unit-tested only (no credits) |

Agents run with their CLI's non-interactive permission bypass inside the
workspace, like the fitflow driver's workers; point them at worktrees, not at
checkouts you care about.

The status bar shows a logo for each backend: in color when it answered the
startup check, greyed out when it did not (hover for why), pulsing while it is
being checked. Click a logo to check again. When the server starts, each
backend whose CLI is on `PATH` gets one tiny prompt (`Reply with exactly: ACK`)
on its cheapest model (`haiku` for Claude Code, `opencode-go/glm-5.3-flash` for
OpenCode, the default model for Codex and Grok), with a 60-second limit. That
costs one small model turn per installed backend. Set
`MEGA_AGENTS_SKIP_AGENT_CHECK=1` to turn it off. `make dev` turns it off by
default, since the backend restarts on every Go change; run
`MEGA_AGENTS_SKIP_AGENT_CHECK=0 make dev` to check the backends.

## Checks

- `make verify` — types, Go and frontend tests with coverage, formatting,
  lint, static analysis, CI-doc sync, size budgets, and the embedded build.
- `make gates` — repository policy executables under `scripts/gates/`.
- `make smoke` — launches the app, probes the status endpoint, and runs the
  `runs` command against an empty home.
- `make e2e` — Playwright specs against the dev server and a real Go backend.
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
