import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test } from "@playwright/test";
import {
  canvas,
  connect,
  dropInto,
  enlarge,
  mouseDrag,
  selectBlock,
} from "./fixtures";

test.describe("Loop blocks", () => {
  // A project holding a loop that holds two blocks needs a tall canvas.
  test.use({ viewport: { width: 1440, height: 1100 } });
  let folder: string;

  test.beforeEach(async ({ page }) => {
    folder = mkdtempSync(join(tmpdir(), "mega-agents-loop-"));
    await page.goto("/");
  });

  test.afterEach(() => {
    rmSync(folder, { recursive: true, force: true });
  });

  test("repeats a gate and its fixer until the gate passes", async ({
    page,
  }) => {
    await mouseDrag(
      page,
      page.getByRole("button", { name: "Project" }),
      canvas(page),
    );
    const project = page.getByRole("button", { name: "Project 1" });
    await page.getByLabel("Path").fill(folder);
    await enlarge(page, project, 560, 380);
    const coder = await dropInto(
      page,
      "Agent",
      project,
      { x: 90, y: 90 },
      "Coder",
    );
    await page.getByLabel("Starting point").check();
    await page.getByLabel("Prompt").fill("Write the feature");
    const loop = await dropInto(
      page,
      "Loop",
      project,
      { x: 60, y: 170 },
      "Until green",
    );
    const gate = await dropInto(
      page,
      "Command",
      loop,
      { x: 90, y: 90 },
      "Gate",
    );
    // Fails on its first run, passes on its second.
    await page
      .getByLabel("Command", { exact: true })
      .fill(
        'n=$(( $(cat runs 2>/dev/null || echo 0) + 1 )); echo $n > runs; echo "run $n"; [ $n -ge 2 ]',
      );
    const fixer = await dropInto(
      page,
      "Agent",
      loop,
      { x: 300, y: 150 },
      "Fixer",
    );
    await page.getByLabel("Prompt").fill("Fix {{result}}");
    await connect(page, gate, fixer);
    await selectBlock(gate);
    await page.getByLabel("Output to Fixer").selectOption("failed");
    await connect(page, coder, loop);
    await selectBlock(loop);
    await page
      .getByLabel("Ends when")
      .selectOption({ label: "Gate takes passed" });

    await expect(
      page.getByRole("tabpanel", { name: /Problems/ }),
    ).toContainText("No problems found");
    await page.getByRole("button", { name: "Run flow" }).click();

    const result = page.getByRole("region", { name: "Run result" });
    await expect(result).toContainText("Run succeeded", { timeout: 15_000 });
    await expect(result).toContainText("Iterations: 2");
    await expect(result).toContainText(/Gate: command\s+succeeded\s+repeat 2/);
    await expect(result).toContainText(/Fixer: agent\s+skipped\s+repeat 2/);
    await expect(loop.locator(".repeats")).toHaveText(/2 of 3/);
    await expect(loop).toHaveClass(/status-succeeded/);

    await result.getByRole("button", { name: "Logs of Gate" }).click();
    const log = page.getByRole("dialog");
    await expect(log).toContainText("🔁 Iteration 1 of 3");
    await expect(log).toContainText("🔁 Iteration 2 of 3");
    await expect(log).toContainText("run 2");
  });
});
