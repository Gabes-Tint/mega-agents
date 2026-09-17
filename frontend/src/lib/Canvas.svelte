<script lang="ts">
  import {
    BLOCK_HUES,
    defaultSize,
    GraphStore,
    labelFor,
    PALETTE,
    rejectedDropHint,
    shortNodeId,
    type GraphNode,
    type NodeType,
  } from "./graph.svelte.js";
  import { hideDragImage } from "./dragImage.js";

  let { graph }: { graph: GraphStore } = $props();

  // Transient explanation for a palette drop that was rejected by the
  // containment rules; cleared on the next drop so it cannot go stale.
  let dropHint = $state("");

  // Grab offset of the node being dragged, kept outside the template because
  // it is transient drag bookkeeping rather than rendered state.
  let dragState: { id: string; dx: number; dy: number } | null = null;

  // Live drop preview for the dragover handler: the container the dragged
  // block would land in, or the one refusing it, and whether it can land.
  // The palette component being dragged travels through graph.draggingType
  // because dataTransfer.getData is protected while a drag is in flight.
  let nestId = $state<string | null>(null);
  let refusedId = $state<string | null>(null);
  let previewValid = $state(false);
  let previewing = $state(false);
  // Where the dragged block would land, drawn in place of the browser's
  // drag image, and the block being moved, if any.
  let landing = $state<{
    type: NodeType;
    name: string;
    x: number;
    y: number;
    w: number;
    h: number;
  } | null>(null);
  let movingId = $state<string | null>(null);
  let resizing = $state(false);

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
    const where = graph.landingOf(type, contentX, contentY, dragState?.id);
    const valid = where.valid;
    previewing = true;
    nestId = where.parentId ?? null;
    refusedId = where.refusedBy ?? null;
    previewValid = valid;
    const moved = dragState ? nodeById(dragState.id) : undefined;
    landing = moved
      ? {
          type,
          name: moved.name,
          x: contentX - (dragState?.dx ?? 0),
          y: contentY - (dragState?.dy ?? 0),
          w: moved.w,
          h: moved.h,
        }
      : {
          type,
          name: labelFor(type),
          x: contentX,
          y: contentY,
          ...defaultSize(type),
        };
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
    nestId = null;
    refusedId = null;
    landing = null;
  }

  function endNodeDrag() {
    dragState = null;
    movingId = null;
    clearPreview();
  }

  function startNodeDrag(
    event: DragEvent & { currentTarget: EventTarget & HTMLButtonElement },
    node: GraphNode,
  ) {
    if (!event.dataTransfer) return;
    const rect = event.currentTarget.getBoundingClientRect();
    event.dataTransfer.setData("text/plain", `node:${node.id}`);
    event.dataTransfer.effectAllowed = "move";
    hideDragImage(event.dataTransfer);
    movingId = node.id;
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
    resizing = true;
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
    resizing = false;
  }

  function drop(
    event: DragEvent & { currentTarget: EventTarget & HTMLDivElement },
  ) {
    event.preventDefault();
    const target = event.currentTarget;
    const rect = target.getBoundingClientRect();
    // Node coordinates are content-space; pointer coordinates are viewport
    // space, so scrolled canvases need the scroll offset added back.
    const contentX = event.clientX - rect.left + target.scrollLeft;
    const contentY = event.clientY - rect.top + target.scrollTop;
    const data = event.dataTransfer?.getData("text/plain") ?? "";
    dropHint = "";
    clearPreview();
    graph.draggingType = null;
    if (isComponentType(data)) {
      const landing = graph.landingOf(data, contentX, contentY);
      if (!landing.valid) {
        const refusing = landing.refusedBy
          ? nodeById(landing.refusedBy)
          : undefined;
        dropHint = rejectedDropHint(data, refusing?.type);
        return;
      }
      const parent = landing.parentId ? nodeById(landing.parentId) : undefined;
      const origin = parent ? graph.absolutePosition(parent) : { x: 0, y: 0 };
      const node = graph.addNode(
        data,
        contentX - origin.x,
        contentY - origin.y,
        parent?.id,
      );
      graph.fitInParent(node.id);
      void loadRepositoryFromProject(node.id);
      return;
    }
    if (!data.startsWith("node:")) return;
    const id = data.slice("node:".length);
    const node = nodeById(id);
    const grab = dragState;
    dragState = null;
    movingId = null;
    if (!node || !grab || grab.id !== id) return;
    const x = contentX - grab.dx;
    const y = contentY - grab.dy;
    const landing = graph.landingOf(node.type, contentX, contentY, id);
    const parent = landing.parentId ? nodeById(landing.parentId) : undefined;
    if (!parent) {
      if (node.parentId === undefined) graph.moveNode(id, x, y);
      else graph.detachNode(id, x, y);
      return;
    }
    if (parent.id === node.parentId) {
      // Children live in their parent's coordinate space, and the parent may
      // itself be nested, so the pointer converts through the parent's
      // absolute origin.
      const origin = graph.absolutePosition(parent);
      graph.moveNode(id, x - origin.x, y - origin.y);
    } else {
      graph.attachToContainer(id, parent.id, x, y);
      void loadRepositoryFromProject(id);
    }
    graph.fitInParent(id);
  }
