<script lang="ts">
  import {
    canHostChild,
    GraphStore,
    PALETTE,
    type GraphNode,
    type NodeType,
  } from "./graph.svelte.js";

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
    return PALETTE.some((item) => item.type === value);
  }

  function nodeById(id: string): GraphNode | undefined {
    return graph.nodes.find((candidate) => candidate.id === id);
  }

  function absolutePosition(node: GraphNode): { x: number; y: number } {
    const parent = node.parentId ? nodeById(node.parentId) : undefined;
    return { x: (parent?.x ?? 0) + node.x, y: (parent?.y ?? 0) + node.y };
  }

  function edgeBounds(): { width: number; height: number } {
    let width = 800;
    let height = 600;
    for (const node of graph.nodes) {
      const position = absolutePosition(node);
      width = Math.max(width, position.x + node.w + 100);
      height = Math.max(height, position.y + node.h + 100);
    }
    return { width, height };
  }

  const extent = $derived.by(() => edgeBounds());

  // Point where the straight line to (targetX, targetY) crosses this box's
  // border, so arrowheads stay visible instead of hiding under the box.
  function borderPoint(
    node: GraphNode,
    x: number,
    y: number,
    targetX: number,
    targetY: number,
  ): { x: number; y: number } {
    const dx = targetX - x;
    const dy = targetY - y;
    const t = Math.min(
      dx === 0 ? Number.POSITIVE_INFINITY : node.w / 2 / Math.abs(dx),
      dy === 0 ? Number.POSITIVE_INFINITY : node.h / 2 / Math.abs(dy),
    );
    return { x: x + dx * t, y: y + dy * t };
  }

  function selectOrConnect(node: GraphNode): void {
    if (graph.connecting && graph.connectFromId) {
      graph.connect(graph.connectFromId, node.id);
      graph.cancelConnect();
      return;
    }
    graph.select(node.id);
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
    const contentX = event.clientX - rect.left + scrollLeft;
    const contentY = event.clientY - rect.top + scrollTop;
    const data = event.dataTransfer?.getData("text/plain") ?? "";
    if (isComponentType(data)) {
      const container = graph.containerAt(contentX, contentY);
      if (container && canHostChild(container.type, data)) {
        graph.addNode(
          data,
          contentX - container.x,
          contentY - container.y,
          container.id,
        );
        return;
      }
      // Incompatible containers ignore the drop, so the new box lands at
      // the content position as a top-level box.
      graph.addNode(data, contentX, contentY);
      return;
    }
    if (data.startsWith("node:")) {
      const id = data.slice("node:".length);
      const node = nodeById(id);
      const grab = dragState;
      dragState = null;
      if (!node || !grab || grab.id !== id) return;
      const container = graph.containerAt(contentX, contentY);
      if (node.parentId === undefined) {
        if (
          container &&
          container.id !== id &&
          !graph.hasChildren(id) &&
          canHostChild(container.type, node.type)
        ) {
          graph.attachToContainer(
            id,
            container.id,
            contentX - grab.dx,
            contentY - grab.dy,
          );
          return;
        }
        graph.moveNode(id, contentX - grab.dx, contentY - grab.dy);
        return;
      }
      const parent = nodeById(node.parentId);
      if (!parent) {
        graph.moveNode(id, contentX - grab.dx, contentY - grab.dy);
        return;
      }
      if (container && container.id === parent.id) {
        graph.moveNode(
          id,
          contentX - grab.dx - parent.x,
          contentY - grab.dy - parent.y,
        );
      } else if (container && canHostChild(container.type, node.type)) {
        // Re-parenting to another compatible container stays one level deep.
        graph.attachToContainer(
          id,
          container.id,
          contentX - grab.dx,
          contentY - grab.dy,
        );
      } else if (container) {
        // An incompatible target container keeps the box in its own parent,
        // repositioned under the pointer.
        graph.moveNode(
          id,
          contentX - grab.dx - parent.x,
          contentY - grab.dy - parent.y,
        );
      } else {
        graph.detachNode(id, contentX - grab.dx, contentY - grab.dy);
      }
      return;
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
  <svg
    class="edges"
    aria-hidden="true"
    style:width="{extent.width}px"
    style:height="{extent.height}px"
  >
    <defs>
      <marker
        id="edge-arrowhead"
        markerWidth="8"
        markerHeight="8"
        refX="7"
        refY="4"
        orient="auto"
      >
        <path d="M0 0 L8 4 L0 8 Z" fill="#5b7a71" />
      </marker>
    </defs>
    {#each graph.edges as edge (edge.id)}
      {@const from = nodeById(edge.from)}
      {@const to = nodeById(edge.to)}
      {#if from && to}
        {@const fromPosition = absolutePosition(from)}
        {@const toPosition = absolutePosition(to)}
        {@const start = borderPoint(
          from,
          fromPosition.x + from.w / 2,
          fromPosition.y + from.h / 2,
          toPosition.x + to.w / 2,
          toPosition.y + to.h / 2,
        )}
        {@const end = borderPoint(
          to,
          toPosition.x + to.w / 2,
          toPosition.y + to.h / 2,
          fromPosition.x + from.w / 2,
          fromPosition.y + from.h / 2,
        )}
        <line
          class="edge-line"
          x1={start.x}
          y1={start.y}
          x2={end.x}
          y2={end.y}
          marker-end="url(#edge-arrowhead)"
        ></line>
      {/if}
    {/each}
  </svg>
  {#each graph.nodes as node (node.id)}
    <button
      type="button"
      class="node"
      class:selected={node.id === graph.selectedId}
      aria-pressed={node.id === graph.selectedId}
      draggable="true"
      style:left="{absolutePosition(node).x}px"
      style:top="{absolutePosition(node).y}px"
      style:width="{node.w}px"
      style:height="{node.h}px"
      ondragstart={(event) => startNodeDrag(event, node)}
      onclick={() => selectOrConnect(node)}
    >
      <span class="node-title">
        {node.name}
        {#if node.start}
          <span class="start-flag" aria-hidden="true">▶</span>
        {/if}
      </span>
      <span class="node-separator" aria-hidden="true"></span>
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

  .edges {
    position: absolute;
    left: 0;
    top: 0;
    pointer-events: none;
    overflow: visible;
  }

  .edge-line {
    stroke: #5b7a71;
    stroke-width: 2;
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
    display: flex;
    flex-direction: column;
    align-items: stretch;
    padding: 0;
    border: 1px solid #b8ccc4;
    border-radius: 0.5rem;
    background: #ffffff;
    color: #17342c;
    font: inherit;
    text-align: left;
    overflow: hidden;
    cursor: pointer;
    box-shadow: 0 1px 2px rgb(23 52 44 / 0.12);
  }

  .node-title {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    padding: 0.45rem 0.75rem;
    font-weight: 600;
    background: #f0f6f3;
    white-space: nowrap;
  }

  .node-separator {
    display: block;
    height: 1px;
    background: #b8ccc4;
  }

  .start-flag {
    color: #ff3e00;
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
