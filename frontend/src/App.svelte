<script lang="ts">
  import Workspace from "./lib/Workspace.svelte";

  let backendMessage = $state("Contacting the Go backend…");

  fetch("/api/status", { signal: AbortSignal.timeout(5000) })
    .then((response) => {
      if (!response.ok)
        throw new Error(`status request failed: ${response.status}`);
      return response.json();
    })
    .then((data: { message: string }) => (backendMessage = data.message))
    .catch(() => (backendMessage = "Could not reach the Go backend"));
</script>

<Workspace status={backendMessage} />
