<script lang="ts">
  import { Dialog } from "bits-ui";
  import {
    GraphStore,
    workflowSlug,
    type WorkflowGraph,
  } from "./graph.svelte.js";
  import Palette from "./Palette.svelte";
  import Canvas from "./Canvas.svelte";
  import PropertiesPanel from "./PropertiesPanel.svelte";

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
    status: string;
    steps: RunStep[];
  }

  // How often a started run is polled for progress.
  const RUN_POLL_MS = 250;

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
    ["attempts", "Attempts"],
    ["case", "Route"],
    ["exitCode", "Exit code"],
  ];

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
    }
  }

  async function runFlow(): Promise<void> {
    running = true;
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
  <div class="toolbar">
    <div class="actions">
      <button
        type="button"
        class="run"
        disabled={running}
        onclick={() => void runFlow()}
      >
        {running ? "Running…" : "Run flow"}
      </button>
      {#if running && runResult?.id && runResult.status === "running"}
        <button type="button" onclick={() => void cancelRun()}>
          Cancel run
        </button>
      {/if}
      <button
        type="button"
        onclick={() => {
          historyDialog = true;
          void listRuns();
        }}
      >
        Runs…
      </button>
      <button type="button" onclick={() => void downloadYaml()}>
        Download YAML
      </button>
      <label class="workflow-name">
        Workflow name
        <input type="text" bind:value={graph.workflowName} />
      </label>
      <button type="button" onclick={() => void saveWorkflow()}>Save</button>
      <button
        type="button"
        onclick={() => {
          openDialog = true;
          void listWorkflows();
        }}
      >
        Open…
      </button>
      <label class="import">
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
      <span class="file-status" role="status" aria-label="Workflow file"
        >{fileStatus}</span
      >
    </div>
    {#if yamlError}
      <p class="yaml-error">{yamlError}</p>
    {/if}
    <section class="run-result" aria-label="Run result" aria-live="polite">
      {#if runError}
        <p class="yaml-error">{runError}</p>
      {:else if runResult}
        <p class="run-status {runResult.status}">
          Run {runResult.status}
        </p>
        <ul>
          {#each runResult.steps as step (step.nodeId)}
            <li>
              <strong class={step.status}>
                {step.name}: {step.action}
                {step.status}
              </strong>
              {#if runResult.id}
                <button
                  type="button"
                  class="log-link"
                  aria-label="Logs of {step.name}"
                  onclick={() => graph.openLog(step.nodeId)}
                >
                  Logs
                </button>
              {/if}
              {#if step.details && "valid" in step.details}
                <span>Valid: {String(step.details.valid)}</span>
              {/if}
              {#each DETAIL_LABELS as [key, label] (key)}
                {#if step.details?.[key] !== undefined && step.details?.[key] !== ""}
                  <span>{label}: {step.details[key]}</span>
                {/if}
              {/each}
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
      {/if}
    </section>
  </div>
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
  <Dialog.Root bind:open={historyDialog}>
    <Dialog.Portal>
      <Dialog.Overlay class="backdrop" />
      <Dialog.Content class="log-viewer">
        <Dialog.Title class="dialog-title">Run history</Dialog.Title>
        {#if pastRuns === null}
          <p>Loading…</p>
        {:else if pastRuns.length === 0}
          <p>No runs recorded yet.</p>
        {:else}
          <ul class="saved-workflows">
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
          <p>Loading…</p>
        {:else if savedWorkflows.length === 0}
          <p>No saved workflows yet.</p>
        {:else}
          <ul class="saved-workflows">
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
</div>

<style>
  .workspace {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: 200px 1fr 260px;
    grid-template-rows: auto 1fr;
  }

  .toolbar {
    grid-column: 1 / -1;
    padding: 0.5rem 0.75rem;
    border-bottom: 1px solid #d5e0db;
  }

  .toolbar button {
    padding: 0.4rem 0.75rem;
    border: 1px solid #b8ccc4;
    border-radius: 0.375rem;
    background: #ffffff;
    color: #17342c;
    font: inherit;
    cursor: pointer;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
  }

  .workflow-name,
  .import {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.8rem;
    color: #5b7a71;
  }

  .workflow-name input {
    padding: 0.3rem 0.4rem;
    border: 1px solid #b8ccc4;
    border-radius: 0.375rem;
    font: inherit;
  }

  .import input {
    max-width: 12rem;
    font-size: 0.75rem;
  }

  .file-status {
    font-size: 0.8rem;
    color: #5b7a71;
  }

  .saved-workflows {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 0.25rem;
  }

  .saved-workflows span {
    margin-left: 0.5rem;
    font-size: 0.75rem;
    color: #5b7a71;
  }

  .toolbar button:disabled {
    cursor: progress;
    opacity: 0.6;
  }

  .toolbar .run {
    border-color: #2f7a5f;
    background: #2f7a5f;
    color: #ffffff;
  }

  .run-status {
    margin: 0.5rem 0 0.25rem;
    font-size: 0.85rem;
    font-weight: 600;
  }

  .run-result ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 0.25rem;
    font-size: 0.8rem;
  }

  .run-result span {
    margin-left: 0.5rem;
    color: #5b7a71;
  }

  .run-result pre {
    margin: 0.25rem 0 0;
    max-height: 8rem;
    overflow: auto;
    white-space: pre-wrap;
    font-size: 0.75rem;
    color: #5b7a71;
  }

  .succeeded {
    color: #2f7a5f;
  }

  .run-result .failed,
  .run-status.failed {
    color: #a03030;
  }

  .log-link {
    margin-left: 0.5rem;
    padding: 0 0.4rem;
    font-size: 0.75rem;
  }

  :global(.log-viewer) {
    position: fixed;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    width: min(48rem, 92vw);
    max-height: 80vh;
    display: grid;
    gap: 0.5rem;
    border: 1px solid #b8ccc4;
    border-radius: 0.375rem;
    background: #ffffff;
    padding: 0.75rem;
  }

  .log-text {
    margin: 0;
    max-height: 60vh;
    overflow: auto;
    white-space: pre-wrap;
    font-size: 0.75rem;
    color: #17342c;
    background: #f4f7f6;
    padding: 0.5rem;
  }

  .yaml-error {
    margin: 0.5rem 0 0;
    color: #a03030;
    font-size: 0.8rem;
  }
</style>
