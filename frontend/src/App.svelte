<script lang="ts">
  import Workspace from "./lib/Workspace.svelte";
  import { parseRoute } from "./lib/workflowUrl.js";

  // Which page the address bar asks for. The list of workflows and their
  // schedules is a page of its own; every other address is the editor,
  // which works out for itself which workflow it opens.
  const schedules =
    parseRoute(globalThis.location.pathname).kind === "schedules";

  // The editor is what the browser almost always asks for, so the workflows
  // page is fetched only when it is the page being opened, as the code
  // editors are.
  const schedulesPage = schedules
    ? import("./lib/Schedules.svelte").then((module) => module.default)
    : undefined;

  let backendMessage = $state("Contacting the Go backend…");

  if (!schedules)
    fetch("/api/status", { signal: AbortSignal.timeout(5000) })
      .then((response) => {
        if (!response.ok)
          throw new Error(`status request failed: ${response.status}`);
        return response.json();
      })
      .then((data: { message: string }) => (backendMessage = data.message))
      .catch(() => (backendMessage = "Could not reach the Go backend"));
</script>

{#if schedulesPage}
  {#await schedulesPage then Page}
    <Page />
  {/await}
{:else}
  <Workspace status={backendMessage} />
{/if}
