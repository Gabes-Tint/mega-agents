<script lang="ts">
  import { GraphStore, type GraphNode, type NodeType } from "./graph.svelte.js";

  let { graph }: { graph: GraphStore } = $props();

  // Grab offset of the node being dragged, kept outside the template because
  // it is transient drag bookkeeping rather than rendered state.
  let dragState: { id: string; dx: number; dy: number } | null = null;

  // Starting pointer position and box size of an active resize gesture.
  let resizeState: {
    id: string;
    startX: number;
    startY: number;
    w: number;
    h: number;
  } | null = null;

  function isComponentType(value: string): value is NodeType {
    return value === "agent" || value === "tool" || value === "project";
  }

  function allowDrop(event: DragEvent) {
    event.preventDefault();
  }

  function startNodeDrag(
    event: DragEvent & { currentTarget: EventTarget & HTMLButtonElement },
    node: GraphNode,
  ) {
    if (!event.dataTransfer) return;
    const rect = event.currentTarget.getBoundingClientRect();
    event.dataTransfer.setData("text/plain", `node:${node.id}`);
    event.dataTransfer.effectAllowed = "move";
    dragState = {
      id: node.id,
      dx: event.clientX - rect.left,
      dy: event.clientY - rect.top,
    };
  }

  function startResize(
    event: PointerEvent & { currentTarget: EventTarget & HTMLElement },
    node: GraphNode,
  ) {
    if (event.button !== 0) return;
    // Cancelling the pointerdown default prevents the browser from turning
    // the gesture into a text selection or the node's own drag-and-drop.
    event.preventDefault();
    event.stopPropagation();
    event.currentTarget.setPointerCapture?.(event.pointerId);
    graph.select(node.id);
    resizeState = {
      id: node.id,
      startX: event.clientX,
      startY: event.clientY,
      w: node.w,
      h: node.h,
    };
  }

  function applyResize(
    event: PointerEvent & { currentTarget: EventTarget & Window },
  ) {
    const active = resizeState;
    if (!active) return;
    graph.resizeNode(
      active.id,
      active.w + (event.clientX - active.startX),
      active.h + (event.clientY - active.startY),
    );
  }

  function endResize() {
    resizeState = null;
  }

  function drop(
    event: DragEvent & { currentTarget: EventTarget & HTMLDivElement },
  ) {
    event.preventDefault();
    const target = event.currentTarget;
    const rect = target.getBoundingClientRect();
    // Node coordinates are content-space; pointer coordinates are viewport
    // space, so scrolled canvases need the scroll offset added back.
    const scrollLeft = target.scrollLeft;
    const scrollTop = target.scrollTop;
    const data = event.dataTransfer?.getData("text/plain") ?? "";
    if (isComponentType(data)) {
      graph.addNode(
        data,
        event.clientX - rect.left + scrollLeft,
        event.clientY - rect.top + scrollTop,
      );
      return;
    }
    if (data.startsWith("node:")) {
      const id = data.slice("node:".length);
      const grab = dragState;
      dragState = null;
      if (grab && grab.id === id) {
        graph.moveNode(
          id,
          event.clientX - rect.left + scrollLeft - grab.dx,
          event.clientY - rect.top + scrollTop - grab.dy,
        );
      }
    }
  }
</script>

<div
  class="canvas"
  role="region"
  aria-label="Graph canvas"
  ondragover={allowDrop}
  ondrop={drop}
>
  {#each graph.nodes as node (node.id)}
    <button
      type="button"
      class="node"
      class:selected={node.id === graph.selectedId}
      aria-pressed={node.id === graph.selectedId}
      draggable="true"
      style:left="{node.x}px"
      style:top="{node.y}px"
      style:width="{node.w}px"
      style:height="{node.h}px"
      ondragstart={(event) => startNodeDrag(event, node)}
      onclick={() => graph.select(node.id)}
    >
      {node.name}
      <span
        class="resize-handle"
        aria-hidden="true"
        onpointerdown={(event) => startResize(event, node)}
      ></span>
    </button>
  {/each}
  {#if graph.nodes.length === 0}
    <p class="hint">Drag components here to build your graph</p>
  {/if}
</div>

<svelte:window
  onpointermove={applyResize}
  onpointerup={endResize}
  onpointercancel={endResize}
/>

<style>
  .canvas {
    position: relative;
    overflow: auto;
    background:
      radial-gradient(#d9e5e0 1px, transparent 1px) 0 0 / 1.5rem 1.5rem,
      #fbfdfc;
  }

  .hint {
    position: absolute;
    inset: 0;
    display: grid;
    place-content: center;
    color: #7d968d;
    pointer-events: none;
    margin: 0;
  }

  .node {
    position: absolute;
    padding: 0.4rem 0.75rem;
    border: 1px solid #b8ccc4;
    border-radius: 0.5rem;
    background: #ffffff;
    color: #17342c;
    font: inherit;
    text-align: left;
    cursor: pointer;
    box-shadow: 0 1px 2px rgb(23 52 44 / 0.12);
  }

  .resize-handle {
    position: absolute;
    right: 0;
    bottom: 0;
    width: 14px;
    height: 14px;
    touch-action: none;
    border-radius: 0 0 0.4rem 0;
    cursor: nwse-resize;
    background: linear-gradient(
      135deg,
      transparent 0 45%,
      #b8ccc4 45% 55%,
      transparent 55% 70%,
      #b8ccc4 70% 80%,
      transparent 80%
    );
  }

  .node.selected {
    border-color: #ff3e00;
    box-shadow: 0 0 0 2px rgb(255 62 0 / 0.35);
  }
</style>
