<script lang="ts">
  import {
    BLOCK_HUES,
    defaultSize,
    GraphStore,
    type ResizeEdges,
    labelFor,
    PALETTE,
    rejectedDropHint,
    shortNodeId,
    type GraphNode,
    type NodeType,
  } from "./graph.svelte.js";
  import { hideDragImage } from "./dragImage.js";
  import BlockIcon from "./BlockIcon.svelte";

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

  // Starting pointer position, box and dragged edges of an active resize.
  let resizeState: {
    id: string;
    startX: number;
    startY: number;
    box: { x: number; y: number; w: number; h: number };
    edges: ResizeEdges;
  } | null = null;

  // Every edge and corner a block resizes from; the bottom right corner is
  // the visible grip.
  const RESIZE_EDGES = [
    "top",
    "right",
    "bottom",
    "left",
    "top left",
    "top right",
    "bottom left",
  ] as const;

  // The sides a block shows a handle on for drawing an arrow from it.
  const SIDES = ["top", "right", "bottom", "left"] as const;
  const HANDLE_SIZE = 20;

  // An arrow being drawn from a block's handle, or an arrow whose end is
  // being dragged: the block at the fixed end, which end follows the pointer
  // (the head of a new arrow), and the block under the pointer, if any, with
  // whether the arrow may end there.
  let linking = $state<{
    anchorId: string;
    end: "from" | "to";
    edgeId?: string;
    x: number;
    y: number;
    targetId?: string;
    valid: boolean;
  } | null>(null);
  let hoverId = $state<string | null>(null);
  // The arrow under the pointer shows its ends like the selected one. They
  // hide a moment after the pointer leaves, so it can reach an end.
  let hoverEdgeId = $state<string | null>(null);
  let hideEnds: ReturnType<typeof setTimeout> | undefined;

  function showEnds(id: string) {
    globalThis.clearTimeout(hideEnds);
    hoverEdgeId = id;
  }

  function leaveEnds() {
    hideEnds = setTimeout(() => (hoverEdgeId = null), 300);
  }

  // The outputs to choose from when a new arrow leaves a block with several,
  // shown where the pointer was released.
  let portMenu = $state<{
    x: number;
    y: number;
    fromId: string;
    toId: string;
    edgeId?: string;
    ports: string[];
  } | null>(null);
  let canvasElement: HTMLDivElement;
  let menuElement = $state<HTMLDivElement>();

  // Handles show on the hovered and the selected block, except while a block
  // is dragged or resized, an arrow is drawn or its output is chosen.
  const handleIds = $derived(
    linking || portMenu || movingId || resizing
      ? []
      : [...new Set([graph.selectedId, hoverId])].filter(
          (id): id is string => id !== null,
        ),
  );

  $effect(() => {
    if (portMenu) menuElement?.querySelector("button")?.focus();
  });

  function edgesOf(edge: string): ResizeEdges {
    const sides = edge.split(" ");
    return {
      top: sides.includes("top"),
      right: sides.includes("right"),
      bottom: sides.includes("bottom"),
      left: sides.includes("left"),
    };
  }

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
    return Number.isFinite(t) ? { x: x + dx * t, y: y + dy * t } : { x, y };
  }

  function borderToward(node: GraphNode, x: number, y: number) {
    const origin = graph.absolutePosition(node);
    return borderPoint(
      node,
      origin.x + node.w / 2,
      origin.y + node.h / 2,
      x,
      y,
    );
  }

  // Top left corner of a handle, just outside the middle of a block's side.
  function handleAt(node: GraphNode, side: (typeof SIDES)[number]) {
    const { x, y } = graph.absolutePosition(node);
    const middleX = x + (node.w - HANDLE_SIZE) / 2;
    const middleY = y + (node.h - HANDLE_SIZE) / 2;
    return {
      top: { x: middleX, y: y - HANDLE_SIZE },
      right: { x: x + node.w, y: middleY },
      bottom: { x: middleX, y: y + node.h },
      left: { x: x - HANDLE_SIZE, y: middleY },
    }[side];
  }

  // Pulls a line's ends in, so an arrow's click target leaves the resize
  // strips of the blocks it joins alone.
  function inset(
    start: { x: number; y: number },
    end: { x: number; y: number },
  ) {
    const length = Math.hypot(end.x - start.x, end.y - start.y);
    const k = length > 20 ? 8 / length : 0;
    const dx = (end.x - start.x) * k;
    const dy = (end.y - start.y) * k;
    return {
      x1: start.x + dx,
      y1: start.y + dy,
      x2: end.x - dx,
      y2: end.y - dy,
    };
  }

  function contentPoint(event: { clientX: number; clientY: number }) {
    const rect = canvasElement.getBoundingClientRect();
    return {
      x: event.clientX - rect.left + canvasElement.scrollLeft,
      y: event.clientY - rect.top + canvasElement.scrollTop,
    };
  }

  function hover(event: PointerEvent) {
    if ((event.target as HTMLElement).closest(".link-handle")) return;
    const point = contentPoint(event);
    hoverId = graph.nodeAt(point.x, point.y)?.id ?? null;
  }

  // The arrow's source and target when its moving end lands on the block.
  function pairWith(
    active: NonNullable<typeof linking>,
    id: string,
  ): [string, string] {
    return active.end === "to" ? [active.anchorId, id] : [id, active.anchorId];
  }

  function startLink(
    event: PointerEvent & { currentTarget: EventTarget & globalThis.Element },
    anchorId: string,
    end: "from" | "to",
    edgeId?: string,
  ) {
    if (event.button !== 0) return;
    event.preventDefault();
    event.stopPropagation();
    event.currentTarget.setPointerCapture?.(event.pointerId);
    portMenu = null;
    linking = { anchorId, end, edgeId, ...contentPoint(event), valid: false };
  }

  function moveLink(event: PointerEvent) {
    if (!linking) return;
    const point = contentPoint(event);
    const target = graph.nodeAt(point.x, point.y);
    linking = {
      ...linking,
      ...point,
      targetId: target?.id,
      valid:
        target !== undefined &&
        graph.canConnect(...pairWith(linking, target.id), linking.edgeId),
    };
  }

  // Draws or moves the arrow onto a block that may take it. A new source
  // with several outputs first asks which one the arrow takes.
  function endLink() {
    const active = linking;
    linking = null;
    if (!active?.targetId || !active.valid) return;
    const [fromId, toId] = pairWith(active, active.targetId);
    const edge = graph.edges.find(
      (candidate) => candidate.id === active.edgeId,
    );
    const ports = graph.outputPorts(fromId);
    if (ports.length > 1 && edge?.from !== fromId) {
      portMenu = {
        x: active.x,
        y: active.y,
        fromId,
        toId,
        edgeId: edge?.id,
        ports,
      };
      return;
    }
    if (!edge) graph.connect(fromId, toId);
    else graph.reconnectEdge(edge.id, active.end, active.targetId);
  }

  function choosePort(port: string) {
    const menu = portMenu;
    portMenu = null;
    if (!menu) return;
    if (menu.edgeId)
      graph.reconnectEdge(menu.edgeId, "from", menu.fromId, port);
    else graph.connect(menu.fromId, menu.toId, port);
  }

  // Up and down move through the outputs, Enter or Space takes one, and Tab
  // leaves the menu; Escape closes it from anywhere.
  function menuKey(event: { key: string; preventDefault(): void }) {
    const items = [...(menuElement?.querySelectorAll("button") ?? [])];
    const index = items.findIndex((item) => item === document.activeElement);
    const step = { ArrowDown: 1, ArrowUp: items.length - 1 }[event.key];
    if (step !== undefined) items[(index + step) % items.length]?.focus();
    else if (event.key === "Enter" || event.key === " ") {
      const port = portMenu?.ports[index];
      if (port !== undefined) choosePort(port);
    } else if (event.key === "Tab") portMenu = null;
    else return;
    event.preventDefault();
  }

  function cancelLink(event: { key: string }) {
    if (event.key !== "Escape") return;
    linking = null;
    portMenu = null;
  }

  function closeMenu(event: PointerEvent) {
    if (portMenu && !menuElement?.contains(event.target as globalThis.Node))
      portMenu = null;
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
    edge: string,
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
      box: { x: node.x, y: node.y, w: node.w, h: node.h },
      edges: edgesOf(edge),
    };
  }

  function applyResize(
    event: PointerEvent & { currentTarget: EventTarget & Window },
  ) {
    const active = resizeState;
    if (!active) return;
    graph.resizeFrom(
      active.id,
      active.box,
      active.edges,
      event.clientX - active.startX,
      event.clientY - active.startY,
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
    if (resizeState) graph.fitInParent(resizeState.id);
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
  class:linking
  role="region"
  aria-label="Graph canvas"
  bind:this={canvasElement}
  onpointermove={hover}
  onpointerleave={() => (hoverId = null)}
  ondragover={previewDrop}
  ondragleave={clearPreview}
  ondrop={drop}
>
  <svg
    class="edges"
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
      <marker
        id="edge-arrowhead-active"
        markerWidth="8"
        markerHeight="8"
        refX="7"
        refY="4"
        orient="auto"
      >
        <path class="edge-arrow-active" d="M0 0 L8 4 L0 8 Z" />
      </marker>
      <marker
        id="edge-arrowhead-invalid"
        markerWidth="8"
        markerHeight="8"
        refX="7"
        refY="4"
        orient="auto"
      >
        <path class="edge-arrow-invalid" d="M0 0 L8 4 L0 8 Z" />
      </marker>
    </defs>
    <g role="listbox" aria-label="Arrows">
      {#each graph.edges as edge (edge.id)}
        {@const from = nodeById(edge.from)}
        {@const to = nodeById(edge.to)}
        {#if from && to}
          {@const fromPosition = graph.absolutePosition(from)}
          {@const toPosition = graph.absolutePosition(to)}
          {@const start = borderToward(
            from,
            toPosition.x + to.w / 2,
            toPosition.y + to.h / 2,
          )}
          {@const end = borderToward(
            to,
            fromPosition.x + from.w / 2,
            fromPosition.y + from.h / 2,
          )}
          {@const selected = edge.id === graph.selectedEdgeId}
          <line
            class="edge-hit"
            role="option"
            tabindex="0"
            aria-selected={selected}
            aria-label="Arrow from {from.name} to {to.name}"
            {...inset(start, end)}
            onpointerenter={() => showEnds(edge.id)}
            onpointerleave={leaveEnds}
            onclick={(event) => {
              graph.selectEdge(edge.id);
              event.currentTarget.focus();
            }}
            onkeydown={(event) => {
              // Delete or Backspace deletes the focused arrow.
              if (event.key !== "Delete" && event.key !== "Backspace") return;
              event.preventDefault();
              graph.removeEdge(edge.id);
            }}
          ></line>
          <line
            class="edge-line"
            class:selected
            class:moving={linking?.edgeId === edge.id}
            x1={start.x}
            y1={start.y}
            x2={end.x}
            y2={end.y}
            marker-end={selected
              ? "url(#edge-arrowhead-active)"
              : "url(#edge-arrowhead)"}
          ></line>
          {#if graph.portOf(edge)}
            <text
              class="edge-port"
              aria-hidden="true"
              x={(start.x + end.x) / 2}
              y={(start.y + end.y) / 2 - 4}
              text-anchor="middle">{graph.portOf(edge)}</text
            >
          {/if}
          {#if (selected || hoverEdgeId === edge.id) && !linking}
            <circle
              class="edge-end"
              data-end="from"
              aria-hidden="true"
              onpointerenter={() => showEnds(edge.id)}
              onpointerleave={leaveEnds}
              cx={start.x}
              cy={start.y}
              r="5"
              onpointerdown={(event) =>
                startLink(event, edge.to, "from", edge.id)}
            ></circle>
            <circle
              class="edge-end"
              data-end="to"
              aria-hidden="true"
              onpointerenter={() => showEnds(edge.id)}
              onpointerleave={leaveEnds}
              cx={end.x}
              cy={end.y}
              r="5"
              onpointerdown={(event) =>
                startLink(event, edge.from, "to", edge.id)}
            ></circle>
          {/if}
        {/if}
      {/each}
    </g>
    {#if linking}
      {@const anchor = nodeById(linking.anchorId)}
      {#if anchor}
        {@const border = borderToward(anchor, linking.x, linking.y)}
        {@const tail = linking.end === "to" ? border : linking}
        {@const head = linking.end === "to" ? linking : border}
        {@const refused = linking.targetId !== undefined && !linking.valid}
        <line
          class="edge-preview"
          class:invalid={refused}
          x1={tail.x}
          y1={tail.y}
          x2={head.x}
          y2={head.y}
          marker-end="url(#edge-arrowhead-{refused ? 'invalid' : 'active'})"
        ></line>
      {/if}
    {/if}
  </svg>
  {#each graph.nodes as node (node.id)}
    <button
      type="button"
      class="node block-{node.type} status-{graph.statusOf(node.id) ?? 'idle'}"
      class:selected={node.id === graph.selectedId}
      class:problem-error={graph.severityOf(node.id) === "error"}
      class:problem-warning={graph.severityOf(node.id) === "warning"}
      class:drop-ok={(previewing && nestId === node.id) ||
        (linking?.targetId === node.id && linking.valid)}
      class:drop-no={(previewing && refusedId === node.id) ||
        (linking?.targetId === node.id && !linking.valid)}
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
        <span class="node-meta">
          <span class="node-id" aria-hidden="true">{shortNodeId(node.id)}</span>
          <BlockIcon {node} />
        </span>
      </span>
      <span class="node-separator" aria-hidden="true"></span>
      {#each RESIZE_EDGES as edge (edge)}
        <span
          class="resize-edge"
          data-edge={edge}
          aria-hidden="true"
          onpointerdown={(event) => startResize(event, node, edge)}
        ></span>
      {/each}
      <span
        class="resize-handle"
        data-edge="bottom right"
        aria-hidden="true"
        onpointerdown={(event) => startResize(event, node, "bottom right")}
      ></span>
    </button>
  {/each}
  {#each handleIds as id (id)}
    {@const node = nodeById(id)}
    {#if node}
      {#each SIDES as side (side)}
        {@const at = handleAt(node, side)}
        <span
          class="link-handle"
          data-side={side}
          aria-hidden="true"
          title="Drag to draw an arrow"
          style:left="{at.x}px"
          style:top="{at.y}px"
          onpointerdown={(event) => startLink(event, node.id, "to")}
        ></span>
      {/each}
    {/if}
  {/each}
  {#if portMenu}
    <div
      class="port-menu"
      role="menu"
      aria-label="Output"
      tabindex="-1"
      bind:this={menuElement}
      style:left="{portMenu.x}px"
      style:top="{portMenu.y}px"
      onkeydown={menuKey}
    >
      <span class="port-menu-title" aria-hidden="true">Output</span>
      {#each portMenu.ports as port, index (index)}
        <button type="button" role="menuitem" onclick={() => choosePort(port)}
          >{port}</button
        >
      {/each}
    </div>
  {/if}
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
  onpointermove={(event) => {
    applyResize(event);
    moveLink(event);
  }}
  onpointerup={() => {
    endResize();
    endLink();
  }}
  onpointercancel={() => {
    endResize();
    linking = null;
  }}
  onpointerdown={closeMenu}
  onkeydown={cancelLink}
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

  /* A wide invisible stroke under each arrow takes its clicks, the only
     part of the layer that does. */
  .edge-hit {
    stroke: transparent;
    stroke-width: 12;
    pointer-events: stroke;
    cursor: pointer;
    outline: none;
  }

  .edge-hit:hover + .edge-line {
    stroke: var(--text-muted);
  }

  .edge-line.selected,
  .edge-hit:focus-visible + .edge-line {
    stroke: var(--accent);
    stroke-width: 2.25;
  }

  .edge-line.moving {
    opacity: 0.3;
  }

  .edge-arrow-active {
    fill: var(--accent);
  }

  .edge-preview {
    stroke: var(--accent);
    stroke-width: 2;
    stroke-dasharray: 6 4;
  }

  .edge-preview.invalid {
    stroke: var(--fail);
  }

  .edge-arrow-invalid {
    fill: var(--fail);
  }

  /* The ends of the selected arrow, dragged onto another block to move it. */
  .edge-end {
    fill: var(--surface);
    stroke: var(--accent);
    stroke-width: 2;
    pointer-events: all;
    cursor: move;
    touch-action: none;
  }

  .edge-end:hover {
    fill: var(--accent);
  }

  /* draw.io style arrows just outside each side of a block; pressing one
     and dragging onto another block draws an arrow to it. */
  .link-handle {
    position: absolute;
    z-index: 4;
    width: 20px;
    height: 20px;
    cursor: crosshair;
    touch-action: none;
  }

  .link-handle::before {
    content: "";
    position: absolute;
    inset: 4px;
    background: var(--accent);
    clip-path: polygon(50% 6%, 100% 94%, 0 94%);
    opacity: 0.6;
    transform: rotate(var(--turn, 0deg));
    transition:
      opacity 120ms ease,
      transform 120ms ease;
  }

  .link-handle[data-side="right"] {
    --turn: 90deg;
  }

  .link-handle[data-side="bottom"] {
    --turn: 180deg;
  }

  .link-handle[data-side="left"] {
    --turn: 270deg;
  }

  .link-handle:hover::before {
    opacity: 1;
    transform: rotate(var(--turn, 0deg)) scale(1.2);
  }

  .canvas.linking,
  .canvas.linking .node {
    cursor: crosshair;
  }

  .port-menu {
    position: absolute;
    z-index: 5;
    display: grid;
    min-width: 8rem;
    padding: 0.25rem;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    background: var(--surface);
    box-shadow: var(--shadow-lifted);
  }

  .port-menu-title {
    padding: 0.15rem 0.5rem 0.25rem;
    font-size: 11px;
    font-weight: 600;
    color: var(--text-faint);
  }

  .port-menu button {
    justify-content: flex-start;
    min-height: 1.7rem;
    border: 0;
    background: transparent;
  }

  .port-menu button:hover,
  .port-menu button:focus-visible {
    background: var(--surface-hover);
    outline: none;
  }

  .port-menu button:focus-visible {
    box-shadow: inset 0 0 0 2px var(--focus);
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
    min-height: 2.25rem;
    box-sizing: border-box;
    padding: 0.2rem 0.4rem 0.2rem 0.65rem;
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

  /* The block's id over its type icon, at the header's right edge. */
  .node-meta {
    margin-left: auto;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1px;
  }

  .node-id {
    line-height: 1;
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

  .problem-mark + .node-meta {
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

  /* Invisible strips along each edge and squares at each corner, under the
     pointer's resize cursors. */
  .resize-edge {
    position: absolute;
    z-index: 1;
    touch-action: none;
  }

  .resize-edge[data-edge="top"],
  .resize-edge[data-edge="bottom"] {
    left: 8px;
    right: 8px;
    height: 6px;
    cursor: ns-resize;
  }

  .resize-edge[data-edge="left"],
  .resize-edge[data-edge="right"] {
    top: 8px;
    bottom: 8px;
    width: 6px;
    cursor: ew-resize;
  }

  .resize-edge[data-edge="top"],
  .resize-edge[data-edge^="top "] {
    top: 0;
  }

  .resize-edge[data-edge="bottom"],
  .resize-edge[data-edge^="bottom "] {
    bottom: 0;
  }

  .resize-edge[data-edge="left"],
  .resize-edge[data-edge$=" left"] {
    left: 0;
  }

  .resize-edge[data-edge="right"],
  .resize-edge[data-edge$=" right"] {
    right: 0;
  }

  .resize-edge[data-edge*=" "] {
    width: 10px;
    height: 10px;
  }

  .resize-edge[data-edge="top left"] {
    cursor: nwse-resize;
  }

  .resize-edge[data-edge="top right"],
  .resize-edge[data-edge="bottom left"] {
    cursor: nesw-resize;
  }

  .resize-handle {
    position: absolute;
    z-index: 1;
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
  .canvas .node.drop-ok,
  .drop-preview.nesting {
    border-color: var(--ok);
    box-shadow:
      0 0 0 3px var(--ok-soft),
      var(--shadow);
  }

  .canvas .node.drop-ok {
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

  .canvas .node.drop-no {
    border-color: var(--fail);
    box-shadow: 0 0 0 3px var(--fail-soft);
  }
</style>
