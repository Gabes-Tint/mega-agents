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
  import NodeMark from "./NodeMark.svelte";
  import { endingNodeIds, statusMark } from "./nodeMarks.js";
  import { arrowScope, routeArrow, type Box } from "./routing.js";
  import {
    clampZoom,
    fitToContent,
    scrollForZoom,
    wheelZoom,
    ZOOM_DEFAULT,
    zoomIn,
    zoomOut,
    zoomPercent,
  } from "./canvasZoom.js";
  import { loadZoom, saveZoom } from "./panelLayout.js";
  import { tick } from "svelte";

  let { graph }: { graph: GraphStore } = $props();

  // How large the content is drawn, remembered between visits. Blocks and
  // arrows keep their content coordinates: only the layer they sit in is
  // scaled, and every pointer position converts through this one number.
  let zoom = $state(loadZoom());

  // Where a run of the flow as it stands could finish. Arrows change it as
  // they are drawn and removed, so it is derived rather than held on a node.
  const endingIds = $derived(endingNodeIds(graph.nodes, graph.edges));

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

  // Every block as the arrow router sees it, in canvas content space. The
  // list is rebuilt whenever a block moves, and each arrow takes from it
  // the blocks that stand in its own way.
  const levels = $derived(
    graph.nodes.map((node) => ({
      id: node.id,
      parentId: node.parentId,
      box: boxOf(node),
    })),
  );

  function boxOf(node: GraphNode): Box {
    const origin = graph.absolutePosition(node);
    return { x: origin.x, y: origin.y, w: node.w, h: node.h };
  }

  // The way an arrow runs: orthogonal segments with rounded corners that go
  // around the blocks drawn at its own level.
  function routeOf(from: GraphNode, to: GraphNode) {
    return routeArrow(
      boxOf(from),
      boxOf(to),
      arrowScope(levels, from.id, to.id),
    );
  }

  // The arrow being drawn or moved, routed the same way, with the pointer
  // standing in for the end that has not landed on a block yet.
  function previewRoute(
    active: NonNullable<typeof linking>,
    anchor: GraphNode,
  ) {
    const pointer: Box = { x: active.x, y: active.y, w: 0, h: 0 };
    const scope = arrowScope(levels, anchor.id, active.targetId);
    return active.end === "to"
      ? routeArrow(boxOf(anchor), pointer, scope)
      : routeArrow(pointer, boxOf(anchor), scope);
  }

  // Top left corner of a handle, just outside the middle of a block's side.
  // Handles keep their size on screen at every zoom, so the box they take up
  // in content space is the screen size converted through the zoom.
  function handleAt(node: GraphNode, side: (typeof SIDES)[number]) {
    const { x, y } = graph.absolutePosition(node);
    const size = HANDLE_SIZE / zoom;
    const middleX = x + (node.w - size) / 2;
    const middleY = y + (node.h - size) / 2;
    return {
      top: { x: middleX, y: y - size },
      right: { x: x + node.w, y: middleY },
      bottom: { x: middleX, y: y + node.h },
      left: { x: x - size, y: middleY },
    }[side];
  }

  // The one conversion from screen pixels to canvas content units: every
  // pointer position and every distance a gesture covers goes through it, so
  // zooming needs no scale factor anywhere else.
  function toContent(x: number, y: number) {
    return { x: x / zoom, y: y / zoom };
  }

  function contentPoint(event: { clientX: number; clientY: number }) {
    const rect = canvasElement.getBoundingClientRect();
    return toContent(
      event.clientX - rect.left + canvasElement.scrollLeft,
      event.clientY - rect.top + canvasElement.scrollTop,
    );
  }

  // The box around every block, which fit to content frames.
  function contentBounds(): Box {
    let left = Infinity;
    let top = Infinity;
    let right = -Infinity;
    let bottom = -Infinity;
    for (const node of graph.nodes) {
      const at = graph.absolutePosition(node);
      left = Math.min(left, at.x);
      top = Math.min(top, at.y);
      right = Math.max(right, at.x + node.w);
      bottom = Math.max(bottom, at.y + node.h);
    }
    if (left > right) return { x: 0, y: 0, w: 0, h: 0 };
    return { x: left, y: top, w: right - left, h: bottom - top };
  }

  // Zooms to the level, leaving the content under the anchor where it is.
  // The anchor is measured from the canvas's top left corner in screen
  // pixels, and defaults to the middle of what is on screen.
  function setZoom(next: number, anchor?: { x: number; y: number }): void {
    const level = clampZoom(next);
    if (level === zoom) return;
    const view = canvasElement;
    const scroll = scrollForZoom(
      { left: view.scrollLeft, top: view.scrollTop },
      anchor ?? { x: view.clientWidth / 2, y: view.clientHeight / 2 },
      zoom,
      level,
    );
    applyZoom(level, scroll);
  }

  // Frames the whole flow, however far it spreads.
  function fitContent(): void {
    const view = canvasElement;
    const fit = fitToContent(contentBounds(), {
      width: view.clientWidth,
      height: view.clientHeight,
    });
    applyZoom(fit.zoom, fit);
  }

  // The scrollable area only takes its new size once the layer is redrawn,
  // so the canvas scrolls after that update rather than against the old one.
  function applyZoom(
    level: number,
    scroll: { left: number; top: number },
  ): void {
    zoom = level;
    saveZoom(level);
    void tick().then(() => {
      canvasElement.scrollLeft = scroll.left;
      canvasElement.scrollTop = scroll.top;
    });
  }

  // Ctrl or Cmd with the wheel, which is also how a trackpad pinch arrives,
  // zooms toward the pointer instead of scrolling the canvas.
  function zoomWheel(event: globalThis.WheelEvent): void {
    if (!event.ctrlKey && !event.metaKey) return;
    event.preventDefault();
    const rect = canvasElement.getBoundingClientRect();
    setZoom(wheelZoom(zoom, event.deltaY), {
      x: event.clientX - rect.left,
      y: event.clientY - rect.top,
    });
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

  // Escape drops what is being drawn; Ctrl or Cmd with plus, minus and zero
  // zoom and reset, in place of the browser's own page zoom.
  function canvasKey(event: globalThis.KeyboardEvent) {
    if (event.key === "Escape") {
      linking = null;
      portMenu = null;
      return;
    }
    if (!event.ctrlKey && !event.metaKey) return;
    const next =
      event.key === "0"
        ? ZOOM_DEFAULT
        : event.key === "-"
          ? zoomOut(zoom)
          : event.key === "+" || event.key === "="
            ? zoomIn(zoom)
            : undefined;
    if (next === undefined) return;
    event.preventDefault();
    setZoom(next);
  }

  function closeMenu(event: PointerEvent) {
    if (portMenu && !menuElement?.contains(event.target as globalThis.Node))
      portMenu = null;
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
    const { x: contentX, y: contentY } = contentPoint(event);
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
    // The block is drawn at the current zoom, so the grab point is a screen
    // distance into it and converts to content units like every other one.
    const rect = event.currentTarget.getBoundingClientRect();
    const grab = toContent(event.clientX - rect.left, event.clientY - rect.top);
    event.dataTransfer.setData("text/plain", `node:${node.id}`);
    event.dataTransfer.effectAllowed = "move";
    hideDragImage(event.dataTransfer);
    movingId = node.id;
    dragState = { id: node.id, dx: grab.x, dy: grab.y };
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
    const travelled = toContent(
      event.clientX - active.startX,
      event.clientY - active.startY,
    );
    graph.resizeFrom(
      active.id,
      active.box,
      active.edges,
      travelled.x,
      travelled.y,
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
    // Node coordinates are content-space; pointer coordinates are viewport
    // space, so the conversion adds back the scroll offset and the zoom.
    const { x: contentX, y: contentY } = contentPoint(event);
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

<div class="canvas-area" style:--zoom={zoom}>
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
    onwheel={zoomWheel}
  >
    <!-- The scrollable area is the content at its drawn size, so scrolling
       still reaches every corner of a zoomed flow; the layer inside it holds
       the content at its own coordinates and is scaled as a whole. -->
    <div
      class="zoom-sizer"
      style:width="{extent.width * zoom}px"
      style:height="{extent.height * zoom}px"
    >
      <div
        class="zoom-layer"
        style:width="{extent.width}px"
        style:height="{extent.height}px"
        style:transform="scale({zoom})"
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
                {@const route = routeOf(from, to)}
                {@const selected = edge.id === graph.selectedEdgeId}
                <path
                  class="edge-hit"
                  role="option"
                  tabindex="0"
                  aria-selected={selected}
                  aria-label="Arrow from {from.name} to {to.name}"
                  d={route.hitPath}
                  onpointerenter={() => showEnds(edge.id)}
                  onpointerleave={leaveEnds}
                  onclick={(event) => {
                    graph.selectEdge(edge.id);
                    event.currentTarget.focus();
                  }}
                  onkeydown={(event) => {
                    // Delete or Backspace deletes the focused arrow.
                    if (event.key !== "Delete" && event.key !== "Backspace")
                      return;
                    event.preventDefault();
                    graph.removeEdge(edge.id);
                  }}
                ></path>
                <path
                  class="edge-line"
                  class:selected
                  class:moving={linking?.edgeId === edge.id}
                  d={route.path}
                  marker-end={selected
                    ? "url(#edge-arrowhead-active)"
                    : "url(#edge-arrowhead)"}
                ></path>
                {#if graph.portOf(edge)}
                  <text
                    class="edge-port"
                    aria-hidden="true"
                    x={route.label.x}
                    y={route.label.y}
                    text-anchor={route.label.anchor}>{graph.portOf(edge)}</text
                  >
                {/if}
                {#if (selected || hoverEdgeId === edge.id) && !linking}
                  <circle
                    class="edge-end"
                    data-end="from"
                    aria-hidden="true"
                    onpointerenter={() => showEnds(edge.id)}
                    onpointerleave={leaveEnds}
                    cx={route.start.x}
                    cy={route.start.y}
                    r={5 / zoom}
                    onpointerdown={(event) =>
                      startLink(event, edge.to, "from", edge.id)}
                  ></circle>
                  <circle
                    class="edge-end"
                    data-end="to"
                    aria-hidden="true"
                    onpointerenter={() => showEnds(edge.id)}
                    onpointerleave={leaveEnds}
                    cx={route.end.x}
                    cy={route.end.y}
                    r={5 / zoom}
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
              {@const preview = previewRoute(linking, anchor)}
              {@const refused =
                linking.targetId !== undefined && !linking.valid}
              <path
                class="edge-preview"
                class:invalid={refused}
                d={preview.path}
                marker-end="url(#edge-arrowhead-{refused
                  ? 'invalid'
                  : 'active'})"
              ></path>
            {/if}
          {/if}
        </svg>
        {#each graph.nodes as node (node.id)}
          {@const mark = statusMark(graph.statusOf(node.id))}
          <button
            type="button"
            class="node block-{node.type} status-{graph.statusOf(node.id) ??
              'idle'}"
            aria-describedby={mark ? `status-${node.id}` : undefined}
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
            onclick={() => graph.select(node.id)}
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
                <NodeMark kind="start" />
              {/if}
              {#if endingIds.has(node.id)}
                <NodeMark kind="end" />
              {/if}
              {#if mark}
                <NodeMark kind={mark} id="status-{node.id}" />
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
                <span class="node-id" aria-hidden="true"
                  >{shortNodeId(node.id)}</span
                >
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
              onpointerdown={(event) =>
                startResize(event, node, "bottom right")}
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
              <button
                type="button"
                role="menuitem"
                onclick={() => choosePort(port)}>{port}</button
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
      </div>
    </div>
    {#if graph.nodes.length === 0}
      <p class="hint">Drag components here to build your graph</p>
    {/if}
    {#if dropHint}
      <p class="drop-hint" role="status">{dropHint}</p>
    {/if}
  </div>
  <div
    class="zoom-bar"
    class:dragging={graph.draggingType !== null || movingId !== null}
    role="group"
    aria-label="Zoom"
  >
    <button
      type="button"
      aria-label="Fit to content"
      title="Fit the whole flow on screen"
      onclick={fitContent}
    >
      <svg viewBox="0 0 12 12" aria-hidden="true">
        <path d="M1 4V1h3M8 1h3v3M11 8v3H8M4 11H1V8" />
      </svg>
    </button>
    <button
      type="button"
      aria-label="Zoom out"
      title="Zoom out"
      onclick={() => setZoom(zoomOut(zoom))}
    >
      <svg viewBox="0 0 12 12" aria-hidden="true"><path d="M2 6h8" /></svg>
    </button>
    <button
      type="button"
      class="zoom-level"
      aria-label="Reset zoom"
      title="Reset to 100%"
      onclick={() => setZoom(ZOOM_DEFAULT)}>{zoomPercent(zoom)}%</button
    >
    <button
      type="button"
      aria-label="Zoom in"
      title="Zoom in"
      onclick={() => setZoom(zoomIn(zoom))}
    >
      <svg viewBox="0 0 12 12" aria-hidden="true"><path d="M2 6h8M6 2v8" /></svg
      >
    </button>
  </div>
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
  onkeydown={canvasKey}
/>

<style>
  /* Holds the scrolling canvas and the zoom controls that float over it. */
  .canvas-area {
    position: relative;
    min-width: 0;
    min-height: 0;
    display: grid;
    /* One screen pixel in the content units the scaled layer is drawn in.
       Anything the pointer has to hit, or read, uses it to keep its size on
       screen however far the content is zoomed. */
    --screen-px: calc(1px / var(--zoom));
  }

  .canvas {
    position: relative;
    overflow: auto;
    background:
      radial-gradient(var(--canvas-dot) 1px, transparent 1px) 0 0 / 20px 20px,
      var(--canvas);
  }

  /* The dot grid is on the scrolling canvas rather than the scaled layer, so
     it stays crisp and evenly spaced at every zoom; nothing snaps to it. */

  .zoom-sizer {
    position: relative;
  }

  /* The content at its own coordinates, drawn at the current scale from its
     top left corner. It is deliberately not promoted to its own layer: the
     browser then redraws the text at the scale it is shown at, rather than
     stretching a picture of it. */
  .zoom-layer {
    position: absolute;
    left: 0;
    top: 0;
    transform-origin: 0 0;
  }

  .zoom-bar {
    position: absolute;
    right: 0.75rem;
    bottom: 0.75rem;
    z-index: 6;
    display: flex;
    align-items: center;
    gap: 0.1rem;
    padding: 0.15rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    box-shadow: var(--shadow-lifted);
  }

  /* The controls float over the canvas, so they step aside while a block is
     dragged: the corner under them takes drops like the rest of the canvas. */
  .zoom-bar.dragging {
    pointer-events: none;
    opacity: 0.4;
  }

  .zoom-bar button {
    min-height: 1.6rem;
    min-width: 1.6rem;
    padding: 0.1rem 0.3rem;
    border: 0;
    background: transparent;
    color: var(--text-muted);
  }

  .zoom-bar button:hover {
    background: var(--surface-hover);
    color: var(--text);
  }

  .zoom-bar svg {
    width: 0.7rem;
    height: 0.7rem;
    fill: none;
    stroke: currentColor;
    stroke-width: 1.4;
    stroke-linecap: round;
  }

  .zoom-level {
    min-width: 3rem;
    font-size: 12px;
    font-variant-numeric: tabular-nums;
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

  /* Arrows are routed paths, so they must never be filled: only the stroke
     of the corners and runs is drawn. */
  /* Arrows keep their weight, their ends and their hit area at the same
     size on screen at every zoom: a stroke that scaled with the content
     would all but disappear once the whole flow is on screen, and an 8px
     target would be 2px wide at a quarter size. */
  .edge-line {
    fill: none;
    stroke: var(--edge);
    stroke-width: calc(1.5 * var(--screen-px));
    stroke-linejoin: round;
  }

  .edge-arrow {
    fill: var(--edge);
  }

  /* A wide invisible stroke under each arrow takes its clicks, the only
     part of the layer that does. */
  .edge-hit {
    fill: none;
    stroke: transparent;
    stroke-width: calc(12 * var(--screen-px));
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
    stroke-width: calc(2.25 * var(--screen-px));
  }

  .edge-line.moving {
    opacity: 0.3;
  }

  .edge-arrow-active {
    fill: var(--accent);
  }

  .edge-preview {
    fill: none;
    stroke: var(--accent);
    stroke-width: calc(2 * var(--screen-px));
    stroke-dasharray: calc(6 * var(--screen-px)) calc(4 * var(--screen-px));
    stroke-linejoin: round;
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
    stroke-width: calc(2 * var(--screen-px));
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
    width: calc(20 * var(--screen-px));
    height: calc(20 * var(--screen-px));
    cursor: crosshair;
    touch-action: none;
  }

  .link-handle::before {
    content: "";
    position: absolute;
    inset: calc(4 * var(--screen-px));
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
    transform: scale(calc(1 / var(--zoom)));
    transform-origin: 0 0;
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
    left: calc(8 * var(--screen-px));
    right: calc(8 * var(--screen-px));
    height: calc(6 * var(--screen-px));
    cursor: ns-resize;
  }

  .resize-edge[data-edge="left"],
  .resize-edge[data-edge="right"] {
    top: calc(8 * var(--screen-px));
    bottom: calc(8 * var(--screen-px));
    width: calc(6 * var(--screen-px));
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
    width: calc(10 * var(--screen-px));
    height: calc(10 * var(--screen-px));
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
    width: calc(14 * var(--screen-px));
    height: calc(14 * var(--screen-px));
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
