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

  function nodeById(id: string): GraphNode | undefined {
    return graph.nodes.find((candidate) => candidate.id === id);
  }

  function absolutePosition(node: GraphNode): { x: number; y: number } {
    const parent = node.parentId ? nodeById(node.parentId) : undefined;
    return { x: (parent?.x ?? 0) + node.x, y: (parent?.y ?? 0) + node.y };
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
      const project = graph.projectAt(contentX, contentY);
      if (project) {
        graph.addNode(
          data,
          contentX - project.x,
          contentY - project.y,
          project.id,
        );
        return;
      }
      graph.addNode(data, contentX, contentY);
      return;
    }
    if (data.startsWith("node:")) {
      const id = data.slice("node:".length);
      const node = nodeById(id);
      const grab = dragState;
      dragState = null;
      if (!node || !grab || grab.id !== id) return;
      const project = graph.projectAt(contentX, contentY);
      if (
        node.parentId === undefined &&
        project &&
        project.id !== id &&
        !graph.hasChildren(id)
      ) {
        graph.attachToProject(
          id,
          project.id,
          contentX - grab.dx,
          contentY - grab.dy,
        );
        return;
      }
      if (node.parentId) {
        const parent = nodeById(node.parentId);
        if (parent && project && project.id === parent.id) {
          graph.moveNode(
            id,
            contentX - grab.dx - parent.x,
            contentY - grab.dy - parent.y,
          );
        } else if (project) {
          // Re-parenting to another top-level project stays one level deep.
          graph.attachToProject(
            id,
            project.id,
            contentX - grab.dx,
            contentY - grab.dy,
          );
        } else {
          graph.detachNode(id, contentX - grab.dx, contentY - grab.dy);
        }
        return;
      }
      graph.moveNode(id, contentX - grab.dx, contentY - grab.dy);
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
      style:left="{absolutePosition(node).x}px"
      style:top="{absolutePosition(node).y}px"
      style:width="{node.w}px"
      style:height="{node.h}px"
      ondragstart={(event) => startNodeDrag(event, node)}
      onclick={() => graph.select(node.id)}
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
