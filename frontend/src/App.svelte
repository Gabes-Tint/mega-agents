<script lang="ts">
  import Workspace from "./lib/Workspace.svelte";

  let backendMessage = $state('Contacting the Go backend…');

  fetch('/api/status', { signal: AbortSignal.timeout(5000) })
    .then((response) => {
      if (!response.ok) throw new Error(`status request failed: ${response.status}`);
      return response.json();
    })
    .then((data: { message: string }) => (backendMessage = data.message))
    .catch(() => (backendMessage = 'Could not reach the Go backend'));
</script>

<header>
  <h1>Mega Agents</h1>
  <p>{backendMessage}</p>
</header>
<Workspace />

<style>
  :global(body) {
    margin: 0;
    font-family: system-ui, sans-serif;
    background: #f4f7f6;
    color: #17342c;
  }

  :global(#app) {
    height: 100vh;
    display: flex;
    flex-direction: column;
  }

  header {
    display: flex;
    align-items: baseline;
    gap: 0.75rem;
    padding: 0.5rem 1rem;
    border-bottom: 1px solid #d5e0db;
    background: #ffffff;
  }

  h1 {
    margin: 0;
    font-size: 1.1rem;
    color: #ff3e00;
  }

  header p {
    margin: 0;
    font-size: 0.85rem;
    color: #5b7a71;
  }
</style>
