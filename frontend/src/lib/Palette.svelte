<script lang="ts">
  import {
    BLOCK_HUES,
    PALETTE,
    type GraphStore,
    type NodeType,
    type PaletteItem,
  } from "./graph.svelte.js";

  let { graph }: { graph: GraphStore } = $props();

  // Where the work happens, then what does the work.
  const GROUPS: { title: string; types: readonly NodeType[] }[] = [
    { title: "Sources", types: ["project", "github", "gitlab", "githubapp"] },
    { title: "Steps", types: ["agent", "command", "jsonschema", "router"] },
  ];

  function itemsOf(types: readonly NodeType[]): PaletteItem[] {
    return PALETTE.filter((item) => types.includes(item.type));
  }

  // The entry whose explanation shows, and where to place it: beside the
  // entry, outside the scrolling palette.
  let explained = $state<{ item: PaletteItem; x: number; y: number } | null>(
    null,
  );

  function explain(event: Event, item: PaletteItem) {
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    explained = { item, x: rect.right + 8, y: rect.top + rect.height / 2 };
  }

  function startDrag(event: DragEvent, item: PaletteItem) {
    explained = null;
    if (!event.dataTransfer) return;
    event.dataTransfer.setData("text/plain", item.type);
    event.dataTransfer.effectAllowed = "copy";
    graph.draggingType = item.type;
  }

  function endDrag() {
    graph.draggingType = null;
  }
</script>

<aside aria-label="Component palette" ondragend={endDrag}>
  <h2>Components</h2>
  {#each GROUPS as group (group.title)}
    <h3>{group.title}</h3>
    <ul>
      {#each itemsOf(group.types) as item (item.type)}
        <li>
          <button
            type="button"
            class="block-{item.type}"
            style:--block-hue={BLOCK_HUES[item.type]}
            draggable="true"
            aria-describedby={explained?.item.type === item.type
              ? "palette-tooltip"
              : undefined}
            onmouseenter={(event) => explain(event, item)}
            onfocus={(event) => explain(event, item)}
            onmouseleave={() => (explained = null)}
            onblur={() => (explained = null)}
            ondragstart={(event) => startDrag(event, item)}
          >
            <span class="swatch" aria-hidden="true"></span>
            {item.label}
          </button>
        </li>
      {/each}
    </ul>
  {/each}
  <p class="tip">Drag a component onto the canvas.</p>
  {#if explained}
    <div
      id="palette-tooltip"
      role="tooltip"
      class="tooltip"
      style:--block-hue={BLOCK_HUES[explained.item.type]}
      style:left="{explained.x}px"
      style:top="{explained.y}px"
    >
      <strong>{explained.item.label}</strong>
      {explained.item.description}
    </div>
  {/if}
</aside>

<style>
  aside {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    min-height: 0;
    padding: 0.75rem 0.6rem;
    background: var(--surface);
    border-right: 1px solid var(--border);
    overflow-y: auto;
  }

  h2 {
    margin: 0 0 0.25rem 0.35rem;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-muted);
  }

  h3 {
    margin: 0.6rem 0 0.2rem 0.35rem;
    font-size: 11px;
    font-weight: 500;
    color: var(--text-faint);
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 0.1rem;
  }

  button {
    justify-content: flex-start;
    width: 100%;
    gap: 0.55rem;
    border-color: transparent;
    background: transparent;
    cursor: grab;
  }

  button:hover:not(:disabled) {
    border-color: var(--border);
    background: var(--surface-sunken);
  }

  button:active {
    cursor: grabbing;
  }

  .swatch {
    flex: none;
    width: 0.7rem;
    height: 0.7rem;
    border-radius: 3px;
    background: var(--block-accent);
  }

  .tooltip {
    position: fixed;
    z-index: 10;
    display: grid;
    gap: 0.2rem;
    width: max-content;
    max-width: 17rem;
    padding: 0.5rem 0.65rem;
    border: 1px solid var(--border);
    border-left: 3px solid var(--block-accent);
    border-radius: var(--radius);
    background: var(--surface);
    color: var(--text-muted);
    font-size: 12px;
    line-height: 1.45;
    box-shadow: var(--shadow-lifted);
    transform: translateY(-50%);
    pointer-events: none;
    animation: appear 120ms ease-out 250ms both;
  }

  .tooltip strong {
    color: var(--text);
    font-weight: 600;
  }

  @keyframes appear {
    from {
      opacity: 0;
    }
  }

  .tip {
    margin: auto 0.35rem 0;
    padding-top: 1rem;
    font-size: 12px;
    color: var(--text-faint);
  }
</style>
