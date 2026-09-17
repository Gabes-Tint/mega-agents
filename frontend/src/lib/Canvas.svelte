<script lang="ts">
  import {
    canExistTopLevel,
    canHostChild,
    GraphStore,
    PALETTE,
    rejectedDropHint,
    shortNodeId,
    type GraphNode,
    type NodeType,
  } from "./graph.svelte.js";

  let { graph }: { graph: GraphStore } = $props();

  // Transient explanation for a palette drop that was rejected by the
  // containment rules; cleared on the next drop so it cannot go stale.
  let dropHint = $state("");

  // Grab offset of the node being dragged, kept outside the template because
  // it is transient drag bookkeeping rather than rendered state.
  let dragState: { id: string; dx: number; dy: number } | null = null;

  // Live drop preview for the dragover handler: the container currently
  // under the pointer and whether it accepts the dragged block. The palette
  // component being dragged travels through graph.draggingType because
  // dataTransfer.getData is protected while a drag is in flight.
  let previewTargetId = $state<string | null>(null);
  let previewValid = $state(false);
  let previewing = $state(false);

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

  function edgeBounds(): { width: number; height: number } {
    let width = 800;
    let height = 600;
    for (const node of graph.nodes) {
      const position = graph.absolutePosition(node);
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

  function draggedType(): NodeType | undefined {
    return (
      graph.draggingType ??
      (dragState ? nodeById(dragState.id)?.type : undefined)
    );
  }

  // Live validation while a drag is in flight: resolve the container under
  // the pointer and preview whether the containment matrix accepts the
  // dragged block there. Runs on every dragover so feedback is immediate.
  function previewDrop(
    event: DragEvent & { currentTarget: EventTarget & HTMLDivElement },
  ) {
    event.preventDefault();
    const type = draggedType();
    if (!type) return;
    const target = event.currentTarget;
    const rect = target.getBoundingClientRect();
    const contentX = event.clientX - rect.left + target.scrollLeft;
    const contentY = event.clientY - rect.top + target.scrollTop;
    const container = graph.containerAt(contentX, contentY, dragState?.id);
    // Mirrors the drop logic: a valid container accepts the block, otherwise
    // a root-permitted block may still land at the top level.
    const valid = container
      ? canHostChild(container.type, type) || canExistTopLevel(type)
      : canExistTopLevel(type);
    previewing = true;
    previewTargetId = container?.id ?? null;
    previewValid = valid;
    if (event.dataTransfer) {
      // A dragged box always permits the drop: the drop handler decides
      // between moving, re-parenting, or staying put. Palette drags are
      // blocked on invalid targets so the drop never fires there.
      event.dataTransfer.dropEffect = dragState
        ? "move"
        : valid
          ? "copy"
          : "none";
    }
  }

  function clearPreview(
    event?: DragEvent & { currentTarget: EventTarget & HTMLDivElement },
  ) {
    if (event && event.relatedTarget) {
      const target = event.currentTarget;
      if (target.contains(event.relatedTarget as globalThis.Node)) return;
    }
    previewing = false;
    previewTargetId = null;
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

  // A GitHub block landing in a project whose folder is a clone gets its
  // repository from the clone's GitHub remote. Only an empty field is filled,
  // and only while the block still sits in that project, so a value typed
  // while the lookup was in flight wins. Lookup failures leave the field for
  // the user to fill in.
  async function loadRepositoryFromProject(id: string): Promise<void> {
    const node = nodeById(id);
    const project = node?.parentId ? nodeById(node.parentId) : undefined;
    if (node?.type !== "github" || node.repository || !project?.path) return;
    try {
      const response = await fetch(
        `/api/git/repository?path=${encodeURIComponent(project.path)}`,
      );
      if (!response.ok) return;
      const body = (await response.json()) as { repository?: unknown };
      const current = nodeById(id);
      if (
        typeof body.repository === "string" &&
        current &&
        !current.repository &&
        current.parentId === project.id
      )
        graph.setRepository(id, body.repository);
    } catch {
      // The field stays empty; a run then loads it from the project itself.
    }
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
    dropHint = "";
    clearPreview();
    graph.draggingType = null;
    if (isComponentType(data)) {
      const container = graph.containerAt(contentX, contentY);
      if (container && canHostChild(container.type, data)) {
        const origin = graph.absolutePosition(container);
        const node = graph.addNode(
          data,
          contentX - origin.x,
          contentY - origin.y,
          container.id,
        );
        void loadRepositoryFromProject(node.id);
        return;
      }
      // The containment matrix decides: a block whose only legal placement
      // is inside a container falls back to the top level when the canvas
      // root accepts it, and is rejected with an explanation otherwise.
      if (canExistTopLevel(data)) {
        graph.addNode(data, contentX, contentY);
        return;
      }
      dropHint = rejectedDropHint(data, container?.type);
      return;
    }
    if (data.startsWith("node:")) {
      const id = data.slice("node:".length);
      const node = nodeById(id);
      const grab = dragState;
      dragState = null;
      if (!node || !grab || grab.id !== id) return;
      // The dragged box never resolves as its own drop container.
      const container = graph.containerAt(contentX, contentY, id);
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
          void loadRepositoryFromProject(id);
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
      // Children live in their parent's coordinate space, and the parent may
      // itself be nested, so the pointer converts through the parent's
      // absolute origin.
      const parentOrigin = graph.absolutePosition(parent);
      if (container && container.id === parent.id) {
        graph.moveNode(
          id,
          contentX - grab.dx - parentOrigin.x,
          contentY - grab.dy - parentOrigin.y,
        );
      } else if (container && canHostChild(container.type, node.type)) {
        // Re-parenting to a compatible container keeps the block nested.
        graph.attachToContainer(
          id,
          container.id,
          contentX - grab.dx,
          contentY - grab.dy,
        );
        void loadRepositoryFromProject(id);
      } else if (container) {
        // An incompatible target container keeps the box in its own parent,
        // repositioned under the pointer.
        graph.moveNode(
          id,
          contentX - grab.dx - parentOrigin.x,
          contentY - grab.dy - parentOrigin.y,
        );
      } else if (canExistTopLevel(node.type)) {
        graph.detachNode(id, contentX - grab.dx, contentY - grab.dy);
      } else {
        // The matrix forbids this block at the root, so it stays inside its
        // own parent, repositioned under the pointer.
        graph.moveNode(
          id,
          contentX - grab.dx - parentOrigin.x,
          contentY - grab.dy - parentOrigin.y,
        );
      }
      return;
    }
  }
</script>

<div
  class="canvas"
  class:preview-invalid={previewing && !previewValid}
  role="region"
  aria-label="Graph canvas"
  ondragover={previewDrop}
  ondragleave={clearPreview}
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
        {@const fromPosition = graph.absolutePosition(from)}
        {@const toPosition = graph.absolutePosition(to)}
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
      class:drop-ok={previewing && previewTargetId === node.id && previewValid}
      class:drop-no={previewing && previewTargetId === node.id && !previewValid}
      aria-pressed={node.id === graph.selectedId}
      draggable="true"
      style:left="{graph.absolutePosition(node).x}px"
      style:top="{graph.absolutePosition(node).y}px"
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
        <span class="node-id" aria-hidden="true">{shortNodeId(node.id)}</span>
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
  {#if dropHint}
    <p class="drop-hint" role="status">{dropHint}</p>
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

  .drop-hint {
    position: absolute;
    left: 50%;
    bottom: 1rem;
    transform: translateX(-50%);
    margin: 0;
    padding: 0.4rem 0.75rem;
    border: 1px solid #e0b4b4;
    border-radius: 0.375rem;
    background: #fdf6f6;
    color: #a03030;
    font-size: 0.85rem;
    pointer-events: none;
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

  .node-id {
    margin-left: auto;
    align-self: center;
    font-size: 0.7rem;
    font-weight: 400;
    color: #7d968d;
    letter-spacing: 0.02em;
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

  .canvas.preview-invalid {
    cursor: not-allowed;
  }

  .node.drop-ok {
    border-color: #3f8f5f;
    box-shadow: 0 0 0 2px rgb(63 143 95 / 0.35);
  }

  .node.drop-no {
    border-color: #a03030;
    box-shadow: 0 0 0 2px rgb(160 48 48 / 0.35);
  }
</style>
