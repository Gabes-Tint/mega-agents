<script lang="ts">
  import { GraphStore } from "./graph.svelte.js";
  import Palette from "./Palette.svelte";
  import Canvas from "./Canvas.svelte";
  import PropertiesPanel from "./PropertiesPanel.svelte";

  interface RunStep {
    nodeId: string;
    name: string;
    action: string;
    status: "succeeded" | "failed";
    remote?: string;
    output?: string;
    error?: string;
  }

  interface RunResult {
    status: "succeeded" | "failed";
    steps: RunStep[];
  }

  const graph = new GraphStore();
  let yamlError = $state("");
  let running = $state(false);
  let runResult = $state<RunResult | null>(null);
  let runError = $state("");

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
        body: JSON.stringify({
          nodes: graph.nodes,
          edges: graph.edges,
        }),
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
    try {
      const response = await fetch("/api/runs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          nodes: graph.nodes,
          edges: graph.edges,
        }),
      });
      if (!response.ok) {
        runError = (await response.text()).trim();
        return;
      }
      runResult = (await response.json()) as RunResult;
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
      <button type="button" onclick={() => void downloadYaml()}>
        Download YAML
      </button>
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
              {#if step.remote}
                <span>Remote: {step.remote}</span>
              {/if}
              {#if step.error}
                <pre class="failed">{step.error}</pre>
              {:else if step.output}
                <pre>{step.output}</pre>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  </div>
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
    gap: 0.5rem;
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

  .yaml-error {
    margin: 0.5rem 0 0;
    color: #a03030;
    font-size: 0.8rem;
  }
</style>
