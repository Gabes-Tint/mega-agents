# Successor agent-development platform

Status: product foundation proposal

This document captures the product intent and lessons that should guide a
successor to Mega Agents. It deliberately defines outcomes, constraints, and
non-goals before package boundaries or implementation choices. Architectural
decisions should be recorded separately once those choices are made.

## Vision

Build a repository-agnostic development tool that prepares an existing
repository for reliable agent coding and then carries bounded development work
through implementation and verification. It should provide both a CLI and a
web interface over the same engine.

The product combines three proven ideas:

- Mega Agents' repository instructions, skills, quality gates, and disciplined
  development practices;
- Fitflow's deterministic driver, worktree isolation, validation, recovery,
  review, and delivery workflow; and
- Agents Army's persistent agent sessions, backend adapters, structured
  replies, teams, and lifecycle management.

These are inputs to a new product, not implementation boundaries that must be
preserved. In particular, the successor should not require one orchestration
program to shell out to a separate agent-management program while both track
overlapping agents, sessions, worktrees, retries, and state.

## Product objective

Deliver the smallest correct change, with sufficient evidence, within the
user's latency and authority budgets.

Quality gates constrain that objective. Process compliance is not itself the
product outcome, and additional ceremony must earn its cost through the risk it
reduces or evidence it produces.

## Users and primary experience

A user points the tool at a repository and can:

1. inspect and prepare the repository for agent development;
2. submit a request directly or select an existing issue;
3. review the proposed scope, acceptance criteria, budget, and execution plan;
4. observe implementation and verification as they happen;
5. pause, cancel, redirect, approve a material scope change, or take over; and
6. receive a bounded result with evidence, limitations, and follow-up
   suggestions clearly separated from completed work.

The CLI and web interface must operate on the same runs and authoritative
state. Neither is a secondary wrapper with different behavior.

## Repository preparation

Repository preparation should inspect the codebase rather than install a fixed
set of assumptions. It should identify, with evidence:

- languages, frameworks, package managers, and executable entry points;
- source, test, generated, vendored, deployment, and sensitive paths;
- focused and full verification commands;
- existing CI, review, release, and deployment conventions;
- repository instructions and supported agent-discovery mechanisms;
- boundaries suitable for worktree isolation and concurrent work; and
- gaps that prevent safe autonomous changes.

The resulting repository profile should be reviewable and editable. Generated
instructions, skills, and gates must remain concise, repository-specific, and
derived from the same policy that runtime validation enforces. Prompts and
validators must not become divergent sources of truth.

Preparation should preserve existing conventions and user work. It must not
silently replace CI, weaken gates, introduce broad permissions, or commit
credentials.

## Composable workflow model

The product's core domain model is a typed workflow graph. The web interface is
a visual authoring and observation surface for that graph, while the CLI is a
headless authoring and execution surface for the same model. Neither surface
defines a separate workflow language or runtime behavior.

Users should be able to compose workflows by connecting blocks through typed
input and output ports. The initial block families should include:

- **resources**, such as repositories, worktrees, forge connections, and
  credential references;
- **transformations**, such as prompt templates, context selection, and JSON
  Schema validation;
- **execution**, such as Codex, Claude Code, Grok, commands, and test suites;
- **control flow**, such as sequence, parallel execution, branching, bounded
  retry, timeout, approval, and budget enforcement;
- **state and artifacts**, such as checkpoints, task results, retained logs,
  and generated files;
- **delivery**, such as commits, pull requests, merges, releases, and
  deployments; and
- **observation**, such as progress events, metrics, and notifications.

Blocks should compose through capabilities instead of relying on a rigid
inheritance hierarchy. Local Git operations and remote forge operations are
distinct capabilities: a Git block can clone, fetch, branch, diff, commit,
merge, and manage worktrees, while GitHub and GitLab blocks can manage issues,
comments, pull or merge requests, checks, releases, and provider-specific
authentication. A forge-backed repository workflow composes those capabilities
instead of treating every provider operation as a subtype of Git.

Each block type and version must publish its configuration, input, and output
contracts. The engine validates connections and required capabilities before a
run begins. Unknown block types, missing inputs, incompatible ports, invalid
references, unavailable credentials, and unbounded cycles are plan-validation
failures rather than mid-run discoveries.

The visual editor should calculate and display task dependencies, concurrency,
the critical path, estimated duration, and estimated cost from the graph. It
should ship with safe templates such as a quick fix or issue-to-pull-request
workflow so users can customize proven flows without drawing every graph from
scratch.

### YAML workflows as code

YAML is the canonical portable representation of a workflow. Dragging,
connecting, or configuring a block in the UI changes an in-memory typed graph
that is serialized deterministically to YAML. Importing or editing YAML rebuilds
the same visual graph. The engine executes the parsed typed model, not ad hoc
YAML strings.

A workflow document should include:

- an explicit workflow API version and kind;
- stable node identifiers and versioned block types;
- typed inputs, outputs, and references between nodes;
- bounded time, cost, retry, and concurrency policies;
- relative artifact or schema paths resolved from the workflow file; and
- optional UI layout metadata that headless execution can ignore.

