<script lang="ts">
  import { MARK_LABELS, type MarkKind } from "./nodeMarks.js";

  // One mark in a block's header. The flags borrow the racing pair: a plain
  // flag waves the flow off, the chequered one ends it. The run marks share
  // their grid and weight so several marks sit in a row without crowding
  // the block's name.
  let { kind, id }: { kind: MarkKind; id?: string } = $props();

  const flag = $derived(kind === "start" || kind === "end");
</script>

<span
  {id}
  class="node-mark {kind}"
  data-mark={kind}
  title={MARK_LABELS[kind]}
  aria-hidden="true"
>
  <svg viewBox="0 0 24 24">
    {#if flag}
      <path class="pole" d="M7 3v18" />
      <path class="cloth" d="M7 4h12v9H7z" />
      {#if kind === "end"}
        <path
          class="chequer"
          d="M7 4h4v4.5H7zM15 4h4v4.5h-4zM11 8.5h4V13h-4z"
        />
      {/if}
    {:else if kind === "breakpoint"}
      <path class="dot" d="M4.5 12a7.5 7.5 0 1 0 15 0a7.5 7.5 0 1 0-15 0" />
    {:else}
      <path class="tone" d="M3 12a9 9 0 1 0 18 0a9 9 0 1 0-18 0" />
      {#if kind === "running"}
        <path class="spinner" d="M12 3.5a8.5 8.5 0 0 1 8.5 8.5" />
      {:else if kind === "paused"}
        <path class="bar" d="M10 8v8" />
        <path class="bar" d="M14 8v8" />
      {:else if kind === "succeeded"}
        <path class="line" d="M7.5 12.5 11 16l5.5-7" />
      {:else if kind === "failed"}
        <path class="line" d="m8.5 8.5 7 7m0-7-7 7" />
      {:else}
        <path class="line" d="M8 12h8" />
      {/if}
    {/if}
  </svg>
  <span class="reading">{MARK_LABELS[kind]}</span>
</span>

<style>
  .node-mark {
    flex: none;
    display: inline-grid;
    place-items: center;
    width: 13px;
    height: 13px;
  }

  svg {
    width: 13px;
    height: 13px;
    overflow: visible;
  }

  /* Only an aria-describedby reference reads this; the header shows the
     drawing. */
  .reading {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }

  .pole {
    fill: none;
    stroke-width: 2;
    stroke-linecap: round;
  }

  .cloth {
    stroke-width: 1;
    stroke-linejoin: round;
  }

  /* The green flag that starts a race, drawn in one colour so it reads at
     13px. */
  .start .pole,
  .start .cloth {
    stroke: var(--ok);
  }

  .start .cloth {
    fill: var(--ok);
  }

  /* The chequer is three real squares on the page's own ground, so it
     inverts with the theme instead of washing out on either. */
  .end .pole,
  .end .cloth {
    stroke: var(--text);
  }

  .end .cloth {
    fill: var(--surface);
  }

  .chequer {
    fill: var(--text);
  }

  .running {
    color: var(--info);
  }

  /* A debugger's red dot, drawn solid so a block carrying a breakpoint reads
     as armed even before a run reaches it. */
  .breakpoint .dot {
    fill: var(--fail);
    stroke: var(--fail-soft);
    stroke-width: 3;
  }

  /* Waiting is not working, so the paused mark takes the warning colour
     rather than the blue a running block spins in. */
  .paused {
    color: var(--warn);
  }

  .bar {
    fill: none;
    stroke: currentColor;
    stroke-width: 2.6;
    stroke-linecap: round;
  }

  .succeeded {
    color: var(--ok);
  }

  .failed {
    color: var(--fail);
  }

  /* A skipped block did no work, so its mark stays neutral rather than
     claiming a result. */
  .skipped {
    color: var(--text-muted);
  }

  .tone {
    fill: currentColor;
    opacity: 0.22;
  }

  .line {
    fill: none;
    stroke: currentColor;
    stroke-width: 2.4;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  .spinner {
    fill: none;
    stroke: currentColor;
    stroke-width: 2.6;
    stroke-linecap: round;
  }

  .running svg {
    animation: spin 0.9s linear infinite;
  }

  /* The paused mark breathes instead of spinning: it is waiting for someone,
     not working. */
  .paused svg {
    animation: breathe 1.6s ease-in-out infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(1turn);
    }
  }

  @keyframes breathe {
    50% {
      opacity: 0.35;
    }
  }

  /* Without motion the arc still reads as a part-drawn ring and the two bars
     still read as paused, so the block keeps a mark of its own. */
  @media (prefers-reduced-motion: reduce) {
    .running svg,
    .paused svg {
      animation: none;
    }
  }
</style>
