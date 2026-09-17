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

  // Brand marks on a 24-unit grid: a stroked mark draws lines, the others
  // fill. Marks without a brand color take the text color of the theme.
  const LOGOS: Record<string, { d: string; stroke?: boolean; color?: string }> =
    {
      claude: {
        d: "M12 12h10m-10 0 7-4m-7 4 5 8.7M12 12V4m0 8v10m0-10-7 4m7-4-8.7-5M12 12H4m8 0L7 3.3m5 8.7 7 4m-7-4 4-7",
        stroke: true,
        color: "#d97757",
      },
      codex: {
        d: "M22.3 9.8a6 6 0 0 0-.5-4.9 6 6 0 0 0-6.5-2.9A6.1 6.1 0 0 0 5 4.2a6 6 0 0 0-4 2.9 6 6 0 0 0 .7 7.1 6 6 0 0 0 .5 4.9 6.1 6.1 0 0 0 6.5 2.9A6 6 0 0 0 13.3 24a6.1 6.1 0 0 0 5.8-4.2 6 6 0 0 0 4-2.9 6.1 6.1 0 0 0-.7-7.1zm-9 12.6a4.5 4.5 0 0 1-2.9-1l.1-.1 4.8-2.8a.8.8 0 0 0 .4-.7v-6.7l2 1.2a.1.1 0 0 1 0 .1v5.6a4.5 4.5 0 0 1-4.5 4.5zm-9.7-4.1a4.5 4.5 0 0 1-.5-3l.1.1 4.8 2.8a.8.8 0 0 0 .8 0l5.8-3.4v2.3a.1.1 0 0 1 0 .1L9.7 20a4.5 4.5 0 0 1-6.1-1.6zM2.3 7.9a4.5 4.5 0 0 1 2.4-2v5.7a.8.8 0 0 0 .4.7l5.8 3.4-2 1.2a.1.1 0 0 1-.1 0l-4.8-2.8a4.5 4.5 0 0 1-1.7-6.2zm16.6 3.9-5.8-3.4 2-1.2a.1.1 0 0 1 .1 0l4.8 2.8a4.5 4.5 0 0 1-.7 8.1v-5.7a.8.8 0 0 0-.4-.7zm2-3-.1-.1-4.8-2.8a.8.8 0 0 0-.8 0L9.4 9.2V6.9a.1.1 0 0 1 0-.1l4.8-2.8a4.5 4.5 0 0 1 6.7 4.7zM8.3 12.9l-2-1.2a.1.1 0 0 1 0-.1V6.1a4.5 4.5 0 0 1 7.4-3.5l-.1.1-4.9 2.8a.8.8 0 0 0-.4.7zm1.1-2.4 2.6-1.5 2.6 1.5v3l-2.6 1.5-2.6-1.5z",
      },
      grok: {
        d: "M3 21 21 3M16.5 4.2A9 9 0 0 0 5.4 18M9.5 20.6a9 9 0 0 0 10.9-12",
        stroke: true,
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
          <path
            d={logo.d}
            fill={logo.stroke ? "none" : "currentColor"}
            stroke={logo.stroke ? "currentColor" : "none"}
            stroke-width="2.5"
            stroke-linecap="round"
          />
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