For example:

```yaml
apiVersion: megaagents.dev/v1alpha1
kind: Workflow

metadata:
  name: issue-to-pull-request

inputs:
  issue:
    type: string

nodes:
  repository:
    uses: git/checkout@v1
    with:
      remote: https://github.com/example/project.git
      credential: secret://github-bot

  issue:
    uses: github/get-issue@v1
    needs: [repository]
    with:
      number: "${{ inputs.issue }}"

  prompt:
    uses: prompt/template@v1
    needs: [issue]
    with:
      template: |
        Implement the following issue within its stated scope:
        ${{ nodes.issue.output }}

  implement:
    uses: agent/codex@v1
    needs: [repository, prompt]
    with:
      prompt: "${{ nodes.prompt.output }}"
      worktree: "${{ nodes.repository.worktree }}"
      timeout: 20m

  validate:
    uses: output/json-schema@v1
    needs: [implement]
    with:
      value: "${{ nodes.implement.output }}"
      schema: ./schemas/implementation-result.json
```

The format should support validation, planning, execution, import, and export
without starting the web interface. Workflows can therefore be committed,
reviewed, diffed, shared, tested, and run in CI. The product must define whether
visual edits preserve handwritten YAML comments and formatting or intentionally
produce normalized output; full source-preserving round trips add meaningful
editor complexity and should not be assumed accidentally.

### Workflow safety and credentials

Workflow files contain credential references, never personal access tokens,
application private keys, or other secret material. Credentials are managed by
a separate secure facility and selected by stable reference, such as
`secret://github-bot`. Exporting or committing a workflow must not export its
secrets.

Control-flow cycles must be statically bounded by attempts, elapsed time, and
cost. An agent may propose a graph change or additional work, but agent output
cannot add executable nodes to the active graph without the approval permitted
by the run's execution contract. This preserves the bounded-scope guarantee
even when workflows themselves are user-configurable.

## Bounded scope

Every run begins with an immutable root objective and a finite execution
contract. The contract records:

- the requested outcome;
- observable acceptance criteria;
- explicitly included and excluded scope;
- latency and cost budgets;
- the maximum number of execution tasks;
- the permitted decomposition depth;
- required verification and delivery actions; and
- the completion condition.

Planning may partition the root request, but it may not redefine or expand it.
The default maximum decomposition depth is one: execution tasks cannot create
grandchildren. Each task must trace to at least one root acceptance criterion,
and the complete task set must not exceed the approved objective.

Creating public tracker issues is not the default decomposition mechanism.
Internal tasks are lightweight execution records. Separate GitHub issues are
created only when the user requests independently tracked deliverables or
explicitly defers work beyond the active run.

Discoveries are classified as:

1. **Required work**: the approved outcome cannot be completed correctly
   without it. The tool explains the evidence and requests approval when it
   materially changes scope, time, cost, risk, or authority.
2. **Incidental defects**: observed problems that do not block the approved
   outcome. They are reported for later and are not added to the run.
3. **Enhancements**: refactors, cleanup, abstractions, documentation projects,
   and adjacent features. They are suggestions only.

Agents may discover unlimited possibilities, but they may execute only the
bounded contract. The run ends when the root acceptance criteria and required
verification pass, not when agents have exhausted possible improvements.

## Proportional execution

The system must choose a workflow proportional to the request and demonstrated
risk.

- Small, well-understood changes should use a fast path with one capable agent,
  focused verification, and minimal planning.
- Decomposition is justified only by scope clarity, independent parallelism,
  risk isolation, or a context boundary that improves delivery.
- Multiple agents and independent review are tools, not mandatory ceremony.
- Sequential slices should not be created when one agent can safely retain the
  necessary context.
- A stronger model may be cheaper than exhausting several long attempts with a
  weaker model. Escalation policy should optimize expected delivery time and
  cost, not per-turn price alone.
- Retries require new evidence or a changed strategy. Repeating essentially the
  same prompt and conditions is not progress.
- Focused checks should run during implementation. Full verification belongs at
  the delivery boundary unless repository policy or risk requires it sooner.

Before execution, the user should see the proposed tasks, dependencies,
critical path, expected duration, and reasons for any expensive stage. A plan
whose estimate is disproportionate to the apparent request must be surfaced for
review rather than silently started.

## Time and cost budgets

Time is a product constraint. Runs should support named modes such as quick and
standard as well as explicit deadlines. A run budget should be allocated across
planning, implementation, verification, correction, and delivery rather than
represented only by a large timeout on individual agent processes.

Each active stage must have:

- a deadline or remaining share of the run budget;
- a definition of meaningful progress;
- a maximum number of attempts;
- a no-progress threshold; and
- an explicit action on exhaustion: change strategy, escalate, request a user
  decision, preserve for resumption, or stop.

