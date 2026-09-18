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
  import AgentHealth from "./AgentHealth.svelte";
  import CodeEditor from "./CodeEditor.svelte";
  import ThemeToggle from "./ThemeToggle.svelte";
  import Splitter from "./Splitter.svelte";
  import {
    loadLayout,
    saveLayout,
    type PanelLayout,
    type PanelName,
  } from "./panelLayout.js";
  import { parseRoute, workflowPath, type Route } from "./workflowUrl.js";

  // What the backend said about itself, shown in the status bar.
  let { status = "" }: { status?: string } = $props();

  interface RunStep {
    nodeId: string;
    name: string;
    action: string;
    status: string;
    error?: string;
    details?: Record<string, unknown>;
    loop?: string;
    iteration?: number;
  }

  // One value a block the run waits before is about to receive.
  interface PausedInput {
    nodeId: string;
    port: string;
    value?: unknown;
  }

  // A block the run stopped in front of, as the run reports it: what it is
  // about to receive and the text it would run.
  interface PausedAt {
    nodeId: string;
    name: string;
    action: string;
    at?: string;
    loop?: string;
    iteration?: number;
    inputs?: PausedInput[];
    resolved: { prompt?: string; command?: string };
  }

  interface RunResult {
    id?: string;
    retryOf?: string;
    status: string;
    steps: RunStep[];
    // Several branches can wait at once, so the run names every block it
    // stopped before rather than one.
    paused?: PausedAt[];
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
    ["iterations", "Iterations"],
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

  // Marks a run's or a step's status, as the logs and the CLI do.
  const STATUS_EMOJI: Record<string, string> = {
    pending: "⏳",
    running: "▶️",
    succeeded: "✅",
    failed: "❌",
    skipped: "⏭️",
    cancelled: "🛑",
    interrupted: "⚠️",
  };

  function emojiOf(status: string): string {
    return STATUS_EMOJI[status] ?? "•";
  }

  function fieldErrors(details: Record<string, unknown> | undefined): string[] {
    const errors = details?.errors;
    if (!Array.isArray(errors)) return [];
    return errors.map(
      (entry: { path?: string; message?: string }) =>
        `${entry.path ?? "$"}: ${entry.message ?? ""}`,
    );
  }

  // The scratch graph at "/" survives reloads of the page in this browser.
  // A saved workflow is not kept here: it has its own address and comes back
  // from the backend, which is where Save puts it.
  const DRAFT_KEY = "mega-agents:draft";

  const graph = new GraphStore();
  // What the address bar points at. Every saved workflow has its own URL, so
  // it can be bookmarked and shared, and Back walks between workflows.
  const opened = parseRoute(globalThis.location.pathname);
  let route = $state<Route>(opened);
  // Names what the address points at when nothing answers to it, instead of
  // leaving an empty canvas.
  let notFound = $state("");

  function loadDraft(): void {
    try {
      const draft = localStorage.getItem(DRAFT_KEY);
      graph.load(
        draft ? (JSON.parse(draft) as WorkflowGraph) : { nodes: [], edges: [] },
      );
    } catch {
      // A missing or unreadable draft starts an empty graph.
      graph.load({ nodes: [], edges: [] });
    }
  }
  if (opened.kind === "scratch") loadDraft();

  $effect(() => {
    const draft = JSON.stringify(graph.toRequest());
    if (route.kind !== "scratch") return;
    try {
      localStorage.setItem(DRAFT_KEY, draft);
    } catch {
      // Storage may be unavailable; the graph still works for this page.
    }
  });

  // The bottom panel shows what to fix before running, or the run.
  let panelTab = $state<"problems" | "run">("problems");
  let layout = $state<PanelLayout>(loadLayout());

  function resizePanel(name: PanelName, size: number): void {
    layout[name] = size;
    saveLayout(layout);
  }
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
      if (inProgress(opened.status) && !running) {
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
      // The saved workflow now has an address. Replacing rather than pushing
      // keeps Back out of the editor the graph was drawn in.
      if (response.ok) moveTo(workflowPath(name), "replace");
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

  // Opens the workflow chosen in the dialog and takes the editor to its
  // address, so Back returns to the workflow that was open before it.
  async function openWorkflow(name: string): Promise<void> {
    openDialog = false;
    if (await loadWorkflow(name)) moveTo(workflowPath(name), "push");
  }

  // Loads the named workflow onto the canvas. A name nothing answers to is
  // reported as missing rather than leaving the canvas as it was.
  async function loadWorkflow(name: string): Promise<boolean> {
    try {
      const response = await fetch(
        `/api/workflows/${encodeURIComponent(name)}`,
      );
      if (!response.ok) {
        if (response.status === 404)
          notFound = `There is no workflow named “${name}”.`;
        else fileStatus = (await response.text()).trim();
        return false;
      }
      graph.load((await response.json()) as WorkflowGraph);
      runResult = null;
      fileStatus = `Opened ${name}`;
      return true;
    } catch {
      fileStatus = "Cannot reach the backend";
      return false;
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

  // Takes the address bar to the path and remembers it. A push leaves a step
  // back to the page the editor was on; a replace does not.
  function moveTo(path: string, how: "push" | "replace"): void {
    if (how === "push") globalThis.history.pushState(null, "", path);
    else globalThis.history.replaceState(null, "", path);
    route = parseRoute(path);
  }

  // Opens what the address bar points at: the scratch graph at "/", a saved
  // workflow at /workflows/<name>, or a message for an address the editor
  // has no page for.
  async function applyLocation(): Promise<void> {
    route = parseRoute(globalThis.location.pathname);
    notFound = "";
    if (route.kind === "workflow") await loadWorkflow(route.name);
    else if (route.kind === "unknown")
      notFound = `There is no page at ${route.path}.`;
    else loadDraft();
  }

  function backToEditor(): void {
    // The address led nowhere, so it is replaced rather than left in history.
    moveTo("/", "replace");
    notFound = "";
    loadDraft();
  }

  // The address the editor opened at. "/" already loaded the draft above.
  if (opened.kind !== "scratch") void applyLocation();

  // Back and Forward move between the workflows that were open.
  $effect(() => {
    const follow = () => void applyLocation();
    globalThis.addEventListener("popstate", follow);
    return () => globalThis.removeEventListener("popstate", follow);
  });

  const loggedStep = $derived(
    runResult?.steps.find((step) => step.nodeId === graph.logNodeId),
  );

  // The text someone typed at a breakpoint, by the block it waits before.
  // It belongs to that pause: the block's own text never changes.
  let edits = $state<Record<string, string>>({});
  // The block whose resume is on its way, so one click is never sent twice.
  let resuming = $state("");
  let pauseError = $state("");

  const pausedAt = $derived(runResult?.paused ?? []);

  function wait(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  function showResult(result: RunResult): void {
    runResult = result;
    // An edit belongs to the pause it was typed at, so it goes when the run
    // leaves the block, and so does the wait for that block's resume.
    const waiting = new Set((result.paused ?? []).map((at) => at.nodeId));
    for (const nodeId of Object.keys(edits))
      if (!waiting.has(nodeId)) delete edits[nodeId];
    if (resuming && !waiting.has(resuming)) resuming = "";
    graph.showRun(result.steps, result.id ?? null);
    if (graph.logNodeId) void loadLog(graph.logNodeId);
  }

  // Which text a paused block would run, or nothing for a block whose work
  // has no text of its own.
  function fieldOf(at: PausedAt): "prompt" | "command" | undefined {
    if (at.resolved.command !== undefined) return "command";
    if (at.resolved.prompt !== undefined) return "prompt";
    return undefined;
  }

  const resolvedText = (at: PausedAt) =>
    at.resolved.command ?? at.resolved.prompt ?? "";

  const textOf = (at: PausedAt) => edits[at.nodeId] ?? resolvedText(at);

  const isEdited = (at: PausedAt) => textOf(at) !== resolvedText(at);

  function undoEdit(at: PausedAt): void {
    delete edits[at.nodeId];
  }

  function valueText(value: unknown): string {
    return typeof value === "string" ? value : JSON.stringify(value, null, 2);
  }

  // Reads the run again after a resume the backend refused, so the canvas
  // and the panel show where the run actually is.
  async function reread(id: string): Promise<void> {
    try {
      const response = await fetch(`/api/runs/${encodeURIComponent(id)}`);
      if (response.ok) showResult((await response.json()) as RunResult);
    } catch {
      // The message already says what happened; the poll keeps trying.
    }
  }

  // Takes one paused block on. The block is always named: with two branches
  // waiting, a resume that named none would be refused.
  async function resume(at: PausedAt, action: string): Promise<void> {
    const id = runResult?.id;
    if (!id) return;
    resuming = at.nodeId;
    pauseError = "";
    const body: Record<string, string> = { nodeId: at.nodeId, action };
    const field = fieldOf(at);
    // A skipped block runs nothing, so an edit has nothing to apply to.
    if (field && action !== "skip" && isEdited(at)) body[field] = textOf(at);
    try {
      const response = await fetch(
        `/api/runs/${encodeURIComponent(id)}/resume`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        },
      );
      if (response.ok) return;
      pauseError = (await response.text()).trim();
      resuming = "";
      await reread(id);
    } catch {
      pauseError = "Cannot reach the backend";
      resuming = "";
    }
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

  // A run waiting at a breakpoint is still in progress: it is followed, and
  // it can still be cancelled, until someone takes it on and it finishes.
  const inProgress = (status: string) =>
    status === "running" || status === "paused";

  // A run the backend started keeps executing after the request returns;
  // poll it so the canvas and the result follow each step.
  async function follow(result: RunResult): Promise<void> {
    let current = result;
    while (current.id && inProgress(current.status)) {
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
    pauseError = "";
    edits = {};
    resuming = "";
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

<div
  class="workspace"
  style:--palette-width="{layout.palette}px"
  style:--properties-width="{layout.properties}px"
  style:--panel-height="{layout.panel}px"
>
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
      {#if running && runResult?.id && inProgress(runResult.status)}
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
                  >{emojiOf(pastRun.status)}
                  {pastRun.workflow} · {pastRun.status} · {pastRun.startedAt}</button
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
  <Splitter
    label="Resize components sidebar"
    name="palette"
    size={layout.palette}
    grows="right"
    onresize={(size) => resizePanel("palette", size)}
  />
  <Splitter
    label="Resize properties sidebar"
    name="properties"
    size={layout.properties}
    grows="left"
    onresize={(size) => resizePanel("properties", size)}
  />
  <Splitter
    label="Resize output panel"
    name="panel"
    size={layout.panel}
    grows="up"
    onresize={(size) => resizePanel("panel", size)}
  />
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
      {#if pauseError}
        <p class="error" role="alert">{pauseError}</p>
      {/if}
      {#if pausedAt.length > 0}
        <section class="paused" aria-label="Paused at a breakpoint">
          {#each pausedAt as at (at.nodeId)}
            {@const field = fieldOf(at)}
            <article class="pause">
              <div class="step-line">
                <strong>
                  ⏸️ Paused before {at.name}: {at.action}
                </strong>
                {#if at.iteration}
                  <span class="meta">repeat {at.iteration}</span>
                {/if}
                <button
                  type="button"
                  class="small ghost"
                  aria-label="Show {at.name} on the canvas"
                  onclick={() => graph.select(at.nodeId)}
                >
                  Show on canvas
                </button>
              </div>
              {#if at.inputs?.length}
                <dl class="arrivals">
                  {#each at.inputs as input (input.nodeId + input.port)}
                    <dt>
                      {nameOf(input.nodeId) || input.nodeId} · {input.port}
                    </dt>
                    <dd><pre>{valueText(input.value)}</pre></dd>
                  {/each}
                </dl>
              {:else}
                <p class="details">Nothing reaches this block.</p>
              {/if}
              {#if field}
                <div class="this-run" class:edited={isEdited(at)}>
                  <span class="run-only">
                    {field === "command" ? "Command" : "Prompt"} for this run only
                  </span>
                  <CodeEditor
                    label="{field === 'command'
                      ? 'Command'
                      : 'Prompt'} for {at.name} in this run"
                    language={field === "command" ? "shell" : "text"}
                    minLines={3}
                    value={textOf(at)}
                    onchange={(value) => (edits[at.nodeId] = value)}
                  />
                  <p class="details">
                    Continue and Step run this text as written. {at.name} keeps the
                    text the workflow holds; nothing typed here is saved.
                    {#if isEdited(at)}
                      <button
                        type="button"
                        class="small ghost"
                        aria-label="Undo the edit to {at.name}"
                        onclick={() => undoEdit(at)}
                      >
                        Undo the edit
                      </button>
                    {/if}
                  </p>
                </div>
              {/if}
              <div class="group">
                <button
                  type="button"
                  class="small"
                  aria-label="Continue {at.name}"
                  disabled={resuming === at.nodeId}
                  onclick={() => void resume(at, "continue")}
                >
                  Continue
                </button>
                <button
                  type="button"
                  class="small"
                  aria-label="Step {at.name}"
                  disabled={resuming === at.nodeId}
                  onclick={() => void resume(at, "step")}
                >
                  Step
                </button>
                <button
                  type="button"
                  class="small"
                  aria-label="Skip {at.name}"
                  disabled={resuming === at.nodeId}
                  onclick={() => void resume(at, "skip")}
                >
                  Skip
                </button>
              </div>
            </article>
          {/each}
        </section>
      {/if}
      <section aria-label="Run result" aria-live="polite">
        <div class="run-summary">
          {#if runResult && !runError}
            <span class="run-status {runResult.status}">
              {emojiOf(runResult.status)} Run {runResult.status}
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
                  <strong>
                    {emojiOf(step.status)}
                    {step.name}: {step.action}
                    {step.status}
                  </strong>
                  {#if step.iteration}
                    <span class="meta">repeat {step.iteration}</span>
                  {/if}
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
    <AgentHealth />
    <span class="backend">{status}</span>
    <span class="file-status" role="status" aria-label="Workflow file"
      >{fileStatus}</span
    >
  </footer>
</div>

{#if notFound}
  <!-- The address named no workflow, so the editor says which one and offers
       the way back instead of showing an empty canvas. -->
  <section class="not-found" aria-label="Page not found">
    <h2>{notFound}</h2>
    {#if route.kind === "workflow"}
      <p>It may have been renamed, deleted, or never saved under that name.</p>
    {/if}
    <button type="button" onclick={backToEditor}>Back to the editor</button>
  </section>
{/if}

<style>
  .not-found {
    position: fixed;
    inset: 0;
    z-index: 20;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.75rem;
    padding: 2rem;
    text-align: center;
    background: var(--surface);
    color: var(--text);
  }

  .not-found h2 {
    margin: 0;
    font-size: 1.1rem;
  }

  .not-found p {
    margin: 0;
    color: var(--text-muted);
  }

  .workspace {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: var(--palette-width) minmax(0, 1fr) var(
        --properties-width
      );
    grid-template-rows: auto minmax(0, 1fr) var(--panel-height) auto;
    grid-template-areas:
      "top top top"
      "palette canvas properties"
      "palette panel properties"
      "status status status";
  }

  .workspace > :global(aside[aria-label="Component palette"]) {
    grid-area: palette;
  }

  .workspace > :global(.canvas-area) {
    grid-area: canvas;
  }

  .workspace > :global(aside[aria-label="Node properties"]) {
    grid-area: properties;
  }

  /* Each splitter overlays the border between its panel and the canvas. */
  .workspace > :global(.splitter-palette) {
    grid-area: palette;
    justify-self: end;
    margin-right: -4px;
  }

  .workspace > :global(.splitter-properties) {
    grid-area: properties;
    justify-self: start;
    margin-left: -4px;
  }

  .workspace > :global(.splitter-panel) {
    grid-area: panel;
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

  /* A paused run is the one thing in the panel waiting on the person
     reading it, so each block it waits before is a card of its own. */
  .paused {
    display: grid;
    gap: 0.4rem;
    margin-bottom: 0.6rem;
  }

  .pause {
    display: grid;
    gap: 0.35rem;
    padding: 0.45rem 0.6rem;
    border: 1px solid var(--warn);
    border-left-width: 3px;
    border-radius: var(--radius);
    background: var(--warn-soft);
  }

  .arrivals {
    margin: 0;
    padding-left: 1rem;
    font-size: 12px;
  }

  .arrivals dt {
    color: var(--text-muted);
  }

  .arrivals dd {
    margin: 0;
  }

  /* The text of the run, not of the block: dashed so it never reads as one
     of the saved fields in the properties panel. */
  .this-run {
    display: grid;
    gap: 0.2rem;
    padding: 0.4rem;
    border: 1px dashed var(--warn);
    border-radius: var(--radius);
    background: var(--surface);
  }

  .run-only {
    color: var(--warn);
    font-size: 11px;
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .this-run.edited {
    border-style: solid;
    box-shadow: 0 0 0 2px var(--warn-soft);
  }

  .this-run.edited .run-only::after {
    content: " · edited";
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
