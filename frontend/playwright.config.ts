import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { defineConfig } from "@playwright/test";

// Browser suite for drag-and-drop, which jsdom cannot exercise, and for flows
// that run on the real Go backend. Kept out of `make verify`: run explicitly
// with `bun run e2e`. Both servers use dedicated ports so a running `make dev`
// session is never reused, and the backend keeps its worktrees in a temporary
// Mega Agents home.
const backendPort = 48_080;
const frontendPort = 45_174;

export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  use: {
    baseURL: `http://localhost:${frontendPort}/`,
    browserName: "chromium",
  },
  webServer: [
    {
      command: "go run .",
      cwd: "..",
      url: `http://localhost:${backendPort}/api/status`,
      env: {
        PORT: String(backendPort),
        MEGA_AGENTS_HOME: mkdtempSync(join(tmpdir(), "mega-agents-home-")),
      },
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: `bun run dev -- --host --port ${frontendPort} --strictPort`,
      url: `http://localhost:${frontendPort}/`,
      env: { MEGA_AGENTS_API: `http://localhost:${backendPort}` },
      reuseExistingServer: false,
    },
  ],
});
