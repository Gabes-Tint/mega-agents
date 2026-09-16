<script lang="ts">
  import { GraphStore } from "./graph.svelte.js";
  import Palette from "./Palette.svelte";
  import Canvas from "./Canvas.svelte";
  import PropertiesPanel from "./PropertiesPanel.svelte";

  const graph = new GraphStore();
</script>

<div class="workspace">
  <Palette />
  <Canvas {graph} />
  <PropertiesPanel {graph} />
</div>

<style>
  .workspace {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: 200px 1fr 260px;
  }
</style>
