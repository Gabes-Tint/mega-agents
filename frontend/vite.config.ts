import { svelte } from "@sveltejs/vite-plugin-svelte";
import { defaultExclude, defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [svelte()],
  resolve: {
    conditions: ["browser"],
  },
  test: {
    environment: "jsdom",
    testTimeout: 5000,
    setupFiles: ["./src/test-setup.ts"],
    // Browser suites live in e2e/ and run via `bun run e2e` (Playwright).
    exclude: [...defaultExclude, "e2e/**"],
    coverage: {
      provider: "v8",
      reporter: ["text", "json-summary"],
      reportsDirectory: "coverage",
      include: ["src/**/*.{ts,svelte}"],
      exclude: ["src/main.ts"],
      thresholds: {
        lines: 80,
        functions: 80,
        branches: 80,
        statements: 80,
      },
    },
  },
  build: {
    outDir: "../internal/web/dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": process.env.MEGA_AGENTS_API ?? "http://localhost:8080",
    },
  },
});