</script>

<div
  class="canvas"
  class:preview-invalid={previewing && !previewValid}
  class:resizing
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
        <path class="edge-arrow" d="M0 0 L8 4 L0 8 Z" />
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
        {#if graph.portOf(edge)}
          <text
            class="edge-port"
            x={(start.x + end.x) / 2}
            y={(start.y + end.y) / 2 - 4}
            text-anchor="middle">{graph.portOf(edge)}</text
          >
        {/if}
      {/if}
    {/each}
  </svg>
  {#each graph.nodes as node (node.id)}
    <button
      type="button"
      class="node block-{node.type} status-{graph.statusOf(node.id) ?? 'idle'}"
      class:selected={node.id === graph.selectedId}
      class:problem-error={graph.severityOf(node.id) === "error"}
      class:problem-warning={graph.severityOf(node.id) === "warning"}
      class:drop-ok={previewing && nestId === node.id}
      class:drop-no={previewing && refusedId === node.id}
      class:dragging={node.id === movingId}
      aria-pressed={node.id === graph.selectedId}
      draggable="true"
      style:--block-hue={BLOCK_HUES[node.type]}
      style:left="{graph.absolutePosition(node).x}px"
      style:top="{graph.absolutePosition(node).y}px"
      style:width="{node.w}px"
      style:height="{node.h}px"
      ondragstart={(event) => startNodeDrag(event, node)}
      ondragend={endNodeDrag}
      onclick={() => selectOrConnect(node)}
      onkeydown={(event) => {
        // Delete or Backspace deletes the focused block.
        if (event.key !== "Delete" && event.key !== "Backspace") return;
        event.preventDefault();
        graph.removeNode(node.id);
      }}
    >
      <span class="node-title">
        {node.name}
        {#if node.start}
          <span class="start-flag" aria-hidden="true">▶</span>
        {/if}
        {#if node.type === "loop"}
          <span class="repeats" aria-hidden="true">
            ↻
            {#if graph.iterationOf(node.id)}
              {graph.iterationOf(node.id)} of {node.maxIterations ?? 3}
            {:else}
              up to {node.maxIterations ?? 3}×
            {/if}
          </span>
        {/if}
        {#if graph.severityOf(node.id)}
          <span
            class="problem-mark"
            title={graph.severityOf(node.id) === "error"
              ? "Has errors to fix"
              : "Has warnings"}
            aria-hidden="true"
          ></span>
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
  {#if landing}
    <div
      class="drop-preview block-{landing.type}"
      class:invalid={!previewValid}
      class:nesting={nestId !== null}
      aria-hidden="true"
      style:--block-hue={BLOCK_HUES[landing.type]}
      style:left="{landing.x}px"
      style:top="{landing.y}px"
      style:width="{landing.w}px"
      style:height="{landing.h}px"
    >
      <span class="node-title">{landing.name}</span>
    </div>
  {/if}
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
      radial-gradient(var(--canvas-dot) 1px, transparent 1px) 0 0 / 20px 20px,
      var(--canvas);
  }

  /* Above the blocks: arrows between blocks nested in a container would
     otherwise be painted over by the container's own background. The layer
     ignores the pointer, so the blocks under it stay clickable. */
  .edges {
    position: absolute;
    left: 0;
    top: 0;
    z-index: 2;
    pointer-events: none;
    overflow: visible;
  }

  .edge-line {
    stroke: var(--edge);
    stroke-width: 1.5;
  }

  .edge-arrow {
    fill: var(--edge);
  }

  .edge-port {
    fill: var(--text-muted);
    font-size: 11px;
    font-weight: 500;
    paint-order: stroke;
    stroke: var(--canvas);
    stroke-width: 4px;
    stroke-linejoin: round;
  }

  .hint {
    position: absolute;
    inset: 0;
    display: grid;
    place-content: center;
    color: var(--text-faint);
    pointer-events: none;
    margin: 0;
  }

  .drop-hint {
    position: sticky;
    left: 50%;
    bottom: 1rem;
    z-index: 3;
    width: fit-content;
    transform: translateX(-50%);
    margin: 0;
    padding: 0.4rem 0.75rem;
    border: 1px solid var(--fail);
    border-radius: var(--radius);
    background: var(--fail-soft);
    color: var(--fail);
    pointer-events: none;
    box-shadow: var(--shadow);
  }

  .node {
    position: absolute;
    display: flex;
    flex-direction: column;
    align-items: stretch;
    justify-content: flex-start;
    gap: 0;
    padding: 0;
    border: 1px solid var(--block-border);
    border-radius: var(--radius-lg);
    background: var(--block-body);
    color: var(--text);
    font: inherit;
    text-align: left;
    overflow: hidden;
    cursor: pointer;
    box-shadow: var(--shadow);
  }

  .node:hover:not(:disabled) {
    background: var(--block-body);
    border-color: var(--block-accent);
  }

  .node-title {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.4rem 0.65rem;
    font-weight: 600;
    background: var(--block-soft);
    white-space: nowrap;
  }

  .node-title::before {
    content: "";
    flex: none;
    width: 0.55rem;
    height: 0.55rem;
    border-radius: 3px;
    background: var(--block-accent);
  }

  .node-separator {
    display: block;
    height: 1px;
    background: var(--block-border);
  }

  .start-flag {
    color: var(--ok);
    font-size: 10px;
  }

  .node-id {
    margin-left: auto;
    align-self: center;
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: 400;
    color: var(--text-faint);
    letter-spacing: 0.02em;
  }

  .repeats {
    padding: 0 0.4rem;
    border-radius: 999px;
    background: var(--block-body);
    color: var(--block-accent);
    font-size: 11px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  .problem-mark {
    margin-left: auto;
    width: 0.5rem;
    height: 0.5rem;
    border-radius: 50%;
  }

  .problem-mark + .node-id {
    margin-left: 0;
  }

  .problem-error .problem-mark {
    background: var(--fail);
    box-shadow: 0 0 0 3px var(--fail-soft);
  }

  .problem-warning .problem-mark {
    background: var(--warn);
    box-shadow: 0 0 0 3px var(--warn-soft);
  }

  .resize-handle {
    position: absolute;
    right: 0;
    bottom: 0;
    width: 14px;
    height: 14px;
    touch-action: none;
    cursor: nwse-resize;
    background: linear-gradient(
      135deg,
      transparent 0 45%,
      var(--border-strong) 45% 55%,
      transparent 55% 70%,
      var(--border-strong) 70% 80%,
      transparent 80%
    );
  }

  .node.selected {
    border-color: var(--accent);
    box-shadow:
      0 0 0 3px var(--focus),
      var(--shadow);
  }

  /* The landing spot of a dragged block: its outline and header, see-through
     so the blocks under it stay readable. */
  .drop-preview {
    box-sizing: border-box;
    position: absolute;
    z-index: 3;
    display: flex;
    flex-direction: column;
    border: 1.5px dashed var(--block-accent);
    border-radius: var(--radius-lg);
    background: color-mix(in oklch, var(--block-body) 55%, transparent);
    box-shadow: var(--shadow);
    pointer-events: none;
    overflow: hidden;
    opacity: 0.9;
  }

  .drop-preview .node-title {
    background: color-mix(in oklch, var(--block-soft) 80%, transparent);
  }

  .drop-preview.invalid {
    border-color: var(--fail);
    background: color-mix(in oklch, var(--fail-soft) 50%, transparent);
  }

  .node.dragging {
    opacity: 0.4;
  }

  .canvas.preview-invalid {
    cursor: not-allowed;
  }

  .node.status-running {
    border-color: var(--info);
    box-shadow: 0 0 0 3px var(--info-soft);
  }

  .node.status-succeeded {
    border-color: var(--ok);
    box-shadow: 0 0 0 3px var(--ok-soft);
  }

  .node.status-failed {
    border-color: var(--fail);
    box-shadow: 0 0 0 3px var(--fail-soft);
  }

  .node.status-skipped {
    opacity: 0.55;
  }

  /* The container a dragged block will land in, and the preview of that
     block, share one highlight so the pairing reads at a glance. */
  .node.drop-ok,
  .drop-preview.nesting {
    border-color: var(--ok);
    box-shadow:
      0 0 0 3px var(--ok-soft),
      var(--shadow);
  }

  .node.drop-ok {
    background: color-mix(in oklch, var(--ok-soft) 45%, var(--block-body));
  }

  .drop-preview.nesting {
    border-style: solid;
  }

  /* A container growing to fit a new block eases into its size; a resize
     follows the pointer directly. */
  .node {
    transition:
      width 140ms ease-out,
      height 140ms ease-out;
  }

  .canvas.resizing .node {
    transition: none;
  }

  @media (prefers-reduced-motion: reduce) {
    .node {
      transition: none;
    }
  }

  .node.drop-no {
    border-color: var(--fail);
    box-shadow: 0 0 0 3px var(--fail-soft);
  }
</style>
