<script lang="ts">
  import { onDestroy, onMount } from "svelte";

  // How often to ask again while a backend is still being checked.
  let { pollMs = 3000 }: { pollMs?: number } = $props();

  interface Health {
    backend: string;
    label: string;
    state: "checking" | "available" | "unavailable";
    reason?: string;
    replyMs?: number;
  }

  const HEALTH_URL = "/api/agents/health";

  // Official brand marks on a 24-unit grid (simple-icons; Grok from
  // lobehub/icons), rounded to one decimal. Marks without a brand color take
  // the theme's text color.
  const LOGOS: Record<string, { d: string; color?: string }> = {
    claude: {
      d: "M4.7 16l4.7-2.7.1-.2-.1-.1h-.2l-.8-.1-2.7-.1-2.3-.1-2.3-.1-.6-.1-.5-.7.1-.4.4-.3.7.1 1.5.1 2.3.1 1.7.1 2.4.3h.4l.1-.2-.2-.1-.1-.1-2.3-1.6-2.6-1.7-1.3-.9-.7-.5-.4-.5-.2-1 .7-.7.9.1h.2l.9.7 1.9 1.5 2.5 1.8.4.3.1-.1v-.1l-.1-.2-1.4-2.5-1.4-2.5-.7-1-.2-.6c0-.3-.1-.5-.1-.8l.8-1 .4-.1 1 .1.4.4.6 1.4 1 2.2 1.6 3.1.4.9.3.8.1.3h.1v-.2l.2-1.7.2-2.1.2-2.7.1-.7.4-1 .7-.4.6.2.5.7-.1.5-.3 1.8-.5 2.9-.4 2h.2l.3-.3 1-1.3 1.6-2.1.7-.8.9-.9.5-.4h1.1l.7 1.1-.3 1.2-1.1 1.3-.9 1.2-1.2 1.7-.8 1.3.1.1h.2l2.8-.6 1.5-.3 1.9-.3.8.4.1.4-.3.8-2 .5-2.3.5-3.4.8h-.1l.1.1 1.5.1h.7 1.6l3 .3.8.5.5.6-.1.5-1.2.6-1.6-.4-3.9-.9-1.3-.3h-.2v.1l1.1 1.1 2 1.8 2.5 2.3.2.6-.4.5-.3-.1-2.2-1.6-.8-.8-2-1.6h-.1v.2l.4.6 2.4 3.5.1 1.1-.2.4-.6.2-.6-.1-1.4-2-1.4-2.1-1.2-2-.1.1-.7 7.3-.3.3-.7.3-.6-.5-.3-.7.3-1.5.4-1.9.3-1.5.3-1.9.1-.7h-.1l-1.5 2-2.1 2.9-1.8 1.9-.4.1-.7-.3.1-.7.4-.6 2.4-3 1.4-1.9.9-1.1v-.2l-6.4 4.2-1.1.1-.5-.4.1-.8.2-.2 1.9-1.4z",
      color: "#d97757",
    },
    codex: {
      d: "M22.3 9.8a6 6 0 0 0-.5-4.9 6 6 0 0 0-6.5-2.9 6.1 6.1 0 0 0-10.3 2.2 6 6 0 0 0-4 2.9 6 6 0 0 0 .7 7.1 6 6 0 0 0 .5 4.9 6.1 6.1 0 0 0 6.6 2.9 6 6 0 0 0 4.5 2 6.1 6.1 0 0 0 5.7-4.2 6 6 0 0 0 4-2.9 6.1 6.1 0 0 0-.7-7.1zm-9 12.6a4.5 4.5 0 0 1-2.9-1l.1-.1 4.8-2.8a.8.8 0 0 0 .4-.6v-6.8l2 1.2a.1.1 0 0 1 .1.1v5.5a4.5 4.5 0 0 1-4.5 4.5zm-9.7-4.1a4.5 4.5 0 0 1-.5-3l.1.1 4.8 2.7a.8.8 0 0 0 .8 0l5.8-3.3v2.3a.1.1 0 0 1 0 .1l-4.9 2.8a4.5 4.5 0 0 1-6.1-1.7zm-1.3-10.4a4.5 4.5 0 0 1 2.4-2v5.7a.8.8 0 0 0 .4.7l5.8 3.3-2 1.2a.1.1 0 0 1-.1 0l-4.8-2.8a4.5 4.5 0 0 1-1.7-6.1zm16.6 3.9l-5.8-3.4 2-1.2a.1.1 0 0 1 .1 0l4.8 2.8a4.5 4.5 0 0 1-.7 8.1v-5.7a.8.8 0 0 0-.4-.6zm2-3.1l-.1-.1-4.8-2.7a.8.8 0 0 0-.8 0l-5.8 3.3v-2.3a.1.1 0 0 1 0-.1l4.9-2.8a4.5 4.5 0 0 1 6.6 4.7zm-12.6 4.2l-2-1.2a.1.1 0 0 1-.1-.1v-5.5a4.5 4.5 0 0 1 7.4-3.5l-.1.1-4.8 2.8a.8.8 0 0 0-.4.6zm1.1-2.4l2.6-1.5 2.6 1.5v3l-2.6 1.5-2.6-1.5z",
    },
    grok: {
      d: "M9.3 15.3l7.9-5.9c.4-.3 1-.2 1.2.3 1 2.3.5 5.2-1.4 7.1-2 2-4.7 2.4-7.2 1.4l-2.7 1.3c3.9 2.7 8.6 2 11.6-1 2.3-2.3 3-5.5 2.4-8.4-1-4.2.2-5.9 2.7-9.4.1 0 .1-.1.2-.2l-3.3 3.3-11.4 11.5m-1.7 1.4c-2.8-2.6-2.3-6.8.1-9.2 1.8-1.7 4.6-2.4 7.2-1.4l2.7-1.2a7.8 7.8 0 0 0-1.9-1 9 9 0 0 0-9.7 1.9c-2.5 2.6-3.3 6.5-2 9.8 1 2.5-.6 4.2-2.3 6-.6.6-1.2 1.3-1.7 1.9l7.6-6.8",
    },
    opencode: { d: "M22 24H2V0h20zM17 4.8H7v14.4h10z" },
  };

  let agents = $state<Health[]>([]);
  let timer: ReturnType<typeof setTimeout> | undefined;

  function duration(ms: number): string {
    return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`;
  }

  function describe(health: Health): string {
    if (health.state === "available")
      return `${health.label}: responded in ${duration(health.replyMs ?? 0)}`;
    if (health.state === "checking") return `${health.label}: checking…`;
    return `${health.label}: ${health.reason ?? "unavailable"}`;
  }

  // Reads the health, or with POST starts a new check, and keeps asking
  // while any backend is still being checked.
  async function load(init?: { method: string }): Promise<void> {
    globalThis.clearTimeout(timer);
    try {
      const response = await fetch(HEALTH_URL, init);
      const body = (await response.json()) as { agents?: Health[] };
      if (!response.ok || !Array.isArray(body.agents)) return;
      agents = body.agents;
    } catch {
      // The status bar already says when the backend is unreachable.
      return;
    }
    if (agents.some((health) => health.state === "checking"))
      timer = setTimeout(() => void load(), pollMs);
  }

  onMount(() => void load());
  onDestroy(() => globalThis.clearTimeout(timer));
</script>

{#if agents.length > 0}
  <div class="agent-health" role="group" aria-label="Agent backends">
    {#each agents as health (health.backend)}
      {@const logo = LOGOS[health.backend]}
      <button
        type="button"
        class="agent"
        data-state={health.state}
        aria-label={describe(health)}
        title={describe(health)}
        style:--agent-color={logo.color}
        onclick={() => void load({ method: "POST" })}
      >
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <path d={logo.d} fill="currentColor" />
        </svg>
      </button>
    {/each}
  </div>
{/if}

<style>
  .agent-health {
    display: flex;
    align-items: center;
    gap: 0.15rem;
  }

  .agent {
    display: inline-flex;
    align-items: center;
    min-height: 1.4rem;
    padding: 0 0.25rem;
    border: 0;
    border-radius: 0;
    background: transparent;
    color: var(--agent-color, var(--text));
    cursor: pointer;
  }

  .agent:hover {
    background: var(--surface-hover);
  }

  .agent svg {
    width: 14px;
    height: 14px;
  }

  /* An unreachable backend shows as a faded grey shadow of its mark. */
  .agent[data-state="unavailable"] svg {
    filter: grayscale(1);
    opacity: 0.35;
  }

  .agent[data-state="checking"] svg {
    filter: grayscale(1);
    opacity: 0.5;
    animation: agent-checking 1.4s ease-in-out infinite;
  }

  @keyframes agent-checking {
    50% {
      opacity: 0.2;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .agent[data-state="checking"] svg {
      animation: none;
    }
  }
</style>
