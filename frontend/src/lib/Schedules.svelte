<script lang="ts">
  // The page that lists every saved workflow and the schedule it runs on.
  //
  // A schedule is a cron expression the server keeps in the workflow file
  // and the server reads: while one is being typed the runs it would start
  // are asked for from the same reader that will run them, so what is shown
  // here is what will actually happen.
  import { onDestroy } from "svelte";
  import ThemeToggle from "./ThemeToggle.svelte";
  import { describeRun } from "./scheduleTimes.js";
  import { workflowPath } from "./workflowUrl.js";

  let { previewDelayMs = 250 }: { previewDelayMs?: number } = $props();

  interface Summary {
    name: string;
    updatedAt: string;
    schedule?: string;
    nextRuns?: string[];
  }

  interface Row {
    name: string;
    updatedAt: string;
    // schedule is what is saved; draft is what is in the field.
    schedule: string;
    draft: string;
    nextRuns: string[];
    // problem is what the server said about the expression being typed, or
    // about a save it refused.
    problem: string;
    checking: boolean;
    saving: boolean;
    saved: boolean;
  }

  let rows = $state<Row[] | null>(null);
  let failure = $state("");
  // The timer each row's next preview is waiting on, so typing on only asks
  // once, and the number of the preview it is waiting for, so a slow answer
  // cannot overwrite a newer one. Neither is drawn, so neither is state.
  const timers: Record<string, ReturnType<typeof setTimeout>> = {};
  const asked: Record<string, number> = {};

  function rowOf(summary: Summary): Row {
    return {
      name: summary.name,
      updatedAt: summary.updatedAt,
      schedule: summary.schedule ?? "",
      draft: summary.schedule ?? "",
      nextRuns: summary.nextRuns ?? [],
      problem: "",
      checking: false,
      saving: false,
      saved: false,
    };
  }

  async function load(): Promise<void> {
    try {
      const response = await fetch("/api/workflows");
      if (!response.ok) {
        failure = "Could not read the saved workflows.";
        rows = [];
        return;
      }
      rows = ((await response.json()) as Summary[]).map(rowOf);
    } catch {
      failure = "Could not reach the Go backend.";
      rows = [];
    }
  }

  void load();

  onDestroy(() => {
    for (const timer of Object.values(timers)) globalThis.clearTimeout(timer);
  });

  // typed keeps the field as it is and asks, once the typing settles, what
  // the expression would run.
  function typed(row: Row, value: string): void {
    row.draft = value;
    row.problem = "";
    row.saved = false;
    globalThis.clearTimeout(timers[row.name]);
    if (value.trim() === "") {
      row.checking = false;
      row.nextRuns = row.draft === row.schedule ? row.nextRuns : [];
      return;
    }
    row.checking = true;
    timers[row.name] = globalThis.setTimeout(
      () => void preview(row),
      previewDelayMs,
    );
  }

  async function preview(row: Row): Promise<void> {
    const attempt = (asked[row.name] ?? 0) + 1;
    asked[row.name] = attempt;
    const query = new globalThis.URLSearchParams({
      schedule: row.draft,
    }).toString();
    try {
      const response = await fetch(`/api/schedule/preview?${query}`);
      // An answer to an older keystroke says nothing about what is typed now.
      if (asked[row.name] !== attempt) return;
      if (!response.ok) {
        row.problem = (await response.text()).trim();
        row.nextRuns = [];
        row.checking = false;
        return;
      }
      const answer = (await response.json()) as { nextRuns?: string[] };
      row.nextRuns = answer.nextRuns ?? [];
      row.problem = "";
    } catch {
      if (asked[row.name] !== attempt) return;
      row.problem = "Could not reach the Go backend.";
    }
    row.checking = false;
  }

  // save puts the workflow on the schedule in its field, or with an empty
  // one takes it off.
  async function save(row: Row, schedule: string): Promise<void> {
    globalThis.clearTimeout(timers[row.name]);
    row.saving = true;
    row.problem = "";
    try {
      const response = await fetch(
        `/api/workflows/${encodeURIComponent(row.name)}/schedule`,
        {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ schedule }),
        },
      );
      if (!response.ok) {
        row.problem = (await response.text()).trim();
        return;
      }
      const summary = (await response.json()) as Summary;
      row.schedule = summary.schedule ?? "";
      row.draft = row.schedule;
      row.nextRuns = summary.nextRuns ?? [];
      row.updatedAt = summary.updatedAt;
      row.saved = true;
    } catch {
      row.problem = "Could not reach the Go backend.";
    } finally {
      row.saving = false;
      row.checking = false;
    }
  }

  function savable(row: Row): boolean {
    return (
      !row.saving &&
      !row.checking &&
      row.problem === "" &&
      row.draft.trim() !== "" &&
      row.draft.trim() !== row.schedule
    );
  }
</script>

