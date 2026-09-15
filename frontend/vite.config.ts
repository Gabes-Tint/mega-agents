import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080'
    }
  }
});
