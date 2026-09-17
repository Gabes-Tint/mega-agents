<script lang="ts">
  import { Dialog } from "bits-ui";
  import {
    GraphStore,
    workflowSlug,
    type Problem,
    type WorkflowGraph,
  } from "./graph.svelte.js";
  import Palette from "./Palette.svelte";
  import Canvas from "./Canvas.svelte";
  import PropertiesPanel from "./PropertiesPanel.svelte";
  import ThemeToggle from "./ThemeToggle.svelte";

  // What the backend said about itself, shown in the status bar.
  let { status = "" }: { status?: string } = $props();

  interface RunStep {
    nodeId: string;
    name: string;
    action: string;
    status: string;
    error?: string;
    details?: Record<string, unknown>;
  }

  interface RunResult {
    id?: string;
    retryOf?: string;
    status: string;
    steps: RunStep[];
  }

  // How often a started run is polled for progress.
  const RUN_POLL_MS = 250;

  // How long the graph must stay unchanged before its problems are checked.
  const PROBLEMS_DELAY_MS = 300;

  // Step details shown as labelled lines, in this order.
  const DETAIL_LABELS: [string, string][] = [
    ["repository", "Repository"],
    ["remote", "Remote"],
    ["path", "Path"],
    ["branch", "Branch"],
    ["base", "Base"],
    ["backend", "Backend"],
    ["model", "Model"],
    ["sessionId", "Session"],
    ["continuedFrom", "Continued from"],
    ["attempts", "Attempts"],
    ["case", "Route"],
    ["exitCode", "Exit code"],
    ["commit", "Commit"],
    ["reusedFrom", "Reused from"],
  ];

  interface Usage {
    inputTokens: number;
    outputTokens: number;
    costUsd: number;
  }

  function usageOf(step: RunStep): Usage | undefined {
    const usage = step.details?.usage;
    return usage && typeof usage === "object" ? (usage as Usage) : undefined;
  }

  // What the run's agents reported they used, added up.
  function runUsage(result: RunResult): string {
    const usages = result.steps
      .map(usageOf)
      .filter((usage): usage is Usage => usage !== undefined);
    if (usages.length === 0) return "";
    const total = usages.reduce(
      (sum, usage) => ({
        inputTokens: sum.inputTokens + usage.inputTokens,
        outputTokens: sum.outputTokens + usage.outputTokens,
        costUsd: sum.costUsd + usage.costUsd,
      }),
      { inputTokens: 0, outputTokens: 0, costUsd: 0 },
    );
    return `Agents cost $${total.costUsd.toFixed(4)} · ${total.inputTokens} input tokens · ${total.outputTokens} output tokens`;
  }

  function fieldErrors(details: Record<string, unknown> | undefined): string[] {
    const errors = details?.errors;
    if (!Array.isArray(errors)) return [];
    return errors.map(
      (entry: { path?: string; message?: string }) =>
        `${entry.path ?? "$"}: ${entry.message ?? ""}`,
    );
  }

  // The graph in progress survives reloads of the page in this browser.
  const DRAFT_KEY = "mega-agents:draft";

  const graph = new GraphStore();
  try {
    const draft = localStorage.getItem(DRAFT_KEY);
    if (draft) graph.load(JSON.parse(draft) as WorkflowGraph);
  } catch {
    // A missing or unreadable draft starts an empty graph.
  }
  $effect(() => {
    const draft = JSON.stringify(graph.toRequest());
    try {
      localStorage.setItem(DRAFT_KEY, draft);
    } catch {
      // Storage may be unavailable; the graph still works for this page.
    }
  });

  // The bottom panel shows what to fix before running, or the run.
  let panelTab = $state<"problems" | "run">("problems");
  let problemsChecked = $state(false);
  let problemsRequest = 0;

  $effect(() => {
    const body = JSON.stringify(graph.toRequest());
    const request = ++problemsRequest;
    const timer = setTimeout(
      () => void checkProblems(body, request),
      PROBLEMS_DELAY_MS,
    );
    return () => globalThis.clearTimeout(timer);
  });

  async function checkProblems(body: string, request: number): Promise<void> {
    try {
      const response = await fetch("/api/workflows/problems", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body,
      });
      if (!response.ok) return;
      const problems: unknown = await response.json();
      if (request !== problemsRequest || !Array.isArray(problems)) return;
      graph.problems = problems as Problem[];
      problemsChecked = true;
    } catch {
      // The list keeps its last answer until the backend answers again.
    }
  }

  function nameOf(nodeId: string | undefined): string {
    return graph.nodes.find((node) => node.id === nodeId)?.name ?? "";
  }

  function plural(count: number, word: string): string {
    return `${count} ${word}${count === 1 ? "" : "s"}`;
  }

  interface WorkflowSummary {
    name: string;
    updatedAt: string;
  }

  interface RunSummary {
    id: string;
    workflow: string;
    status: string;
    startedAt: string;
  }

  let historyDialog = $state(false);
  let pastRuns = $state<RunSummary[] | null>(null);

  async function listRuns(): Promise<void> {
    pastRuns = null;
    try {
      const response = await fetch("/api/runs");
      pastRuns = response.ok ? ((await response.json()) as RunSummary[]) : [];
    } catch {
      pastRuns = [];
    }
  }

  async function openRun(id: string): Promise<void> {
    historyDialog = false;
    panelTab = "run";
    runError = "";
    try {
      const response = await fetch(`/api/runs/${encodeURIComponent(id)}`);
      if (!response.ok) {
        runError = (await response.text()).trim();
        return;
      }
      const opened = (await response.json()) as RunResult;
      showResult(opened);
      if (opened.status === "running" && !running) {
        running = true;
        await follow(opened);
        running = false;
      }
    } catch {
      runError = "Cannot reach the backend";
    }
  }

  async function retryRun(): Promise<void> {
    const id = runResult?.id;
    if (!id) return;
    running = true;
    panelTab = "run";
    runError = "";
    try {
      const response = await fetch(
        `/api/runs/${encodeURIComponent(id)}/retry`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: "{}",
        },
      );
      if (!response.ok) {
        runError = (await response.text()).trim();
        return;
      }
      const retried = (await response.json()) as RunResult;
      showResult(retried);
      await follow(retried);
    } catch {
      runError = "Cannot reach the backend";
    } finally {
      running = false;
    }
  }

  async function cancelRun(): Promise<void> {
    const id = runResult?.id;
    if (!id) return;
    try {
      const response = await fetch(
        `/api/runs/${encodeURIComponent(id)}/cancel`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: "{}",
        },
      );
      if (!response.ok) runError = (await response.text()).trim();
    } catch {
      runError = "Cannot reach the backend";
    }
  }

  interface TemplateSummary {
    name: string;
    title: string;
    description: string;
  }

  let templateDialog = $state(false);
  let templates = $state<TemplateSummary[] | null>(null);

  async function listTemplates(): Promise<void> {
    templates = null;
    try {
      const response = await fetch("/api/templates");
      templates = response.ok
        ? ((await response.json()) as TemplateSummary[])
        : [];
    } catch {
      templates = [];
    }
  }

  async function startFromTemplate(template: TemplateSummary): Promise<void> {
    templateDialog = false;
    try {
      const response = await fetch(`/api/templates/${template.name}`);
      if (!response.ok) {
        fileStatus = (await response.text()).trim();
        return;
      }
      graph.load((await response.json()) as WorkflowGraph);
      runResult = null;
      fileStatus = `Started from template ${template.title}`;
    } catch {
      fileStatus = "Cannot reach the backend";
    }
  }

  let fileStatus = $state("");
  let openDialog = $state(false);
  let savedWorkflows = $state<WorkflowSummary[] | null>(null);

  async function saveWorkflow(): Promise<void> {
    const name = workflowSlug(graph.workflowName);
    fileStatus = "Saving…";
    try {
      const response = await fetch(`/api/workflows/${name}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(graph.toRequest()),
      });
      fileStatus = response.ok
        ? `Saved as ${name}`
        : (await response.text()).trim();
    } catch {
      fileStatus = "Cannot reach the backend";
    }
  }

  async function listWorkflows(): Promise<void> {
    savedWorkflows = null;
    try {
      const response = await fetch("/api/workflows");
      savedWorkflows = response.ok
        ? ((await response.json()) as WorkflowSummary[])
        : [];
    } catch {
      savedWorkflows = [];
      fileStatus = "Cannot reach the backend";
    }
  }

  async function openWorkflow(name: string): Promise<void> {
    openDialog = false;
    try {
      const response = await fetch(`/api/workflows/${name}`);
      if (!response.ok) {
        fileStatus = (await response.text()).trim();
        return;
      }
      graph.load((await response.json()) as WorkflowGraph);
      runResult = null;
      fileStatus = `Opened ${name}`;
    } catch {
      fileStatus = "Cannot reach the backend";
    }
  }

  async function importWorkflow(file: File | undefined): Promise<void> {
    if (!file) return;
    try {
      const response = await fetch("/api/workflows/import", {
        method: "POST",
        headers: { "Content-Type": "application/yaml" },
        body: await file.text(),
      });
      if (!response.ok) {
        fileStatus = (await response.text()).trim();
        return;
      }
      graph.load((await response.json()) as WorkflowGraph);
      runResult = null;
      fileStatus = `Imported ${file.name}`;
    } catch {
      fileStatus = "Cannot reach the backend";
    }
  }
  let yamlError = $state("");
  let running = $state(false);
  let runResult = $state<RunResult | null>(null);
  let runError = $state("");
  let logText = $state("");
  let logRequest = 0;

  const loggedStep = $derived(
    runResult?.steps.find((step) => step.nodeId === graph.logNodeId),
  );

  function wait(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  function showResult(result: RunResult): void {
    runResult = result;
    graph.showRun(result.steps, result.id ?? null);
    if (graph.logNodeId) void loadLog(graph.logNodeId);
  }

  async function loadLog(nodeId: string): Promise<void> {
    const request = ++logRequest;
    try {
      const response = await fetch(
        `/api/runs/${encodeURIComponent(graph.runId ?? "")}/logs/${encodeURIComponent(nodeId)}`,
      );
      const text = response.ok
        ? await response.text()
        : "No log recorded for this step";
      if (request === logRequest) logText = text;
    } catch {
      if (request === logRequest) logText = "Cannot reach the backend";
    }
  }

  // Opening a block's log, from here or from its properties, loads it.
  $effect(() => {
    const nodeId = graph.logNodeId;
    logText = "";
    if (nodeId) void loadLog(nodeId);
  });

  // A run the backend started keeps executing after the request returns;
  // poll it so the canvas and the result follow each step.
  async function follow(result: RunResult): Promise<void> {
    let current = result;
    while (current.id && current.status === "running") {
      await wait(RUN_POLL_MS);
      try {
        const response = await fetch(
          `/api/runs/${encodeURIComponent(current.id)}`,
        );
        if (!response.ok) throw new Error(await response.text());
        current = (await response.json()) as RunResult;
      } catch {
        runError = `Lost track of run ${current.id}; see it with: mega-agents runs show ${current.id}`;
        return;
      }
      showResult(current);
    }
  }

  function attachmentFilename(header: string | null): string {
    const match = /filename="([^"]+)"/.exec(header ?? "");
    return match?.[1] ?? "workflow.yaml";
  }

  async function downloadYaml(): Promise<void> {
    yamlError = "";
    try {
      const response = await fetch("/api/workflows/yaml", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(graph.toRequest()),
      });
      if (!response.ok) {
        yamlError = await response.text();
        panelTab = "run";
        return;
      }
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = attachmentFilename(
        response.headers.get("Content-Disposition"),
      );
      anchor.click();
      URL.revokeObjectURL(url);
    } catch {
      yamlError = "Cannot reach the backend";
      panelTab = "run";
    }
  }

  async function runFlow(): Promise<void> {
    running = true;
    panelTab = "run";
    runResult = null;
    runError = "";
    graph.showRun([]);
    graph.openLog(null);
    try {
      const response = await fetch("/api/runs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(graph.toRequest()),
      });
      if (!response.ok) {
        runError = (await response.text()).trim();
        return;
      }
      const started = (await response.json()) as RunResult;
      showResult(started);
      await follow(started);
    } catch {
      runError = "Cannot reach the backend";
    } finally {
      running = false;
    }
  }
</script>

<div class="workspace">
  <header class="topbar">
    <div class="brand">
      <svg class="logo" viewBox="0 0 20 20" aria-hidden="true">
        <rect x="1.5" y="1.5" width="7" height="7" rx="2" />
        <rect x="11.5" y="11.5" width="7" height="7" rx="2" />
        <path d="M8.5 5h3a2 2 0 0 1 2 2v4.5" />
      </svg>
      <h1>Mega Agents</h1>
    </div>
    <input
      type="text"
      class="workflow-name"
      aria-label="Workflow name"
      bind:value={graph.workflowName}
    />
    <div class="group" role="group" aria-label="Workflow file actions">
      <button
        type="button"
        class="ghost"
        onclick={() => {
          templateDialog = true;
          void listTemplates();
        }}
      >
        Templates…
      </button>
      <button
        type="button"
        class="ghost"
        onclick={() => {
          openDialog = true;
          void listWorkflows();
        }}
      >
        Open…
      </button>
      <button type="button" class="ghost" onclick={() => void saveWorkflow()}
        >Save</button
      >
      <label class="import ghost-like">
        Import YAML
        <input
          type="file"
          accept=".yaml,.yml"
          onchange={(event) => {
            void importWorkflow(event.currentTarget.files?.[0]);
            event.currentTarget.value = "";
          }}
        />
      </label>
      <button type="button" class="ghost" onclick={() => void downloadYaml()}>
        Download YAML
      </button>
    </div>
    <div class="spacer"></div>
    <div class="group" role="group" aria-label="Run actions">
      <button
        type="button"
        class="ghost"
        onclick={() => {
          historyDialog = true;
          void listRuns();
        }}
      >
        Runs…
      </button>
      {#if running && runResult?.id && runResult.status === "running"}
        <button type="button" class="danger" onclick={() => void cancelRun()}>
          Cancel run
        </button>
      {/if}
      <button
        type="button"
        class="run"
        disabled={running}
        onclick={() => void runFlow()}
      >
        <svg viewBox="0 0 12 12" aria-hidden="true"
          ><path d="M3 1.8v8.4L10 6Z" /></svg
        >
        {running ? "Running…" : "Run flow"}
      </button>
      <ThemeToggle />
    </div>
  </header>
  <Dialog.Root
    open={graph.logNodeId !== null && !!runResult}
    onOpenChange={(open) => {
      if (!open) graph.openLog(null);
    }}
  >
    <Dialog.Portal>
      <Dialog.Overlay class="backdrop" />
      <Dialog.Content class="log-viewer">
        <Dialog.Title class="dialog-title">
          Logs: {loggedStep?.name ?? ""}
        </Dialog.Title>
        <pre class="log-text">{logText || "Loading…"}</pre>
        <Dialog.Close class="close-button">Close</Dialog.Close>
      </Dialog.Content>
    </Dialog.Portal>
  </Dialog.Root>
  <Dialog.Root bind:open={templateDialog}>
    <Dialog.Portal>
      <Dialog.Overlay class="backdrop" />
      <Dialog.Content class="log-viewer">
        <Dialog.Title class="dialog-title">Start from a template</Dialog.Title>
        {#if templates === null}
          <p class="muted">Loading…</p>
        {:else}
          <ul class="picker">
            {#each templates as template (template.name)}
              <li>
                <button
                  type="button"
                  onclick={() => void startFromTemplate(template)}
                  >{template.title}</button
                >
                <span>{template.description}</span>
              </li>
            {/each}
          </ul>
        {/if}
        <Dialog.Close class="close-button">Close</Dialog.Close>
      </Dialog.Content>
    </Dialog.Portal>
  </Dialog.Root>
  <Dialog.Root bind:open={historyDialog}>
    <Dialog.Portal>
      <Dialog.Overlay class="backdrop" />
      <Dialog.Content class="log-viewer">
        <Dialog.Title class="dialog-title">Run history</Dialog.Title>
        {#if pastRuns === null}
          <p class="muted">Loading…</p>
        {:else if pastRuns.length === 0}
          <p class="muted">No runs recorded yet.</p>
        {:else}
          <ul class="picker">
            {#each pastRuns as pastRun (pastRun.id)}
              <li>
                <button type="button" onclick={() => void openRun(pastRun.id)}
                  >{pastRun.workflow} · {pastRun.status} · {pastRun.startedAt}</button
                >
              </li>
            {/each}
          </ul>
        {/if}
        <Dialog.Close class="close-button">Close</Dialog.Close>
      </Dialog.Content>
    </Dialog.Portal>
  </Dialog.Root>
  <Dialog.Root bind:open={openDialog}>
    <Dialog.Portal>
      <Dialog.Overlay class="backdrop" />
      <Dialog.Content class="log-viewer">
        <Dialog.Title class="dialog-title">Open workflow</Dialog.Title>
        {#if savedWorkflows === null}
          <p class="muted">Loading…</p>
        {:else if savedWorkflows.length === 0}
          <p class="muted">No saved workflows yet.</p>
        {:else}
          <ul class="picker">
            {#each savedWorkflows as workflow (workflow.name)}
              <li>
                <button
                  type="button"
                  onclick={() => void openWorkflow(workflow.name)}
                  >{workflow.name}</button
                >
                <span>{workflow.updatedAt}</span>
              </li>
            {/each}
          </ul>
        {/if}
        <Dialog.Close class="close-button">Close</Dialog.Close>
      </Dialog.Content>
    </Dialog.Portal>
  </Dialog.Root>
  <Palette {graph} />
  <Canvas {graph} />
  <PropertiesPanel {graph} />
  <section class="panel" aria-label="Output">
    <div class="panel-header" role="tablist" aria-label="Output">
      <button
        type="button"
        role="tab"
        id="tab-problems"
        class="tab"
        aria-selected={panelTab === "problems"}
        aria-controls="panel-problems"
        onclick={() => (panelTab = "problems")}
      >
        Problems
        {#if graph.problems.length > 0}
          <span class="count" class:has-errors={graph.problemCounts.errors > 0}
            >{graph.problems.length}</span
          >
        {/if}
      </button>
      <button
        type="button"
        role="tab"
        id="tab-run"
        class="tab"
        aria-selected={panelTab === "run"}
        aria-controls="panel-run"
        onclick={() => (panelTab = "run")}
      >
        Run
      </button>
    </div>
    <div
      class="panel-body"
      role="tabpanel"
      id="panel-problems"
      aria-labelledby="tab-problems"
      hidden={panelTab !== "problems"}
    >
      {#if graph.problems.length > 0}
        <ul class="problems">
          {#each graph.problems as problem, index (index)}
            <li class="problem {problem.severity}">
              <span class="severity" aria-hidden="true"></span>
              <span class="message" id="problem-{index}">{problem.message}</span
              >
              {#if problem.nodeId}
                {@const nodeId = problem.nodeId}
                <button
                  type="button"
                  class="small ghost"
                  aria-label="Select the block with problem {index + 1}"
                  aria-describedby="problem-{index}"
                  onclick={() => graph.select(nodeId)}
                >
                  {nameOf(nodeId)}
                </button>
              {/if}
            </li>
          {/each}
        </ul>
      {:else if graph.nodes.length === 0}
        <p class="muted">
          Nothing to check yet. Drag components onto the canvas.
        </p>
      {:else if problemsChecked}
        <p class="muted">No problems found. The workflow is ready to run.</p>
      {:else}
        <p class="muted">Checking…</p>
      {/if}
    </div>
    <div
      class="panel-body"
      role="tabpanel"
      id="panel-run"
      aria-labelledby="tab-run"
      hidden={panelTab !== "run"}
    >
      <section aria-label="Run result" aria-live="polite">
        <div class="run-summary">
          {#if runResult && !runError}
            <span class="run-status {runResult.status}">
              <span class="dot" aria-hidden="true"></span>
              Run {runResult.status}
            </span>
            {#if runResult.retryOf}
              <span class="meta">Retry of {runResult.retryOf}</span>
            {/if}
            {#if runUsage(runResult)}
              <span class="meta">{runUsage(runResult)}</span>
            {/if}
            {#if runResult.id && !running && ["failed", "cancelled", "interrupted"].includes(runResult.status)}
              <button
                type="button"
                class="small"
                onclick={() => void retryRun()}
              >
                Retry from failure
              </button>
            {/if}
          {/if}
        </div>
        {#if yamlError}
          <p class="error">{yamlError}</p>
        {/if}
        {#if runError}
          <p class="error">{runError}</p>
        {:else if runResult}
          <ul class="steps">
            {#each runResult.steps as step (step.nodeId)}
              <li class="step {step.status}">
                <div class="step-line">
                  <span class="dot" aria-hidden="true"></span>
                  <strong>
                    {step.name}: {step.action}
                    {step.status}
                  </strong>
                  {#if runResult.id}
                    <button
                      type="button"
                      class="small ghost"
                      aria-label="Logs of {step.name}"
                      onclick={() => graph.openLog(step.nodeId)}
                    >
                      Logs
                    </button>
                  {/if}
                </div>
                <div class="details">
                  {#if usageOf(step)}
                    {@const usage = usageOf(step)!}
                    <span
                      >Cost: ${usage.costUsd.toFixed(4)} · {usage.inputTokens} in
                      /
                      {usage.outputTokens} out</span
                    >
                  {/if}
                  {#if step.details && "valid" in step.details}
                    <span>Valid: {String(step.details.valid)}</span>
                  {/if}
                  {#each DETAIL_LABELS as [key, label] (key)}
                    {#if step.details?.[key] !== undefined && step.details?.[key] !== ""}
                      <span>{label}: {step.details[key]}</span>
                    {/if}
                  {/each}
                  {#if typeof step.details?.url === "string"}
                    <a href={step.details.url} target="_blank" rel="noreferrer"
                      >{step.details.url}</a
                    >
                  {/if}
                </div>
                {#if step.error}
                  <pre class="failed">{step.error}</pre>
                {:else if step.details?.output}
                  <pre>{step.details.output}</pre>
                {/if}
                {#each fieldErrors(step.details) as fieldError (fieldError)}
                  <pre class="failed">{fieldError}</pre>
                {/each}
                {#if step.details?.output && step.action === "command"}
                  <pre>{step.details.output}</pre>
                {/if}
                {#if step.details?.reply}
                  <pre>{step.details.reply}</pre>
                {/if}
              </li>
            {/each}
          </ul>
        {:else if !yamlError}
          <p class="muted">
            No run yet. Run flow executes the graph from its starting points.
          </p>
        {/if}
      </section>
    </div>
  </section>
  <footer class="statusbar">
    <button
      type="button"
      class="problem-counts"
      aria-label="{plural(graph.problemCounts.errors, 'error')}, {plural(
        graph.problemCounts.warnings,
        'warning',
      )}"
      onclick={() => (panelTab = "problems")}
    >
      <span class="severity error" aria-hidden="true"></span>
      {graph.problemCounts.errors}
      <span class="severity warning" aria-hidden="true"></span>
      {graph.problemCounts.warnings}
    </button>
    <span class="backend">{status}</span>
    <span class="file-status" role="status" aria-label="Workflow file"
      >{fileStatus}</span
    >
  </footer>
</div>

<style>
  .workspace {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: 220px minmax(0, 1fr) 300px;
    grid-template-rows: auto minmax(0, 1fr) clamp(7rem, 28vh, 18rem) auto;
    grid-template-areas:
      "top top top"
      "palette canvas properties"
      "palette panel properties"
      "status status status";
  }

  .workspace > :global(aside[aria-label="Component palette"]) {
    grid-area: palette;
  }

  .workspace > :global(.canvas) {
    grid-area: canvas;
  }

  .workspace > :global(aside[aria-label="Node properties"]) {
    grid-area: properties;
  }

  .topbar {
    grid-area: top;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem 0.75rem;
    padding: 0.45rem 0.75rem;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    padding-right: 0.25rem;
  }

  .logo {
    width: 1.15rem;
    height: 1.15rem;
    fill: none;
    stroke: var(--accent);
    stroke-width: 1.6;
  }

  h1 {
    margin: 0;
    font-size: 14px;
    font-weight: 650;
    letter-spacing: -0.01em;
  }

  .workflow-name {
    width: 12rem;
    border-color: transparent;
    background: var(--surface-sunken);
    font-weight: 500;
  }

  .workflow-name:hover {
    border-color: var(--border-strong);
  }

  .group {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.25rem;
  }

  .spacer {
    flex: 1;
  }

  .ghost,
  .ghost-like {
    border-color: transparent;
    background: transparent;
    color: var(--text-muted);
  }

  .ghost:hover,
  .ghost-like:hover {
    color: var(--text);
  }

  .import {
    position: relative;
    display: inline-flex;
    align-items: center;
    min-height: 1.9rem;
    padding: 0.3rem 0.7rem;
    border: 1px solid transparent;
    border-radius: var(--radius);
    font-weight: 500;
    cursor: pointer;
  }

  .import:hover {
    background: var(--surface-hover);
  }

  .import:focus-within {
    outline: 2px solid var(--accent);
  }

  /* The native file input stays in place for the label and tests but is
     visually replaced by the label text. */
  .import input {
    position: absolute;
    inset: 0;
    opacity: 0;
    cursor: pointer;
  }

  .run {
    border-color: var(--accent);
    background: var(--accent);
    color: var(--accent-text);
    padding-inline: 0.9rem;
  }

  .run:hover:not(:disabled) {
    border-color: var(--accent-hover);
    background: var(--accent-hover);
  }

  .run svg {
    width: 0.7rem;
    height: 0.7rem;
    fill: currentColor;
  }

  .danger {
    border-color: var(--fail);
    color: var(--fail);
    background: transparent;
  }

  .small {
    min-height: 1.5rem;
    padding: 0.1rem 0.5rem;
    font-size: 12px;
  }

  .panel {
    grid-area: panel;
    min-height: 0;
    display: flex;
    flex-direction: column;
    border-top: 1px solid var(--border);
    background: var(--surface);
  }

  .panel-header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.4rem 0.75rem;
    padding: 0.3rem 0.75rem;
    border-bottom: 1px solid var(--border);
  }

  .panel-header {
    gap: 0.25rem;
    padding-block: 0;
  }

  .tab {
    min-height: 2rem;
    padding: 0.25rem 0.6rem;
    border: 0;
    border-bottom: 2px solid transparent;
    border-radius: 0;
    background: transparent;
    color: var(--text-muted);
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .tab:hover:not(:disabled) {
    background: transparent;
    color: var(--text);
  }

  .tab[aria-selected="true"] {
    border-bottom-color: var(--accent);
    color: var(--text);
  }

  .count {
    min-width: 1.1rem;
    padding: 0 0.3rem;
    border-radius: 999px;
    background: var(--warn-soft);
    color: var(--warn);
    font-size: 11px;
    letter-spacing: 0;
    font-variant-numeric: tabular-nums;
  }

  .count.has-errors {
    background: var(--fail-soft);
    color: var(--fail);
  }

  .run-summary {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.4rem 0.75rem;
    margin-bottom: 0.4rem;
  }

  .run-summary:empty {
    display: none;
  }

  .problems {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
  }

  .problem {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    min-height: 1.75rem;
    padding: 0 0.4rem;
    border-radius: var(--radius);
  }

  .problem:hover {
    background: var(--surface-sunken);
  }

  .problem .message {
    color: var(--text);
  }

  .problem button {
    color: var(--text-faint);
    font-weight: 400;
  }

  .severity {
    flex: none;
    width: 0.6rem;
    height: 0.6rem;
    border-radius: 50%;
    background: var(--text-faint);
  }

  .error > .severity,
  .severity.error {
    background: var(--fail);
  }

  .warning > .severity,
  .severity.warning {
    border-radius: 1px;
    background: var(--warn);
    clip-path: polygon(50% 0, 100% 100%, 0 100%);
  }

  .problem-counts {
    min-height: 1.4rem;
    gap: 0.3rem;
    padding: 0 0.4rem;
    border: 0;
    border-radius: 0;
    background: transparent;
    color: var(--text-muted);
    font-size: 12px;
    font-variant-numeric: tabular-nums;
  }

  .problem-counts .severity.warning {
    margin-left: 0.35rem;
  }

  .panel-body {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: 0.5rem 0.75rem;
  }

  .meta {
    color: var(--text-muted);
    font-size: 12px;
    font-variant-numeric: tabular-nums;
  }

  .run-status {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    font-weight: 600;
  }

  .dot {
    flex: none;
    width: 0.5rem;
    height: 0.5rem;
    border-radius: 50%;
    background: var(--text-faint);
  }

  .succeeded > .dot,
  .succeeded > .step-line > .dot {
    background: var(--ok);
  }

  .failed > .dot,
  .failed > .step-line > .dot {
    background: var(--fail);
  }

  .running > .dot,
  .running > .step-line > .dot {
    background: var(--info);
  }

  .cancelled > .dot,
  .interrupted > .dot,
  .cancelled > .step-line > .dot,
  .interrupted > .step-line > .dot {
    background: var(--warn);
  }

  .run-status.failed {
    color: var(--fail);
  }

  .run-status.succeeded {
    color: var(--ok);
  }

  .steps {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 0.35rem;
  }

  .step {
    padding: 0.35rem 0.5rem;
    border-radius: var(--radius);
  }

  .step:hover {
    background: var(--surface-sunken);
  }

  .step-line {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .step-line strong {
    font-weight: 550;
  }

  .step.failed .step-line strong {
    color: var(--fail);
  }

  .details {
    display: flex;
    flex-wrap: wrap;
    gap: 0.1rem 0.9rem;
    padding-left: 1rem;
    color: var(--text-muted);
    font-size: 12px;
  }

  .details a {
    color: var(--accent);
  }

  .panel pre {
    margin: 0.3rem 0 0 1rem;
    max-height: 8rem;
    overflow: auto;
    white-space: pre-wrap;
    padding: 0.4rem 0.5rem;
    border-radius: var(--radius);
    background: var(--surface-sunken);
    font-size: 12px;
    color: var(--text-muted);
  }

  .panel pre.failed {
    background: var(--fail-soft);
    color: var(--fail);
  }

  .muted {
    margin: 0;
    color: var(--text-muted);
  }

  .error {
    margin: 0 0 0.4rem;
    color: var(--fail);
  }

  .statusbar {
    grid-area: status;
    display: flex;
    align-items: center;
    gap: 1.25rem;
    min-height: 1.5rem;
    padding: 0 0.75rem;
    border-top: 1px solid var(--border);
    background: var(--surface-sunken);
    color: var(--text-muted);
    font-size: 12px;
  }

  .picker {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 0.25rem;
    overflow: auto;
  }

  .picker li {
    display: grid;
    gap: 0.15rem;
    padding: 0.35rem;
    border-radius: var(--radius);
  }

  .picker li:hover {
    background: var(--surface-sunken);
  }

  .picker button {
    justify-content: flex-start;
    width: fit-content;
  }

  .picker span {
    padding-left: 0.2rem;
    font-size: 12px;
    color: var(--text-muted);
  }

  .log-text {
    margin: 0;
    min-height: 0;
    overflow: auto;
    white-space: pre-wrap;
    font-size: 12px;
    line-height: 1.55;
    color: var(--text);
    background: var(--surface-sunken);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 0.6rem 0.75rem;
  }
</style>