<div class="page">
  <header class="topbar">
    <a class="brand" href="/">
      <svg class="logo" viewBox="0 0 20 20" aria-hidden="true">
        <rect x="1.5" y="1.5" width="7" height="7" rx="2" />
        <rect x="11.5" y="11.5" width="7" height="7" rx="2" />
        <path d="M8.5 5h3a2 2 0 0 1 2 2v4.5" />
      </svg>
      <h1>Mega Agents</h1>
    </a>
    <div class="spacer"></div>
    <a class="ghost" href="/">Back to the editor</a>
    <ThemeToggle />
  </header>

  <main>
    <div class="intro">
      <h2>Workflows</h2>
      <p>
        A workflow with a schedule starts itself while the editor is running.
        Schedules are cron expressions in this machine's time zone, such as
        <code>0 9 * * 1-5</code> for every weekday at nine, or
        <code>@daily</code>. A run already going never holds the next one back.
      </p>
    </div>

    {#if failure}
      <p class="problem">{failure}</p>
    {/if}

    {#if rows === null}
      <p class="quiet">Reading the saved workflows…</p>
    {:else if rows.length === 0}
      <p class="quiet">
        No saved workflows yet. Save one in the editor and it can be put on a
        schedule here.
      </p>
    {:else}
      <ul class="workflows">
        {#each rows as row (row.name)}
          <li class="workflow">
            <div class="identity">
              <a class="name" href={workflowPath(row.name)}>{row.name}</a>
              <span class="quiet">
                saved {describeRun(row.updatedAt).absolute}
              </span>
            </div>
            <div class="schedule">
              <label class="field">
                <span class="label">Schedule</span>
                <input
                  type="text"
                  spellcheck="false"
                  placeholder="0 9 * * 1-5"
                  aria-label="Schedule for {row.name}"
                  value={row.draft}
                  oninput={(event) => typed(row, event.currentTarget.value)}
                />
              </label>
              <button
                type="button"
                class="save"
                disabled={!savable(row)}
                aria-label="Save schedule for {row.name}"
                onclick={() => void save(row, row.draft.trim())}
              >
                {row.saving ? "Saving…" : "Save"}
              </button>
              {#if row.schedule !== ""}
                <button
                  type="button"
                  class="ghost"
                  disabled={row.saving}
                  aria-label="Clear schedule for {row.name}"
                  onclick={() => void save(row, "")}
                >
                  Clear
                </button>
              {/if}
            </div>
            <div class="runs">
              {#if row.problem}
                <p class="problem">{row.problem}</p>
              {:else if row.checking}
                <p class="quiet">Working out the next runs…</p>
              {:else if row.nextRuns.length > 0}
                <p class="quiet">
                  {row.draft === row.schedule
                    ? "Next runs"
                    : "Next runs once saved"}
                </p>
                <ol class="next">
                  {#each row.nextRuns as run (run)}
                    {@const shown = describeRun(run)}
                    <li>
                      <time datetime={run}>{shown.absolute}</time>
                      <span class="quiet">{shown.relative}</span>
                    </li>
                  {/each}
                </ol>
              {:else if row.draft.trim() === ""}
                <p class="quiet">Runs when you ask it to.</p>
              {:else}
                <p class="quiet">This expression never comes round.</p>
              {/if}
              {#if row.saved}
                <p class="saved">
                  {row.schedule === ""
                    ? "Taken off the schedule."
                    : "Now on the schedule."}
                </p>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </main>
</div>

<style>
  .page {
    display: flex;
    flex-direction: column;
    min-height: 100vh;
    background: var(--surface);
    color: var(--text);
  }

  .topbar {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 16px;
    border-bottom: 1px solid var(--border);
    background: var(--panel);
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 8px;
    color: inherit;
    text-decoration: none;
  }

  .brand h1 {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
  }

  .logo {
    width: 20px;
    height: 20px;
    fill: none;
    stroke: currentColor;
    stroke-width: 1.5;
  }

  .spacer {
    flex: 1;
  }

  .ghost {
    padding: 5px 10px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: transparent;
    color: inherit;
    font: inherit;
    font-size: 13px;
    text-decoration: none;
    cursor: pointer;
  }

  main {
    width: min(880px, 100%);
    margin: 0 auto;
    padding: 24px 16px 48px;
  }

  .intro h2 {
    margin: 0 0 6px;
    font-size: 20px;
  }

  .intro p {
    margin: 0 0 20px;
    max-width: 60ch;
    color: var(--muted);
    font-size: 13px;
    line-height: 1.6;
  }

  code {
    padding: 1px 4px;
    border-radius: 4px;
    background: var(--surface-strong, rgba(127, 127, 127, 0.15));
    font-size: 12px;
  }

  .workflows {
    display: grid;
    gap: 12px;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .workflow {
    display: grid;
    gap: 10px;
    padding: 14px 16px;
    border: 1px solid var(--border);
    border-radius: 10px;
    background: var(--panel);
  }

  .identity {
    display: flex;
    align-items: baseline;
    gap: 10px;
    flex-wrap: wrap;
  }

  .name {
    color: inherit;
    font-size: 15px;
    font-weight: 600;
  }

  .schedule {
    display: flex;
    align-items: end;
    gap: 8px;
    flex-wrap: wrap;
  }

  .field {
    display: grid;
    gap: 4px;
    flex: 1 1 260px;
  }

  .label {
    color: var(--muted);
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  input {
    padding: 6px 8px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--surface);
    color: inherit;
    font-family: var(--mono, ui-monospace, monospace);
    font-size: 13px;
  }

  .save {
    padding: 7px 12px;
    border: 1px solid transparent;
    border-radius: 6px;
    background: var(--accent, #3b82f6);
    color: #fff;
    font: inherit;
    font-size: 13px;
    cursor: pointer;
  }

  .save:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .next {
    display: grid;
    gap: 2px;
    margin: 4px 0 0;
    padding: 0 0 0 18px;
    font-size: 13px;
  }

  .next li {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }

  time {
    font-variant-numeric: tabular-nums;
  }

  .quiet {
    margin: 0;
    color: var(--muted);
    font-size: 12px;
  }

  .problem {
    margin: 0;
    color: var(--danger, #dc2626);
    font-size: 12px;
  }

  .saved {
    margin: 6px 0 0;
    color: var(--muted);
    font-size: 12px;
  }
</style>
