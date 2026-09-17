<script lang="ts">
  import { Dialog } from "bits-ui";
  import { GraphStore, isForgeType, shortNodeId } from "./graph.svelte.js";

  let { graph }: { graph: GraphStore } = $props();

  interface Browsing {
    path: string;
    directories: string[];
    error: string;
  }

  let browsing = $state<Browsing | null>(null);
  let dialogOpen = $state(false);
  let loadSequence = 0;

  async function loadDirectory(path: string): Promise<void> {
    const sequence = ++loadSequence;
    browsing = { path, directories: [], error: "" };
    const query = path ? `?path=${encodeURIComponent(path)}` : "";
    try {
      const response = await fetch(`/api/directories${query}`);
      if (!response.ok) {
        if (sequence === loadSequence) {
          browsing = { path, directories: [], error: await response.text() };
        }
        return;
      }
      const body = (await response.json()) as {
        path: string;
        directories: string[];
      };
      if (sequence !== loadSequence) return;
      browsing = { path: body.path, directories: body.directories, error: "" };
    } catch {
      if (sequence === loadSequence) {
        browsing = { path, directories: [], error: "Cannot reach the backend" };
      }
    }
  }

  function closeBrowser(): void {
    loadSequence++;
    dialogOpen = false;
    browsing = null;
  }

  function enterDirectory(name: string): void {
    const current = browsing?.path ?? "";
    void loadDirectory(current === "/" ? `/${name}` : `${current}/${name}`);
  }

  function parentPath(path: string): string {
    const index = path.lastIndexOf("/");
    return index <= 0 ? "/" : path.slice(0, index);
  }

  function chooseFolder(): void {
    const node = graph.selected;
    if (node && browsing) graph.assignProjectFolder(node.id, browsing.path);
    closeBrowser();
  }
</script>

<Dialog.Root
  bind:open={dialogOpen}
  onOpenChange={(open) => {
    if (open) {
      const node = graph.selected;
      if (node) void loadDirectory(node.path ?? "");
    } else {
      closeBrowser();
    }
  }}
