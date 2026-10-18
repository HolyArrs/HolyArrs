import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { spawnSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { oldKey, oldRadarrPort, oldSeerrPort } from "./fakes.mjs";
import { moviesDir, noSidewaysScroll, owner, signIn } from "./helpers.ts";

// Move from other apps (6.2.2): read a fake old Radarr and Seerr, see the
// plan, take the suggested path rule, try it, move, read the report, see
// the movie adopted in place, a second try adds nothing, undo (a Job); a phone and
// accessibility.
test.describe.configure({ mode: "serial" });

let page: Page;
const movieFolder = join(moviesDir, "Lodestone Ferry (2010)");

test.beforeAll(async ({ browser }) => {
  mkdirSync(movieFolder, { recursive: true });
  writeFileSync(
    join(movieFolder, "Lodestone.Ferry.2010.1080p.mkv"),
    "stand-in video ".repeat(64),
  );
  const ctx = await browser.newContext({
    viewport: { width: 1280, height: 900 },
  });
  page = await ctx.newPage();
  page.on("dialog", (d) => d.accept());
  await signIn(page, owner());
});

async function readApp(app: string, port: number) {
  await page.getByLabel("App", { exact: true }).selectOption(app);
  await page
    .getByLabel("Address", { exact: true })
    .fill(`http://127.0.0.1:${port}`);
  await page.getByLabel("API key", { exact: true }).fill(oldKey);
  await page.getByRole("button", { name: /^Read / }).click();
}

test("read two old apps, see the plan, try it and move", async () => {
  await page.goto("/settings");
  await page.getByRole('main').getByRole('link').filter({ hasText: "Move from other apps" }).click();
  await expect(
    page.getByRole("heading", { name: "Move from other apps", level: 1 }),
  ).toBeVisible();

  // A wrong key is refused in plain words, and never shown back.
  await page
    .getByLabel("Address", { exact: true })
    .fill(`http://127.0.0.1:${oldRadarrPort}`);
  await page.getByLabel("API key", { exact: true }).fill("wrong-key-here");
  await page.getByRole("button", { name: "Read Radarr" }).click();
  await expect(
    page.getByRole("alert").filter({ hasText: "refused the key" }),
  ).toBeVisible();
  await expect(page.getByLabel("API key", { exact: true })).toHaveValue("");

  await readApp("radarr", oldRadarrPort);
  const read = page.getByRole("list", { name: "Read apps" });
  await expect(read.getByText("Radarr", { exact: true })).toBeVisible();
  await readApp("seerr", oldSeerrPort);
  await expect(read.getByText("Seerr", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "Make the plan" }).click();
  const plan = page.getByTestId("plan");
  await expect(plan).toContainText("Titles");
  await expect(page.getByTestId("not-moved-count")).toBeVisible();
  // The old path is not here: HolyArrs suggests the Library folder of the same name.
  await page.getByRole("button", { name: /Use 1 suggested rule/ }).click();
  await expect(page.getByLabel("Old app's path", { exact: true })).toHaveValue(
    "/old/movies",
  );
  await expect(plan).toContainText("Lodestone Ferry");
  // The old app's owner is the owner here only when confirmed.
  await plan
    .locator("summary")
    .filter({ hasText: /^People/ })
    .click();
  await plan
    .getByRole("checkbox", { name: /This is me/ })
    .first()
    .check();
  await expect(plan).toContainText("is you");

  await page.getByRole("button", { name: "Try it" }).click();
  await expect(page.getByTestId("dry-summary")).toContainText(
    "will be adopted in place",
  );

  await page.getByRole("button", { name: "Approve and move" }).click();
  const report = page.getByTestId("move-report");
  await expect(report).toContainText("done", { timeout: 20_000 });
  await report
    .locator("summary")
    .filter({ hasText: /^Titles/ })
    .click();
  await expect(report).toContainText("Adopted in place");
  await expect(report).toContainText("Needs your input");
  await expect(
    report.getByRole("link", { name: "Download the report" }),
  ).toHaveAttribute("href", /\/api\/v1\/migrate\/moves\/\d+\/report/);
  const dl = await page.request.get(
    (await report
      .getByRole("link", { name: "Download the report" })
      .getAttribute("href")) ?? "",
  );
  expect(dl.status()).toBe(200);
  expect(await dl.text()).toContain("Lodestone Ferry");

  // The movie is in the Library, adopted from its folder.
  await page.goto("/library");
  await expect(page.getByText("Lodestone Ferry").first()).toBeVisible();
});

test("a second try adds nothing, and undo takes the move back", async () => {
  await page.goto("/settings/move");
  const savedReads = page
    .getByRole("list", { name: "Read apps" })
    .getByRole("checkbox");
  await expect(savedReads).toHaveCount(2);
  for (const box of await savedReads.all())
    await box.check();
  await page.getByRole("button", { name: "Make the plan" }).click();
  await page.getByRole("button", { name: /Use 1 suggested rule/ }).click();
  await page.getByRole("button", { name: "Try it" }).click();
  await expect(page.getByTestId("dry-summary")).toContainText("already here");
  await expect(page.getByTestId("dry-summary")).not.toContainText(
    "will be adopted",
  );

  await page
    .getByRole("button", { name: /^Undo move \d+/ })
    .first()
    .click();
  await expect(page.getByText(/Undone:/)).toBeVisible();
  const r = await page.request.get("/api/v1/titles");
  const titles = ((await r.json()) as { titles: { name: string }[] }).titles;
  expect(titles.some((t) => t.name === "Lodestone Ferry")).toBe(false);
});

// 3.5.13: `holyarrs import-secrets` while HolyArrs runs goes through its
// API with an owner key; the old Radarr's key is saved for the Move tool,
// offered without being shown, used for one read and then forgotten.
test("import a made-up secrets file, then read Radarr with the saved key", async () => {
  const tmp = join(dirname(fileURLToPath(import.meta.url)), ".tmp");
  const me = (await (await page.request.get("/api/v1/auth/me")).json()) as {
    csrf: string;
  };
  const made = await page.request.post("/api/v1/auth/keys", {
    headers: { "X-CSRF-Token": me.csrf, "Content-Type": "application/json" },
    data: { label: "import", scopes: ["admin"] },
  });
  const ownerKey = ((await made.json()) as { key: string }).key;
  const origin = new URL(page.url()).origin;
  const file = join(tmp, "made-up-secrets.yaml");
  writeFileSync(
    file,
    [
      "version: 1",
      "radarr:",
      `  address: http://127.0.0.1:${oldRadarrPort}`,
      `  api_key: ${oldKey}`,
      "  allow_local_network: true",
      "holyarrs:",
      `  address: ${origin}`,
      `  owner_api_key: ${ownerKey}`,
      "",
    ].join("\n"),
    { mode: 0o600 },
  );
  const bin = join(
    tmp,
    process.platform === "win32" ? "holyarrs.exe" : "holyarrs",
  );
  const run = spawnSync(
    bin,
    ["import-secrets", "-file", file, "-config", join(tmp, "config")],
    { encoding: "utf8" },
  );
  const said = run.stdout + run.stderr;
  expect(run.status, said).toBe(0);
  expect(said).toContain("mode: through the running HolyArrs");
  expect(said).toMatch(/radarr \(old app\)\s+set/);
  expect(said).not.toContain(oldKey);
  expect(said).not.toContain(ownerKey);

  await page.goto("/settings/move");
  await page.getByLabel("App", { exact: true }).selectOption("radarr");
  const use = page.getByTestId("use-saved-key");
  await expect(use).toContainText("Use the saved key for Radarr");
  await expect(page.getByLabel("API key", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Read Radarr" }).click();
  await expect(use).toHaveCount(0);
  await expect(page.getByLabel("API key", { exact: true })).toBeVisible();
  const left = await page.request.get("/api/v1/migrate/saved-keys");
  expect(await left.text()).not.toContain("radarr");
  expect(await page.content()).not.toContain(oldKey);
});

test("the move screen at phone width, and accessibility", async ({
  browser,
}) => {
  const ctx = await browser.newContext({
    viewport: { width: 412, height: 915 },
    isMobile: true,
    hasTouch: true,
  });
  const phone = await ctx.newPage();
  await signIn(phone, owner());
  await phone.goto("/settings/move");
  await expect(
    phone.getByRole("heading", { name: "Move from other apps", level: 1 }),
  ).toBeVisible();
  await phone.getByRole("button", { name: "Add a path rule" }).click();
  await noSidewaysScroll(phone);
  const a11y = await new AxeBuilder({ page: phone }).include("main").analyze();
  expect(
    a11y.violations.map(
      (v) => `${v.id}: ${v.nodes.map((n) => n.target).join(" ")}`,
    ),
  ).toEqual([]);
  const desktop = await new AxeBuilder({ page }).include("main").analyze();
  expect(desktop.violations.map((v) => v.id)).toEqual([]);
  await ctx.close();
});
