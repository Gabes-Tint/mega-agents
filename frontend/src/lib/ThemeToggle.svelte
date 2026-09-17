<script lang="ts">
  type Theme = "light" | "dark";

  // A theme the user picked is remembered in this browser; until then the
  // editor follows the system preference.
  const THEME_KEY = "mega-agents:theme";

  function initialTheme(): Theme {
    try {
      const stored = localStorage.getItem(THEME_KEY);
      if (stored === "light" || stored === "dark") return stored;
    } catch {
      // Storage may be unavailable; fall back to the system preference.
    }
    return globalThis.matchMedia?.("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light";
  }

  let theme = $state<Theme>(initialTheme());

  $effect(() => {
    document.documentElement.dataset.theme = theme;
  });

  function toggle(): void {
    theme = theme === "dark" ? "light" : "dark";
    try {
      localStorage.setItem(THEME_KEY, theme);
    } catch {
      // The choice still applies to this page.
    }
  }
</script>

<button
  type="button"
  class="theme-toggle"
  aria-label={theme === "dark"
    ? "Switch to light theme"
    : "Switch to dark theme"}
  title={theme === "dark" ? "Light theme" : "Dark theme"}
  onclick={toggle}
>
  {#if theme === "dark"}
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <circle cx="8" cy="8" r="3" />
      <path
        d="M8 1.5v1.5M8 13v1.5M1.5 8H3M13 8h1.5M3.4 3.4l1 1M11.6 11.6l1 1M3.4 12.6l1-1M11.6 4.4l1-1"
      />
    </svg>
  {:else}
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <path d="M13.5 9.5A5.5 5.5 0 0 1 6.5 2.5a5.5 5.5 0 1 0 7 7Z" />
    </svg>
  {/if}
</button>

<style>
  .theme-toggle {
    display: inline-grid;
    place-items: center;
    width: 2rem;
    height: 2rem;
    padding: 0;
    border: 1px solid transparent;
    border-radius: var(--radius);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
  }

  .theme-toggle:hover {
    background: var(--surface-hover);
    color: var(--text);
  }

  svg {
    width: 1rem;
    height: 1rem;
    fill: none;
    stroke: currentColor;
    stroke-width: 1.5;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
</style>