>
  <aside aria-label="Node properties">
    <h2>Properties</h2>
    {#if graph.selected}
      {@const node = graph.selected}
      <p>Type: {node.type}</p>
      <p>
        Id: <span class="node-id">{shortNodeId(node.id)}</span>
      </p>
      <label>
        Name
        <input
          type="text"
          value={node.name}
          oninput={(event) => graph.rename(node.id, event.currentTarget.value)}
        />
      </label>
      <label>
        <input
          type="checkbox"
          checked={node.start ?? false}
          onchange={(event) =>
            graph.setStart(node.id, event.currentTarget.checked)}
        />
        Starting point
      </label>
      <button
        type="button"
        aria-pressed={graph.connecting}
        onclick={() =>
          graph.connecting
            ? graph.cancelConnect()
            : graph.startConnect(node.id)}
      >
        Connect
      </button>
      {#if node.type === "project"}
        <label>
          Path
          <input
            type="text"
            value={node.path ?? ""}
            oninput={(event) =>
              graph.setPath(node.id, event.currentTarget.value)}
          />
        </label>
        <Dialog.Trigger class="trigger">Browse…</Dialog.Trigger>
      {/if}
      {#if isForgeType(node.type)}
        <label>
          Repository
          <input
            type="text"
            value={node.repository ?? ""}
            oninput={(event) =>
              graph.setRepository(node.id, event.currentTarget.value)}
          />
        </label>
        <label>
          Secret key
          <input
            type="text"
            value={node.secretKey ?? ""}
            oninput={(event) =>
              graph.setSecretKey(node.id, event.currentTarget.value)}
          />
        </label>
        <label>
          <input
            type="checkbox"
            checked={node.authenticated ?? false}
            onchange={(event) =>
              graph.setAuthenticated(node.id, event.currentTarget.checked)}
          />
          Already authenticated (OAuth)
        </label>
      {/if}
      {#if node.type === "githubapp"}
        <label>
          App ID
          <input
            type="text"
            value={node.appId ?? ""}
            oninput={(event) =>
              graph.setAppId(node.id, event.currentTarget.value)}
          />
        </label>
        <label>
          Private key
          <input
            type="text"
            value={node.privateKeyPath ?? ""}
            oninput={(event) =>
              graph.setPrivateKeyPath(node.id, event.currentTarget.value)}
          />
        </label>
      {/if}
    {:else}
      <p>Select a node on the canvas to edit its properties.</p>
    {/if}
  </aside>

  <Dialog.Portal>
    <Dialog.Overlay class="backdrop" />
    <Dialog.Content class="browser">
      <Dialog.Title class="dialog-title">Directory browser</Dialog.Title>
      {#if browsing}
        <code class="path">{browsing.path || "Home"}</code>
        {#if browsing.error}
          <p class="error">{browsing.error}</p>
        {:else}
          <ul>
            {#each browsing.directories as directory (directory)}
              <li>
                <button
                  type="button"
                  class="directory"
                  onclick={() => enterDirectory(directory)}
                >
                  {directory}
                </button>
              </li>
            {/each}
          </ul>
          <div class="browser-actions">
            <button
              type="button"
              onclick={() => void loadDirectory(parentPath(browsing!.path))}
            >
              Up
            </button>
            <button type="button" onclick={chooseFolder}>
              Choose this folder
            </button>
            <Dialog.Close class="close-button">Cancel</Dialog.Close>
          </div>
        {/if}
      {:else}
        <p class="error">Loading…</p>
      {/if}
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>

<style>
  aside {
    padding: 0.75rem;
    background: #eef4f1;
    border-left: 1px solid #d5e0db;
    overflow-y: auto;
  }

  h2 {
    font-size: 0.85rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: #5b7a71;
    margin: 0 0 0.75rem;
  }

  .node-id {
    font-size: 0.7rem;
    color: #5b7a71;
    letter-spacing: 0.02em;
  }

  p {
    margin: 0 0 0.75rem;
  }

  label {
    display: grid;
    gap: 0.25rem;
    font-size: 0.85rem;
    color: #5b7a71;
  }

  input {
    padding: 0.4rem 0.5rem;
    border: 1px solid #b8ccc4;
    border-radius: 0.375rem;
    font: inherit;
    color: #17342c;
  }

  button {
    width: 100%;
    padding: 0.5rem 0.75rem;
    border: 1px solid #b8ccc4;
    border-radius: 0.375rem;
    background: #ffffff;
    color: #17342c;
    font: inherit;
    cursor: pointer;
    margin-bottom: 0.75rem;
  }

  button[aria-pressed="true"] {
    border-color: #ff3e00;
    box-shadow: 0 0 0 2px rgb(255 62 0 / 0.35);
  }

  /* Bits renders Dialog.Trigger, Dialog.Title, Dialog.Close, Dialog.Overlay,
     and Dialog.Content outside this component's scoped tree, so their styles
     must be global. The trigger and close button share the scoped button look
     above. */
  :global(.trigger) {
    width: 100%;
    padding: 0.5rem 0.75rem;
    border: 1px solid #b8ccc4;
    border-radius: 0.375rem;
    background: #ffffff;
    color: #17342c;
    font: inherit;
    cursor: pointer;
    margin-bottom: 0.75rem;
  }

  :global(.close-button) {
    width: 100%;
    padding: 0.5rem 0.75rem;
    border: 1px solid #b8ccc4;
    border-radius: 0.375rem;
    background: #ffffff;
    color: #17342c;
    font: inherit;
    cursor: pointer;
    margin-bottom: 0;
  }

  :global(.dialog-title) {
    font-size: 0.85rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: #5b7a71;
    margin: 0 0 0.5rem;
  }

  :global(.backdrop) {
    position: fixed;
    inset: 0;
    background: rgb(23 52 44 / 0.35);
  }

  :global(.browser) {
    position: fixed;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    width: min(20rem, 90vw);
    max-height: 60vh;
    overflow-y: auto;
    border: 1px solid #b8ccc4;
    border-radius: 0.375rem;
    background: #ffffff;
    padding: 0.75rem;
  }

  .path {
    display: block;
    font-size: 0.75rem;
    color: #5b7a71;
    word-break: break-all;
    margin-bottom: 0.5rem;
  }

  .error {
    color: #a03030;
    font-size: 0.8rem;
  }

  ul {
    list-style: none;
    margin: 0 0 0.5rem;
    padding: 0;
    display: grid;
    gap: 0.25rem;
  }

  .directory {
    text-align: left;
    margin-bottom: 0;
  }

  .browser-actions {
    display: flex;
    gap: 0.5rem;
  }
</style>
