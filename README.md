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
every stop handed to Gabriel or marked blocked.

Every saved workflow has its own address. **Open…** takes the editor to
`/workflows/<name>`, so a workflow can be bookmarked, shared or reopened by
link, and Back and Forward walk between the workflows that were open. **Save**
moves the address to the workflow it has just written, replacing the entry
instead of adding one, so Back never lands in the empty editor again. An
address that names no saved workflow says which one is missing and offers the
way back to the editor; the server answers 404 for it while still serving the
editor, and a missing asset stays a plain 404 so a broken build is not hidden
behind an HTML page.

The unsaved graph in progress is the page at `/`, and it is kept in the browser,
so reloading that page does not lose it; a saved workflow comes back from its
file instead.

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

A flow that outgrows the screen is zoomed between 25% and 200%: Ctrl or Cmd
with the wheel, or a trackpad pinch, zooms toward the pointer; Ctrl or Cmd
with **+**, **-** and **0** steps and resets it; and the controls in the
canvas's bottom right corner zoom in and out, show the level, reset it when
clicked, and **fit the whole flow on screen**. Blocks are dropped, moved,
resized and joined under the pointer at every level; arrows, their ends and
the handles around a block keep their size on screen, so they stay clickable
once the whole flow is in view. The level is remembered in this browser, like
the panel sizes.

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

A command-line run meets the breakpoints saved in the workflow too. At a
terminal it stops and asks — continue, step, skip or inspect what arrives —
one question at a time. With no terminal (CI, a piped run, `make e2e`) it
never waits for an answer that cannot come: it names the blocks whose
breakpoints it is running past on stderr and carries on, so a headless run
can never hang silently. `mega-agents runs show` says where a paused run
waits and the prompt or command it is about to run. `mega-agents retry` meets
the same breakpoints, except on the steps it reuses: a step that gives back
what it recorded does not run, so nothing stops before it.

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
  `MEGA_AGENTS_WORKSPACE_PATH|BRANCH|BASE|REPOSITORY` set. It takes the same
  placeholders an agent's prompt does and fills each one in before the shell
  sees it, as text wherever it lands: quoted as one word on its own, so
  `cat {{workspace.path}}/README.md` reads one path however it is spelled,
  and as the value itself inside quotes you wrote, so
  `echo "tests said {{results.tests}}"` prints the result. A value holding a
  quote, a dollar sign or a semicolon is always text the command reads, never
  shell it runs. Exit 0 leaves on
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
- **Breakpoints** stop a run *before* a block runs, like a debugger, so you
  see what it is about to receive and can stop an expensive agent before it
  spends anything. Tick **Breakpoint** in the selected block's properties,
  beside **Starting point**, or press **B** on the block on the canvas: it
  wears a red dot in its header from then on. A breakpoint is a property of
  the block (`breakpoint: true` in the workflow file, beside `start: true`),
  so it is saved with the flow and survives a reload. The run reports `paused`
  with the block it waits before, the values that arrived and the prompt or
  command it is about to run, and waits indefinitely: the block's clock only
  starts when it runs. Only that block waits — everything already running
  beside it keeps going, and blocks that do not depend on it still start.

  On the canvas the block the run waits before takes a pause mark and a ring
  that breathes, in the warning colour rather than the blue of a block that is
  working (still, but still marked, when the machine asks for less motion).
  The **Run** panel gives each waiting block a card: which repeat it is on,
  every value that reached it with the block and output it came from, and the
  prompt or command it would run, filled in, in a code editor. Editing that
  text is the **edit then run** of the list below; the card is marked *for
  this run only*, says so beside the editor, and offers **Undo the edit**,
  because nothing typed there is ever saved to the block. **Continue**,
  **Step** and **Skip** sit under the card and name their block, so with two
  branches waiting each card takes only its own block on. **Cancel run** stays
  in the toolbar while a run is paused. If the run has meanwhile moved on, the
  panel says what the backend answered and re-reads the run, so the canvas
  shows where it actually is.

  The same three actions, and the edit, are what `POST /api/runs/{id}/resume`
  takes:

  ```sh
  curl -X POST -H 'Content-Type: application/json' \
    -d '{"nodeId": "a1", "action": "continue", "prompt": "Write the smallest fix"}' \
    http://localhost:8080/api/runs/<run-id>/resume
  ```

  `continue` runs the block and goes on to the next breakpoint or the end,
  `step` runs it and stops before the next block, and `skip` settles it as
  skipped, which skips everything downstream of it as any other skip does. A
  `prompt` or `command` runs the block with that text instead, **for this run
  only**: the text is used as written, the saved workflow is never changed, and
  the step records both what ran and what the workflow holds, so a recorded run
  never misrepresents what was executed. `nodeId` may be left out when the run
  waits at a single block; the editor always names the block, so two waiting
  branches never make it guess. Resuming a run that is not paused is a
  conflict. **Cancel run** works while paused. A breakpoint on a block inside
  a loop stops on **every repeat**, and the pause says which repeat it is.
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
