import { expect, test, type Page } from "@playwright/test";

async function dragSplitter(page: Page, name: string, dx: number, dy: number) {
  const box = await page.getByRole("separator", { name }).boundingBox();
  if (!box) throw new Error(`${name} is not visible`);
  const x = box.x + box.width / 2;
  const y = box.y + box.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x + dx, y + dy, { steps: 6 });
  await page.mouse.up();
}

const widthOf = async (page: Page, name: string) =>
  (await page.getByRole("complementary", { name }).boundingBox())?.width ?? 0;
const outputHeight = async (page: Page) =>
  (await page.getByRole("region", { name: "Output" }).boundingBox())?.height ??
  0;

test("the sidebars and the output panel resize and stay resized", async ({
  page,
}) => {
  await page.goto("/");
  const palette = await widthOf(page, "Component palette");
  const properties = await widthOf(page, "Node properties");
  const output = await outputHeight(page);

  await dragSplitter(page, "Resize components sidebar", 80, 0);
  await dragSplitter(page, "Resize properties sidebar", -60, 0);
  await dragSplitter(page, "Resize output panel", 0, -100);

  const resized = async () => [
    await widthOf(page, "Component palette"),
    await widthOf(page, "Node properties"),
    await outputHeight(page),
  ];
  const expected = [palette + 80, properties + 60, output + 100];
  const actual = await resized();
  actual.forEach((size, index) =>
    expect(size).toBeCloseTo(expected[index]!, 0),
  );
  await expect(
    page.getByRole("separator", { name: "Resize output panel" }),
  ).toHaveCSS("cursor", "row-resize");

  await page.reload();
  const reloaded = await resized();
  reloaded.forEach((size, index) =>
    expect(size).toBeCloseTo(expected[index]!, 0),
  );
});
