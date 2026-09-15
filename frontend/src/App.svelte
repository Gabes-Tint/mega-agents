<script lang="ts">
  let backendMessage = 'Contacting the Go backend…';

  fetch('/api/status', { signal: AbortSignal.timeout(5000) })
    .then((response) => {
      if (!response.ok) throw new Error(`status request failed: ${response.status}`);
      return response.json();
    })
    .then((data: { message: string }) => (backendMessage = data.message))
    .catch(() => (backendMessage = 'Could not reach the Go backend'));
</script>

<main>
  <h1>Mega Agents</h1>
  <p>{backendMessage}</p>
</main>

<style>
  :global(body) {
    margin: 0;
    font-family: system-ui, sans-serif;
    background: #f4f7f6;
    color: #17342c;
  }

  main {
    min-height: 100vh;
    display: grid;
    place-content: center;
    text-align: center;
  }

  h1 {
    color: #ff3e00;
  }
</style>
