<script lang="ts">
  import { Dialog } from "bits-ui";
  import {
    AGENT_BACKENDS,
    GIT_ACTIONS,
    GraphStore,
    isForgeType,
    shortNodeId,
    type ActionField,
    type AgentField,
    type GitAction,
  } from "./graph.svelte.js";

  let { graph }: { graph: GraphStore } = $props();

  interface Browsing {
    path: string;
    directories: string[];
    error: string;
  }

  let browsing = $state<Browsing | null>(null);
  let dialogOpen = $state(false);
  let newAction = $state<GitAction>("fetch");

  // Text settings of an Agent block, in the order the panel shows them.
  const AGENT_TEXT: {
    field: AgentField;
    label: string;
    placeholder: string;
  }[] = [
    { field: "model", label: "Model", placeholder: "the CLI's default" },
    { field: "effort", label: "Effort", placeholder: "e.g. low, medium, high" },
  ];

  function schemaIsJSON(text: string | undefined): boolean {
    if (!text?.trim()) return true;
    try {
      JSON.parse(text);
      return true;
    } catch {
      return false;
    }
  }

  // Configuration each Git action accepts, in the order the panel shows it.
  const ACTION_FIELDS: Record<
    GitAction,
    { field: ActionField; label: string; placeholder: string }[]
  > = {
    fetch: [],
    worktree: [
      { field: "branch", label: "Branch", placeholder: "feature/name" },
      { field: "base", label: "Base", placeholder: "remote default branch" },
      {
        field: "worktreePath",
        label: "Worktree path",
        placeholder: "~/.mega-agents/worktrees/…",
      },
    ],
    rebase: [
      { field: "onto", label: "Onto", placeholder: "the workspace base" },
    ],
  };
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
      {#if graph.hasLog(node.id)}
        <button type="button" onclick={() => graph.openLog(node.id)}>
          Show logs
        </button>
      {/if}
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
      {#if node.type === "github"}
        <h3 id="actions-heading">Actions</h3>
        <ol aria-labelledby="actions-heading" class="action-list">
          {#each graph.actionSequence(node.id) as action, index (action.id)}
            <li>
              <button type="button" onclick={() => graph.select(action.id)}>
                {index + 1}. {action.name}
              </button>
            </li>
          {/each}
        </ol>
        <label>
          New action
          <select bind:value={newAction}>
            {#each GIT_ACTIONS as option (option.action)}
              <option value={option.action}>{option.label}</option>
            {/each}
          </select>
        </label>
        <button
          type="button"
          onclick={() => graph.addAction(node.id, newAction)}
        >
          Add action
        </button>
      {/if}
      {#if node.type === "action" && node.action}
        <p>
          Git action: {GIT_ACTIONS.find(
            (option) => option.action === node.action,
          )?.label}
        </p>
        {#each ACTION_FIELDS[node.action] as input (input.field)}
          <label>
            {input.label}
            <input
              type="text"
              placeholder={input.placeholder}
              value={node[input.field] ?? ""}
              oninput={(event) =>
                graph.setActionField(
                  node.id,
                  input.field,
                  event.currentTarget.value,
                )}
            />
          </label>
        {/each}
      {/if}
      {#if node.type === "agent"}
        <label>
          Backend
          <select
            value={node.backend ?? "claude"}
            onchange={(event) =>
              graph.setAgentField(
                node.id,
                "backend",
                event.currentTarget.value,
              )}
          >
            {#each AGENT_BACKENDS as option (option.backend)}
              <option value={option.backend}>{option.label}</option>
            {/each}
          </select>
        </label>
        {#each AGENT_TEXT as input (input.field)}
          <label>
            {input.label}
            <input
              type="text"
              placeholder={input.placeholder}
              value={node[input.field] ?? ""}
              oninput={(event) =>
                graph.setAgentField(
                  node.id,
                  input.field,
                  event.currentTarget.value,
                )}
            />
          </label>
        {/each}
        <label>
          Prompt
          <textarea
            rows="5"
            value={node.prompt ?? ""}
            oninput={(event) =>
              graph.setAgentField(node.id, "prompt", event.currentTarget.value)}
          ></textarea>
        </label>
        <p class="hint">
          Placeholders: {"{{workspace.path}}"}, {"{{workspace.branch}}"},
          {"{{workspace.base}}"}, {"{{workspace.repository}}"} from a connected worktree;
          {"{{result}}"} or {"{{results.<agent>}}"} from connected agents.
        </p>
        <label>
          Output schema
          <textarea
            rows="4"
            placeholder="optional JSON Schema the reply must satisfy"
            value={node.outputSchema ?? ""}
            oninput={(event) =>
              graph.setAgentField(
                node.id,
                "outputSchema",
                event.currentTarget.value,
              )}
          ></textarea>
        </label>
        {#if !schemaIsJSON(node.outputSchema)}
          <p class="error" role="alert">The output schema is not valid JSON</p>
        {/if}
        <label>
          Retries
          <input
            type="number"
            min="0"
            max="5"
            placeholder="2"
            value={node.retries ?? ""}
            oninput={(event) =>
              graph.setAgentNumber(
                node.id,
                "retries",
                event.currentTarget.value,
              )}
          />
        </label>
        <label>
          Timeout (minutes)
          <input
            type="number"
            min="1"
            max="240"
            placeholder="30"
            value={node.timeoutMinutes ?? ""}
            oninput={(event) =>
              graph.setAgentNumber(
                node.id,
                "timeoutMinutes",
                event.currentTarget.value,
              )}
          />
        </label>
      {/if}
      {#if node.type === "jsonschema"}
        <label>
          Schema
          <textarea
            rows="8"
            placeholder="JSON Schema the value must satisfy"
            value={node.schema ?? ""}
            oninput={(event) =>
              graph.setSchema(node.id, event.currentTarget.value)}
          ></textarea>
        </label>
        {#if !schemaIsJSON(node.schema)}
          <p class="error" role="alert">The schema is not valid JSON</p>
        {/if}
      {/if}
      {#if node.type === "router"}
        <h3>Cases</h3>
        <p class="hint">
          CEL conditions over value, checked in order; the first that holds
          wins, otherwise the value takes default. Example: value.verdict ==
          "approve".
        </p>
        {#each node.cases ?? [] as routeCase, index (index)}
          <label>
            Case {index + 1} name
            <input
              type="text"
              value={routeCase.name}
              oninput={(event) =>
                graph.setCase(
                  node.id,
                  index,
                  "name",
                  event.currentTarget.value,
                )}
            />
          </label>
          <label>
            Case {index + 1} expression
            <input
              type="text"
              value={routeCase.expression}
              oninput={(event) =>
                graph.setCase(
                  node.id,
                  index,
                  "expression",
                  event.currentTarget.value,
                )}
            />
          </label>
          <button
            type="button"
            onclick={() => graph.removeCase(node.id, index)}
          >
            Remove case {index + 1}
          </button>
        {/each}
        <button type="button" onclick={() => graph.addCase(node.id)}>
          Add case
        </button>
      {/if}
      {#if graph.outputPorts(node.id).length > 0}
        {#each graph.outgoingEdges(node.id) as edge (edge.id)}
          {@const target = graph.nodes.find(
            (candidate) => candidate.id === edge.to,
          )}
          <label>
            Output to {target?.name}
            <select
              value={graph.portOf(edge)}
              onchange={(event) =>
                graph.setEdgePort(edge.id, event.currentTarget.value)}
            >
              {#each graph.outputPorts(node.id) as port (port)}
                <option value={port}>{port}</option>
              {/each}
            </select>
          </label>
        {/each}
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

  h3 {
    font-size: 0.8rem;
    color: #5b7a71;
    margin: 0.75rem 0 0.25rem;
  }

  textarea {
    resize: vertical;
    font-family: ui-monospace, monospace;
    font-size: 0.8rem;
  }

  .error {
    color: #a03030;
    font-size: 0.8rem;
  }

  .hint {
    font-size: 0.75rem;
    color: #5b7a71;
  }

  .action-list {
    margin: 0 0 0.5rem;
    padding: 0;
    list-style: none;
  }

  .action-list button {
    margin-bottom: 0.25rem;
    text-align: left;
  }

  input,
  select,
  textarea {
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