The system should estimate and continuously revise completion time. It must
distinguish model work, tool execution, queueing, blocked input, retries, and
idle time so a long run is diagnosable rather than merely slow.

## Transparency and control

The primary progress view is a live causal timeline, not a spinner and not an
unfiltered terminal transcript. Events should explain what happened, why it
happened, what evidence resulted, and what happens next.

At any point, the user should be able to see:

- the current objective and active task;
- files inspected and changed;
- commands and agent turns currently running, with elapsed time;
- task dependencies and the critical path;
- decisions made by the planner or driver and the evidence behind them;
- focused and full verification results;
- remaining time, cost, and retry budgets;
- model or provider usage when available;
- blockers, failures, and whether measurable progress is occurring; and
- pending user decisions.

Raw prompts, replies, command output, and artifacts should remain available for
diagnosis, subject to redaction and access policy, but should not be the only
way to understand a run.

Pause, cancel, redirect, approval, and takeover are workflow commands with
defined state transitions. Cancellation must terminate child process groups,
release or preserve resources deliberately, and leave a resumable or clearly
terminal record. It must not depend on killing an outer process and repairing
unknown state later.

## Reliability and recovery

The product should preserve the defensive properties that worked well in the
existing systems:

- isolated Git worktrees for mutating concurrent assignments;
- explicit authority and path boundaries;
- structured agent replies when decisions drive automation;
- independent validation of agent claims;
- bounded retries and capability escalation;
- durable session and run identity;
- refusal to continue from missing, stale, or contradictory evidence;
- resumability after interruption; and
- an auditable record of decisions and repository mutations.

Workflow state and legal transitions should be explicit and mechanically
validated. Recovery should reconstruct what happened from durable facts rather
than infer progress from a collection of procedural flags. Prompts, runtime
policy, and validation should share typed definitions wherever practical.

Gate results should be structured. At minimum they should identify the command
or policy, outcome, duration, affected paths, evidence, failure category,
retryability, and responsible task. Original command output must remain
available without forcing the workflow to scrape prose for every decision.

## Agent and backend model

The product must distinguish:

- an agent identity or reusable conversational context;
- a provider session owned by a backend;
- a workflow role such as implementer or reviewer;
- a concrete assignment within a repository and run; and
- the worktree and capabilities granted to that assignment.

Backends should advertise capabilities such as session resumption, session
forking, structured output, event streaming, reasoning controls, interactive
takeover, and usage reporting. Workflow behavior should depend on declared and
verified capabilities rather than backend-name conditionals spread through the
engine.

Every assignment receives explicit capabilities: writable paths, allowed
commands, network policy, available secrets, mutation authority, and delivery
authority. Prompt instructions reinforce these boundaries but are not the sole
enforcement mechanism.

## Repository-specific policy

The orchestration engine must not embed one repository's product layers,
directory allowlists, test commands, CI provider, issue tracker, deployment
sequence, or release process. Those belong in a repository profile or a
well-defined extension.

Fitflow's binary domain/UI slicing, Bun gates, GitHub delivery, Android release,
and deployment rules are useful source material, not universal workflow law.
The successor should be able to express those policies without requiring every
repository to adopt them.

## Success measures

Product evaluation should include outcomes rather than only workflow
conformance:

- elapsed time and time to first meaningful change;
- percentage of runs completed within their approved budget;
- acceptance criteria satisfied without user correction;
- unnecessary task and issue creation;
- retries that introduced new evidence versus repeated a failed strategy;
- time spent waiting, running tools, and running models;
- scope changes requested and approved;
- user interventions and their causes;
- escaped regressions and policy violations; and
- user ability to correctly understand current progress and blockers.

Fast delivery does not excuse incorrect work. Conversely, exhaustive ceremony
that takes a day to deliver a small change is not a high-quality result.

## Non-goals

The initial product is not intended to:

- run an endless autonomous product backlog;
- invent and implement adjacent features without approval;
- require multiple agents for every request;
- mirror every internal task into an external issue tracker;
- replace repository policy with one universal development workflow;
- conceal long-running activity behind a simplified status indicator;
- automatically merge or deploy unless the user and repository policy grant
  that authority; or
- preserve existing Python boundaries solely for compatibility with the
  prototypes.

## Open product questions

The following should be resolved before or alongside architectural design:

- What are the default latency and cost budgets for quick and standard modes?
- Which scope changes may proceed under preapproved policy, if any?
- When should the tool require plan approval before implementation?
- Which controls remain available during an agent turn versus between turns?
- What information may be retained from prompts, model replies, and command
  output, and for how long?
- How should repository profiles be reviewed, versioned, and upgraded?
- Which delivery actions are opt-in globally, per repository, and per run?
- What minimum backend capabilities are required for the first release?

## Relationship to future architecture documents

This document states what the successor must accomplish and why. Technical
choices such as Go package boundaries, event storage, database selection,
process supervision, extension interfaces, and API protocols belong in focused
architecture documents or decision records. Those choices should demonstrate
how they serve the requirements here and identify which alternatives were
rejected.
