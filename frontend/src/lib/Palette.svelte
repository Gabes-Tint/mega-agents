<script lang="ts">
  import { PALETTE, type PaletteItem } from "./graph.svelte.js";

  function startDrag(event: DragEvent, item: PaletteItem) {
    if (!event.dataTransfer) return;
    event.dataTransfer.setData("text/plain", item.type);
    event.dataTransfer.effectAllowed = "copy";
  }
</script>

<aside aria-label="Component palette">
  <h2>Components</h2>
  <ul>
    {#each PALETTE as item (item.type)}
      <li>
        <button
          type="button"
          draggable="true"
          ondragstart={(event) => startDrag(event, item)}
        >
          {item.label}
        </button>
      </li>
    {/each}
  </ul>
</aside>

<style>
  aside {
    padding: 0.75rem;
    background: #eef4f1;
    border-right: 1px solid #d5e0db;
    overflow-y: auto;
  }

  h2 {
    font-size: 0.85rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: #5b7a71;
    margin: 0 0 0.75rem;
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 0.5rem;
  }

  button {
    width: 100%;
    padding: 0.5rem 0.75rem;
    border: 1px solid #b8ccc4;
    border-radius: 0.375rem;
    background: #ffffff;
    color: #17342c;
    font: inherit;
    text-align: left;
    cursor: grab;
  }

  button:active {
    cursor: grabbing;
  }
</style>
