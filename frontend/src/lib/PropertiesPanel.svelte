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
    issue: [],
    commit: [
      {
        field: "message",
        label: "Commit message",
        placeholder: "Changes from Mega Agents run",
      },
    ],
    push: [],
    pullrequest: [
      { field: "title", label: "Title", placeholder: "e.g. Close #7" },
      { field: "body", label: "Body", placeholder: "what the change does" },
      {
        field: "base",
        label: "Base branch",
        placeholder: "the branch the workspace came from",
      },
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
        {#if node.action === "issue"}
          <label>
            Issue number
            <input
              type="number"
              min="1"
              value={node.issue ?? ""}
              oninput={(event) =>
                graph.setActionIssue(node.id, event.currentTarget.value)}
            />
          </label>
        {/if}
        {#if node.action === "commit" || node.action === "pullrequest"}
          <p class="hint">
            Text may use {"{{workspace.branch}}"} and the other workspace fields,
            and {"{{result}}"} from a connected block.
          </p>
        {/if}
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
          <input
            type="checkbox"
            checked={node.continueSession ?? false}
            onchange={(event) =>
              graph.setContinueSession(node.id, event.currentTarget.checked)}
          />
          Continue the connected agent's conversation
        </label>
        <label>
          Max cost (USD)
          <input
            type="number"
            min="0"
            step="0.01"
            placeholder="no budget"
            value={node.maxCostUsd ?? ""}
            oninput={(event) =>
              graph.setAgentNumber(
                node.id,
                "maxCostUsd",
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
      {#if node.type === "command"}
        <label>
          Command
          <textarea
            rows="3"
            placeholder="e.g. make verify"
            value={node.command ?? ""}
            oninput={(event) =>
              graph.setCommand(node.id, event.currentTarget.value)}
          ></textarea>
        </label>
        <p class="hint">
          Runs with sh in the connected workspace or the project folder. Exit 0
          takes passed; anything else takes failed, or fails the run when no
          arrow takes failed.
        </p>
        <label>
          Timeout (minutes)
          <input
            type="number"
            min="1"
            max="240"
            placeholder="10"
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
      <button
        type="button"
        class="delete"
        onclick={() => graph.removeNode(node.id)}
      >
        <svg viewBox="0 0 16 16" aria-hidden="true">
          <path
            d="M2.5 4h11M6 4V2.5h4V4M4 4l.7 9.5h6.6L12 4M6.8 6.5v4.5M9.2 6.5v4.5"
          />
        </svg>
        Delete block
      </button>
      <p class="hint">
        Delete or Backspace on a selected block deletes it too.
      </p>
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
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    min-height: 0;
    padding: 0.75rem;
    background: var(--surface);
    border-left: 1px solid var(--border);
    overflow-y: auto;
  }

  h2 {
    margin: 0;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-muted);
  }

  .node-id {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-muted);
    letter-spacing: 0.02em;
  }

  p {
    margin: 0;
    color: var(--text-muted);
  }

  label {
    display: grid;
    gap: 0.25rem;
    font-size: 12px;
    font-weight: 500;
    color: var(--text-muted);
  }

  /* A checkbox reads as one line: box, then its label. */
  label:has(> input[type="checkbox"]) {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    color: var(--text);
  }

  h3 {
    margin: 0.5rem 0 0;
    padding-top: 0.6rem;
    border-top: 1px solid var(--border);
    font-size: 12px;
    font-weight: 600;
    color: var(--text);
  }

  .error {
    color: var(--fail);
    font-size: 12px;
  }

  .hint {
    font-size: 12px;
    color: var(--text-faint);
  }

  .action-list {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.2rem;
  }

  .action-list button {
    justify-content: flex-start;
  }

  button {
    width: 100%;
  }

  .delete {
    margin-top: 0.5rem;
    border-color: var(--fail);
    background: transparent;
    color: var(--fail);
  }

  .delete:hover:not(:disabled) {
    background: var(--fail-soft);
  }

  .delete svg {
    width: 0.9rem;
    height: 0.9rem;
    fill: none;
    stroke: currentColor;
    stroke-width: 1.4;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  button[aria-pressed="true"] {
    border-color: var(--accent);
    background: var(--focus);
  }

  .path {
    display: block;
    margin-bottom: 0.5rem;
    padding: 0.35rem 0.5rem;
    border-radius: var(--radius);
    background: var(--surface-sunken);
    font-size: 12px;
    color: var(--text-muted);
    word-break: break-all;
  }

  ul {
    list-style: none;
    margin: 0 0 0.5rem;
    padding: 0;
    display: grid;
    gap: 0.2rem;
  }

  .directory {
    justify-content: flex-start;
  }

  .browser-actions {
    display: flex;
    gap: 0.5rem;
  }

  .browser-actions button,
  .browser-actions :global(.close-button) {
    width: auto;
    flex: 1;
  }
</style>
