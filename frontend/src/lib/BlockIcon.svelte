<script lang="ts">
  import { BLOCK_ICONS, blockKind, iconOf } from "./blockIcons.js";
  import { labelFor, type GraphNode } from "./graph.svelte.js";

  let { node }: { node: Pick<GraphNode, "type" | "action"> } = $props();

  const icon = $derived(iconOf(node));
  const shape = $derived(BLOCK_ICONS[icon]);
  const kind = $derived(blockKind(node.type));
</script>

<span
  class="block-icon {kind}"
  data-icon={icon}
  title="{labelFor(node.type)} · {kind}"
  aria-hidden="true"
>
  <svg viewBox="0 0 24 24">
    {#if shape?.mark}
      <path class="mark" d={shape.mark} />
    {/if}
    {#if shape?.tone}
      <path class="tone" d={shape.tone} />
    {/if}
    {#if shape?.line}
      <path class="line" d={shape.line} />
    {/if}
  </svg>
</span>

<style>
  /* Objects sit on a rounded square, actions on a circle; both take the
     block's own color. */
  .block-icon {
    display: inline-grid;
    place-items: center;
    width: 18px;
    height: 18px;
    border-radius: 5px;
    background: var(--block-body);
    color: var(--block-accent);
    box-shadow: inset 0 0 0 1px var(--block-border);
  }

  .block-icon.action {
    border-radius: 50%;
  }

  svg {
    width: 12px;
    height: 12px;
    overflow: visible;
  }

  .mark {
    fill: currentColor;
  }

  .tone {
    fill: currentColor;
    opacity: 0.28;
  }

  .line {
    fill: none;
    stroke: currentColor;
    stroke-width: 2;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
</style>
