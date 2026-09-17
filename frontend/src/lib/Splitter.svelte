<script lang="ts">
  import { clampPanel, PANEL_LIMITS, type PanelName } from "./panelLayout.js";

  // A divider people drag, or move with the arrow keys, to resize the panel
  // beside it. `grows` is the pointer direction that makes the panel larger:
  // right for the left sidebar, left for the right one, up for the bottom
  // panel.
  let {
    label,
    name,
    size,
    grows,
    onresize,
  }: {
    label: string;
    name: PanelName;
    size: number;
    grows: "left" | "right" | "up";
    onresize: (size: number) => void;
  } = $props();

  const STEP = 16;
  const vertical = $derived(grows !== "up");
  let drag: { start: number; size: number } | null = null;

  function travel(event: PointerEvent): number {
    return vertical ? event.clientX : event.clientY;
  }

  function begin(event: PointerEvent & { currentTarget: HTMLElement }) {
    if (event.button !== 0) return;
    event.preventDefault();
    event.currentTarget.setPointerCapture?.(event.pointerId);
    drag = { start: travel(event), size };
  }

  function move(event: PointerEvent) {
    if (!drag) return;
    const moved = travel(event) - drag.start;
    onresize(
      clampPanel(name, drag.size + (grows === "right" ? moved : -moved)),
    );
  }

  function keydown(event: { key: string; preventDefault(): void }) {
    const larger = { left: "ArrowLeft", right: "ArrowRight", up: "ArrowUp" }[
      grows
    ];
    const smaller = { left: "ArrowRight", right: "ArrowLeft", up: "ArrowDown" }[
      grows
    ];
    const next =
      event.key === larger
        ? size + STEP
        : event.key === smaller
          ? size - STEP
          : event.key === "Home"
            ? PANEL_LIMITS[name].min
            : event.key === "End"
              ? PANEL_LIMITS[name].max
              : undefined;
    if (next === undefined) return;
    event.preventDefault();
    onresize(clampPanel(name, next));
  }
</script>

<!-- A focusable separator with a value is the WAI-ARIA window splitter, an
     interactive widget Svelte's checks take for a static one. -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
<div
  class="splitter splitter-{name}"
  class:vertical
  role="separator"
  tabindex="0"
  aria-label={label}
  aria-orientation={vertical ? "vertical" : "horizontal"}
  aria-valuenow={size}
  aria-valuemin={PANEL_LIMITS[name].min}
  aria-valuemax={PANEL_LIMITS[name].max}
  title="Drag to resize; double-click to restore"
  onpointerdown={begin}
  onkeydown={keydown}
  ondblclick={() => onresize(PANEL_LIMITS[name].default)}
></div>

<svelte:window
  onpointermove={move}
  onpointerup={() => (drag = null)}
  onpointercancel={() => (drag = null)}
/>

<style>
  /* A thin hit area over the panel's border; the border lights up while the
     divider is hovered, dragged or focused. */
  .splitter {
    position: relative;
    z-index: 5;
    touch-action: none;
    height: 8px;
    margin-top: -4px;
    align-self: start;
    cursor: row-resize;
  }

  .splitter.vertical {
    width: 8px;
    height: auto;
    margin: 0;
    align-self: stretch;
    cursor: col-resize;
  }

  .splitter::after {
    content: "";
    position: absolute;
    inset: 3px 0;
    background: transparent;
    transition: background 120ms;
  }

  .splitter.vertical::after {
    inset: 0 3px;
  }

  .splitter:hover::after,
  .splitter:active::after,
  .splitter:focus-visible::after {
    background: var(--accent);
  }

  .splitter:focus-visible {
    outline: none;
  }
</style>
