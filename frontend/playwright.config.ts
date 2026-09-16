import { defineConfig } from "@playwright/test";

// Browser smoke suite for drag-and-drop, which jsdom cannot exercise. Kept
// out of `make verify`: run explicitly with `bun run e2e`. Reuses a running
// dev server (e.g. `make dev`); CI wiring is tracked in docs/ci-doc.md.
export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  use: {
    baseURL: "http://localhost:5173/",
    browserName: "chromium",
  },
  webServer: {
    command: "bun run dev -- --host",
    url: "http://localhost:5173/",
    reuseExistingServer: !process.env.CI,
  },
});
