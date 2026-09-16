<script lang="ts">
  import { GraphStore } from "./graph.svelte.js";
  import Palette from "./Palette.svelte";
  import Canvas from "./Canvas.svelte";
  import PropertiesPanel from "./PropertiesPanel.svelte";

  const graph = new GraphStore();
  let yamlError = $state("");

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
</script>

<div class="workspace">
  <div class="toolbar">
    <button type="button" onclick={() => void downloadYaml()}>
      Download YAML
    </button>
    {#if yamlError}
      <p class="yaml-error">{yamlError}</p>
    {/if}
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

  .yaml-error {
    margin: 0.5rem 0 0;
    color: #a03030;
    font-size: 0.8rem;
  }
</style>
