import AxeBuilder from "@axe-core/playwright";
import {
  expect,
  test,
  type Browser,
  type BrowserContext,
  type Page,
  type Route,
} from "@playwright/test";
import { spawnSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { oldKey, oldRadarrPort, oldSeerrPort } from "../fakes/index.mjs";
import { moviesDir, noSidewaysScroll, owner, signIn } from "../helpers/helpers.ts";

// Move from other apps (6.2.2), in four steps: read a fake old Radarr and
// Seerr, choose what to move, move (a quiet test run first; problems open
// "Before you import"), done, undo. Then the variants (qBittorrent sign-in,
// backup file, saved key, Forget, Stop reading), the choices the server asks
// for, the straight-through move, poll errors, stale tests, ?move=N, old
// ?step=try addresses, no confirm() anywhere, sideways scroll, measured
// symmetry and a 2,000-title list, at 360, 390 and 1280.
test.describe.configure({ mode: "serial" });

let page: Page;
let dialogs: string[] = [];
const movieFolder = join(moviesDir, "Lodestone Ferry (2010)");

function watchDialogs(p: Page) {
  // No confirm() anywhere: every question is a Sheet.
  p.on("dialog", (d) => {
    dialogs.push(d.message());
    void d.dismiss();
  });
}

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
  watchDialogs(page);
  await signIn(page, owner());
});

test.afterEach(() => {
  expect(dialogs, "a native dialog opened").toEqual([]);
  dialogs = [];
});

const primary = (p: Page) => p.locator("#move-primary");
const reason = (p: Page) => p.locator("#move-reason");
const current = (p: Page) =>
  p.getByTestId("move-steps").locator('[aria-current="step"]');
/** Opens "Change what's imported" on Choose (the full list), if it is closed. */
async function openAll(p: Page) {
  const b = p.getByRole("button", { name: "Change what's imported" });
  await expect(b).toBeVisible();
  if ((await b.getAttribute("aria-expanded")) !== "true") await b.click();
  await expect(b).toHaveAttribute("aria-expanded", "true");
}
const readSheet = (p: Page) =>
  p.getByRole("dialog", { name: /^(Add an app|Edit .+)$/ });

async function addApp(p: Page, app: string, port: number, key = oldKey) {
  await p.getByRole("button", { name: "Add an app" }).first().click();
  const sheet = readSheet(p);
  await expect(sheet).toBeVisible();
  await sheet.getByLabel("App", { exact: true }).selectOption(app);
  await sheet
    .getByLabel("Address", { exact: true })
    .fill(`http://127.0.0.1:${port}`);
  await sheet.getByLabel("API key", { exact: true }).fill(key);
  return sheet;
}

/** Each card's .srow > .ctl right edges line up within 1px; rows are at least 52px tall. */
async function measuredSymmetry(p: Page) {
  const cards = await p.evaluate(() =>
    [...document.querySelectorAll("main .ui-settings")].map((card) => {
      const rows = [...card.querySelectorAll(":scope .srow")].filter(
        (r) => !r.closest("dialog") && (r as HTMLElement).offsetParent,
      );
      const stacked = (r: Element) =>
        getComputedStyle(r).gridTemplateColumns.split(" ").length < 2;
      return {
        title: card.querySelector("h2")?.textContent ?? "",
        rights: rows
          .filter((r) => !stacked(r))
          .map((r) => r.querySelector(":scope > .ctl"))
          .filter((c): c is Element => !!c && c.children.length > 0)
          .map((c) => Math.round(c.getBoundingClientRect().right)),
        switches: rows
          .map((r) => r.querySelector(":scope > .ctl > .switch:last-child"))
          .filter((s): s is Element => !!s)
          .map((s) => Math.round(s.getBoundingClientRect().left)),
        heights: rows.map((r) => Math.round(r.getBoundingClientRect().height)),
      };
    }),
  );
  for (const c of cards) {
    if (c.rights.length > 1)
      expect(
        Math.max(...c.rights) - Math.min(...c.rights),
        `${c.title}: right column`,
      ).toBeLessThanOrEqual(1);
    if (c.switches.length > 1)
      expect(
        Math.max(...c.switches) - Math.min(...c.switches),
        `${c.title}: switches`,
      ).toBeLessThanOrEqual(1);
    for (const h of c.heights)
      expect(h, `${c.title}: row height`).toBeGreaterThanOrEqual(52);
  }
}

test("full flow: read, choose, move (with Before you move), done and undo", async () => {
  await page.goto("/settings/move");
  await expect(
    page.getByRole("heading", { name: "Import from other apps", level: 1 }),
  ).toBeVisible();
  await expect(current(page)).toHaveAttribute("aria-label", /Step 1 of 4, Read/);
  // Empty: one primary, no footer.
  await expect(
    page.getByRole("heading", { name: "Bring your old apps over" }),
  ).toBeVisible();
  await expect(primary(page)).toHaveCount(0);

  // A wrong key is refused in plain words, inside the sheet, and never shown back.
  let sheet = await addApp(page, "radarr", oldRadarrPort, "wrong-key-here");
  await sheet.getByRole("button", { name: "Add Radarr" }).click();
  await expect(
    sheet.getByRole("alert").filter({ hasText: "refused the key" }),
  ).toBeVisible();
  await expect(sheet.getByLabel("API key", { exact: true })).toHaveValue("");
  await noSidewaysScroll(page);

  // Test answers in seconds, then the read.
  await sheet.getByLabel("API key", { exact: true }).fill(oldKey);
  await sheet.getByRole("button", { name: "Test", exact: true }).click();
  await expect(sheet.getByTestId("probe-ok")).toHaveText("Radarr v6.4.4");
  await sheet.getByRole("button", { name: "Add Radarr" }).click();
  await expect(sheet).toBeHidden();
  const rows = page.getByTestId("read-row");
  await expect(rows.filter({ hasText: "Radarr" })).toBeVisible();
  // Counts in our words: no raw keys.
  await expect(rows.first()).not.toContainText("_");
  await expect(rows.first()).toContainText("1 movie");

  sheet = await addApp(page, "seerr", oldSeerrPort);
  await sheet.getByRole("button", { name: "Add Seerr" }).click();
  await expect(sheet).toBeHidden();
  await expect(rows.filter({ hasText: "Seerr" })).toBeVisible();
  await measuredSymmetry(page);

  // Step 2: the plan makes itself.
  await primary(page).click();
  await expect(current(page)).toHaveAttribute("aria-label", /Step 2 of 4, Choose/);
  await expect(page).toHaveURL(/step=choose&read=\d+,\d+/);
  await openAll(page);
  const groups = page.getByTestId("group-row");
  await expect(groups.filter({ hasText: "Titles" })).toBeVisible();
  // The old folder is not here: the Library folder of the same name is suggested.
  const folder = page.getByTestId("folder-row").first();
  await expect(folder).toContainText("/old/movies");
  await expect(folder).toContainText("Suggested");
  await expect(primary(page)).toBeEnabled();

  // Browser Back goes to the step before, Forward comes back.
  await page.goBack();
  await expect(current(page)).toHaveAttribute("aria-label", /Step 1 of 4/);
  await page.goForward();
  await expect(current(page)).toHaveAttribute("aria-label", /Step 2 of 4/);
  await openAll(page);

  // A group switched off stays in place, marked Left out.
  const alerts = page.getByRole("checkbox", { name: "Import Alerts" });
  await alerts.uncheck();
  await expect(groups.filter({ hasText: "Alerts" })).toContainText("Left out");
  await alerts.check();
  await expect(groups.filter({ hasText: "Alerts" })).not.toContainText("Left out");

  // A bad folder: the error is on its own row and Move waits.
  const pick = folder.locator("select");
  const suggested = await pick.inputValue();
  await pick.selectOption({ label: "Other path…" });
  const other = folder.getByLabel(/^Path here for/);
  await other.fill("relative/path");
  await other.press("Enter");
  await expect(folder.locator(".form-error")).toContainText("full paths");
  await expect(primary(page)).toBeDisabled();
  await expect(reason(page)).toHaveText("Fix the folder marked above.");
  await pick.selectOption(suggested);
  await expect(folder.locator(".form-error")).toHaveCount(0);
  await expect(primary(page)).toBeEnabled();

  // The old owner: This is me can be turned on and off again.
  const me = page.getByRole("checkbox", { name: /^This is me/ });
  // The only owner starts as "This is me".
  await expect(me).toBeChecked();
  await me.check();
  await expect(me).toBeChecked();
  await me.uncheck();
  await expect(me).not.toBeChecked();
  await noSidewaysScroll(page);
  await measuredSymmetry(page);

  // Move: a quiet test run first (the button is busy, no step of its own).
  await expect(primary(page)).toBeEnabled();
  await expect(primary(page)).toHaveText("Import");
  const tested = page.waitForRequest("**/api/v1/migrate/dry-run");
  await primary(page).click();
  await tested;
  // Same name as the owner here: a choice, so "Before you import" opens.
  const before = page.getByRole("dialog", { name: "Before you import" });
  await expect(before).toBeVisible({ timeout: 20_000 });
  await expect(current(page)).toHaveAttribute("aria-label", /Step 2 of 4, Choose/);
  await expect(before.getByRole("button", { name: "Import anyway" })).toBeVisible();
  // Go back closes it and stays on Choose.
  await before.getByRole("button", { name: "Go back" }).click();
  await expect(before).toBeHidden();
  await expect(current(page)).toHaveAttribute("aria-label", /Step 2 of 4, Choose/);
  await primary(page).click();
  await expect(before).toBeVisible({ timeout: 20_000 });
  const choice = before.getByRole("region", { name: "Needs a choice" }).getByTestId("choice-row").first();
  await expect(choice).toContainText("Someone here has the same name");
  await expect(choice.getByRole("radio", { name: "Same person" })).toHaveAttribute("aria-checked", "false");
  await expect(choice.getByRole("radio", { name: "Leave out" })).toHaveAttribute("aria-checked", "false");
  await choice.getByRole("radio", { name: "Leave out" }).click();
  await expect(choice.getByRole("radio", { name: "Leave out" })).toHaveAttribute("aria-checked", "true");
  // The row stays after the choice, and the choice can go back.
  await choice.getByRole("radio", { name: "Same person" }).click();
  await expect(before.getByRole("region", { name: "Needs a choice" }).getByTestId("choice-row").first().getByRole("radio", { name: "Same person" })).toHaveAttribute("aria-checked", "true");
  await noSidewaysScroll(page);
  await expect(before.getByRole("button", { name: "Import anyway" })).toBeEnabled({ timeout: 20_000 });

  // Move anyway starts it; follow the progress.
  await before.getByRole("button", { name: "Import anyway" }).click();
  const seen: number[] = [];
  const progress = page.getByRole("progressbar", { name: "Import progress" });
  await expect(async () => {
    if (await progress.isVisible())
      seen.push(Number(await progress.getAttribute("aria-valuenow")));
    await expect(page.getByTestId("done-result")).toContainText("Imported");
  }).toPass({ timeout: 20_000 });
  for (let i = 1; i < seen.length; i++)
    expect(seen[i]).toBeGreaterThanOrEqual(seen[i - 1]);

  // Step 4: what to do now, results, the report.
  await expect(current(page)).toHaveAttribute("aria-label", /Step 4 of 4, Done/);
  await expect(page).toHaveURL(/move=\d+/);
  await page.getByRole("button", { name: "Show Titles" }).click();
  await expect(page.getByRole("dialog", { name: "Titles" })).toContainText("Using the one here");
  await page.getByRole("dialog", { name: "Titles" }).getByRole("button", { name: "Close", exact: true }).click();
  const report = page.getByRole("link", { name: "Download the report" });
  await expect(report).toHaveAttribute("href", /\/api\/v1\/migrate\/moves\/\d+\/report/);
  const dl = await page.request.get((await report.getAttribute("href")) ?? "");
  expect(dl.status()).toBe(200);
  expect(await dl.text()).toContain("Lodestone Ferry");
  await noSidewaysScroll(page);

  // The movie is in the Library, in its folder.
  const titles = (await (await page.request.get("/api/v1/titles")).json()) as { titles: { name: string }[] };
  expect(titles.titles.some((t) => t.name === "Lodestone Ferry")).toBe(true);

  // Undo, after a confirm.
  await page.getByRole("button", { name: "Undo this import" }).click();
  const undo = page.getByRole("dialog", { name: /^Undo import \d+\?$/ });
  await expect(undo).toContainText("Download the report first");
  await undo.getByRole("button", { name: "Undo import" }).click();
  await expect(page.getByTestId("done-result")).toContainText("Undone", { timeout: 20_000 });
  const after = (await (await page.request.get("/api/v1/titles")).json()) as { titles: { name: string }[] };
  expect(after.titles.some((t) => t.name === "Lodestone Ferry")).toBe(false);

  // Done resets to step 1; the reads that moved start off.
  await primary(page).click();
  await expect(current(page)).toHaveAttribute("aria-label", /Step 1 of 4/);
  await expect(page.getByTestId("earlier-move").first()).toContainText("Undone");
});

test("old addresses land on Choose; a change while testing runs it again; a changed plan checks again", async () => {
  await page.goto("/settings/move");
  const snaps = (await (await page.request.get("/api/v1/migrate/snapshots")).json()) as { snapshots: { id: number }[] };
  const ids = snaps.snapshots.map((s) => s.id);
  // An address that names Move without a move, or the old Try it, lands on Choose.
  await page.goto(`/settings/move?step=move&read=${ids.join(",")}`);
  await expect(current(page)).toHaveAttribute("aria-label", /Step 2 of 4, Choose/);
  await page.goto(`/settings/move?step=try&read=${ids.join(",")}`);
  await expect(current(page)).toHaveAttribute("aria-label", /Step 2 of 4, Choose/);
  await expect(page).toHaveURL(/step=choose/);
  await expect(page.getByTestId("move-steps").locator("li")).toHaveCount(4);

  // Change a group, then Move: the quiet test is held, the button is busy and no sheet opens.
  let release: () => void = () => {};
  const held = new Promise<void>((r) => (release = r));
  await page.route("**/api/v1/migrate/dry-run", async (route: Route) => {
    await held;
    // The page may cancel a held run when it starts a newer one.
    await route.continue().catch(() => {});
  });
  await openAll(page);
  await page.getByRole("checkbox", { name: "Import Alerts" }).uncheck();
  await expect(primary(page)).toBeEnabled();
  const asked = page.waitForRequest("**/api/v1/migrate/dry-run");
  await primary(page).click();
  await asked;
  await expect(primary(page)).toHaveAttribute("aria-busy", "true");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  release();
  await page.unroute("**/api/v1/migrate/dry-run");
  const sheet = page.getByRole("dialog", { name: /^(Before you import|Import \d+ items?\?)$/ });
  await expect(sheet).toBeVisible({ timeout: 20_000 });
  await expect(current(page)).toHaveAttribute("aria-label", /Step 2 of 4, Choose/);

  // The server says the plan changed: it tests again and asks again, still on Choose.
  await page.route("**/api/v1/migrate/moves", (route: Route) =>
    route.request().method() === "POST"
      ? route.fulfill({ status: 409, json: { error: "the plan changed since its dry run; dry run again" } })
      : route.continue(),
  );
  const again = page.waitForRequest("**/api/v1/migrate/dry-run");
  await sheet.getByRole("button", { name: /^(Import anyway|Import)$/ }).click();
  await again;
  await expect(page.getByRole("dialog").getByText("Something changed, so HolyArrs checked again.")).toBeVisible({ timeout: 20_000 });
  await expect(current(page)).toHaveAttribute("aria-label", /Step 2 of 4, Choose/);
  await page.unroute("**/api/v1/migrate/moves");
  await page.keyboard.press("Escape");
});

test("variants: qBittorrent sign-in, backup file, Stop reading and Forget", async () => {
  await page.goto("/settings/move");
  await expect(page.getByRole("button", { name: "Add an app" }).first()).toBeVisible();
  const before = await page.getByTestId("read-row").count();

  // qBittorrent asks for a user name and a password (or its API key).
  await page.getByRole("button", { name: "Add an app" }).first().click();
  const sheet = readSheet(page);
  await sheet.getByLabel("App", { exact: true }).selectOption("qbittorrent");
  await expect(sheet.getByLabel("Address", { exact: true })).toHaveAttribute("placeholder", /:8080$/);
  await sheet.getByLabel("Address", { exact: true }).fill("http://127.0.0.1:9");
  await sheet.getByRole("button", { name: "Add qBittorrent" }).click();
  await expect(sheet.locator("#move-user:invalid")).toHaveCount(1);
  await expect(sheet.locator("#move-pass:invalid")).toHaveCount(1);

  await sheet.getByLabel("App", { exact: true }).selectOption("radarr");
  await noSidewaysScroll(page);

  // Stop reading: the request is aborted and no read is added.
  await sheet.getByLabel("Address", { exact: true }).fill(`http://127.0.0.1:${oldRadarrPort}`);
  await sheet.getByLabel("API key", { exact: true }).fill(oldKey);
  await page.route("**/api/v1/migrate/read", () => {
    // Never answers: only Stop reading ends it.
  });
  await sheet.getByRole("button", { name: "Add Radarr" }).click();
  await expect(sheet.getByTestId("read-capsule")).toContainText("Adding Radarr");
  await sheet.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(sheet.getByTestId("read-capsule")).toHaveCount(0);
  await page.unroute("**/api/v1/migrate/read");
  await sheet.getByRole("button", { name: "Cancel" }).click();
  await expect(page.getByTestId("read-row")).toHaveCount(before);

  // Forget asks in a Sheet first.
  await page.getByRole("button", { name: /^Forget the .+ read from/ }).last().click();
  const forget = page.getByRole("dialog", { name: /^Forget this .+ read\?$/ });
  await expect(forget).toContainText("Finished imports stay.");
  await forget.getByRole("button", { name: "Cancel" }).click();
  await expect(page.getByTestId("read-row")).toHaveCount(before);
  // Let the sheet finish closing before it opens again.
  await expect(forget).toBeHidden();
  await page.getByRole("button", { name: /^Forget the .+ read from/ }).last().click();
  await forget.getByRole("button", { name: "Remove", exact: true }).click();
  await expect(page.getByTestId("read-row")).toHaveCount(before - 1);
});

// 3.5.13: `holyarrs import-secrets` while HolyArrs runs goes through its
// API with an owner key; the old Radarr's key is saved for the Move tool,
// offered without being shown, and kept for Read again until a move
// finishes (the key is kept until the move is done).
test("import a made-up secrets file, then read Radarr with the saved key", async () => {
  const tmp = join(dirname(fileURLToPath(import.meta.url)), "..", ".tmp");
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
  await expect(page.getByRole("button", { name: "Add an app" }).first()).toBeVisible();
  const before = await page.getByTestId("read-row").count();
  await page.getByRole("button", { name: "Add an app" }).first().click();
  const sheet = readSheet(page);
  await sheet.getByLabel("App", { exact: true }).selectOption("radarr");
  const use = sheet.getByTestId("use-saved-key");
  await expect(use).toContainText(`Sent only to http://127.0.0.1:${oldRadarrPort}`);
  // The saved address is plain text; no key field, no local-network switch.
  await expect(sheet.getByTestId("saved-address")).toBeVisible();
  await expect(sheet.getByLabel("Address", { exact: true })).toHaveCount(0);
  await expect(sheet.getByLabel("API key", { exact: true })).toHaveCount(0);
  await expect(sheet.locator("#move-local")).toHaveCount(0);
  await sheet.getByRole("button", { name: "Add Radarr" }).click();
  await expect(sheet).toBeHidden();
  // One row per app: reading Radarr again keeps one Radarr row.
  await expect(page.getByTestId("read-row").filter({ hasText: "Radarr" })).toHaveCount(1);
  const left = await page.request.get("/api/v1/migrate/saved-keys");
  expect(await left.text()).toContain(`"kind":"radarr"`);
  expect(await left.text()).not.toContain(oldKey);

  // Read again: the app is locked, the kept key is used without asking.
  await expect(sheet).toBeHidden();
  await page.getByRole("button", { name: "Edit Radarr" }).first().click();
  const again = readSheet(page);
  await expect(again).toBeVisible();
  await expect(again.getByLabel("App", { exact: true })).toBeDisabled();
  await expect(again.getByTestId("use-saved-key")).toHaveCount(0);
  await expect(again.getByTestId("saved-address")).toHaveText(`http://127.0.0.1:${oldRadarrPort}`);
  await expect(again).toContainText("Saved key");
  await expect(again.getByLabel("API key", { exact: true })).toHaveCount(0);
  // A different key is one quiet link away; Cancel leaves it as it was.
  await again.getByRole("button", { name: "Use a different key" }).click();
  await expect(again.getByLabel("API key", { exact: true })).toBeVisible();
  await again.getByRole("button", { name: "Cancel" }).click();
  await expect(again).toBeHidden();
  await page.getByRole("button", { name: "Edit Radarr" }).first().click();
  await expect(again.getByTestId("saved-address")).toBeVisible();
  await again.getByRole("button", { name: "Save", exact: true }).click();
  await expect(again).toBeHidden();
  // One row per app: reading Radarr again keeps one Radarr row.
  await expect(page.getByTestId("read-row").filter({ hasText: "Radarr" })).toHaveCount(1);
  expect(await (await page.request.get("/api/v1/migrate/saved-keys")).text()).toContain(`"kind":"radarr"`);
  expect(await page.content()).not.toContain(oldKey);
});

// Fixtures for the steps the fakes cannot reach on demand: a person the
// server cannot match by number, a move that is still running, poll errors
// and a 2,000-title read. Every migrate call is answered here.
type Fixture = {
  moveAnswers?: (n: number) => { status: number; json: unknown };
  titles?: number;
  /** No person to choose and nothing missing: Move goes straight through. */
  clean?: boolean;
  /** Titles whose folder is not found here. */
  missing?: number;
};
function line(group: string, key: string, name: string, result: string, more: Record<string, unknown> = {}) {
  return { group, key, name, result, ...more };
}
async function fixture(p: Page, f: Fixture = {}) {
  const snap = { id: 901, created_at: "2026-11-03T14:02:00Z", kind: "radarr", via: "api", app_version: "6.4.4", counts: { titles: f.titles ?? 3, indexers: 4, download_clients: 1 }, size: 1000 };
  const seerr = { id: 902, created_at: "2026-11-03T14:05:00Z", kind: "seerr", via: "api", app_version: "2.7.0", counts: { users: 2, requests: 1 }, size: 100 };
  const items = Array.from({ length: f.titles ?? 3 }, (_, i) => ({ key: `title:movie:${i}`, name: `Title ${i}`, from: `radarr titles[${i}]`, detail: `uses the folder /data/movies/Title ${i} where it is` }));
  const groups = ["folders", "pick_rules", "titles", "never_add", "outside_lists", "sources", "downloaders", "naming", "subtitles", "people", "wishlist", "alerts"].map((name) => ({
    name,
    label: { folders: "Library folders", pick_rules: "Pick rules", titles: "Titles", never_add: "Never-add list", outside_lists: "Outside lists", sources: "Sources", downloaders: "Downloaders", naming: "Naming", subtitles: "Subtitles", people: "People", wishlist: "Requests", alerts: "Alerts" }[name],
    skipped: false,
    items: name === "titles" ? items : name === "people" ? [{ key: "person:seerr:7", name: "Pia", from: "seerr users[1]", person: { role: "member" } }] : [],
  }));
  const plan = { snapshots: [901, 902], apps: ["radarr", "seerr"], groups, not_moved: [], needs_input: ["radarr: indexer \"Old\": api key"], old_paths: [] };
  const dry = {
    dry: true,
    apps: ["radarr", "seerr"],
    at: "2026-11-03T14:10:00Z",
    lines: [
      ...items.map((it) => line("titles", it.key, it.name, "adopt")),
      ...Array.from({ length: f.missing ?? 0 }, (_, i) => line("titles", `title:movie:m${i}`, `Missing ${i}`, "not_found", { detail: `/old/movies/Missing ${i}` })),
      ...(f.clean ? [] : [line("people", "person:seerr:7", "Pia", "skipped", { detail: "HolyArrs can't tell if this is the same person. Choose Same person or Leave out.", choose: "numeric_id" })]),
    ],
    counts: f.clean ? { adopt: items.length } : { adopt: items.length, skipped: 1, ...(f.missing ? { not_found: f.missing } : {}) },
    not_moved: [],
    needs_input: [],
    next: ["Some people need a choice: pick Same person or Leave out for each, then try and import again.", "Keep the old apps until HolyArrs runs the way you want; this import changed nothing in them."],
  };
  const done = { id: 7, created_at: "2026-11-03T14:20:00Z", snapshots: [901], state: "done", report: { ...dry, dry: false, lines: dry.lines.map((l) => ({ ...l, result: l.result === "adopt" ? "adopted" : l.result })), counts: { adopted: items.length, skipped: 1 } } };
  let n = 0;
  await p.route(/\/api\/v1\/(migrate|library-folders)/, async (route: Route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^.*\/api\/v1/, "");
    const method = route.request().method();
    if (path === "/library-folders") return route.fulfill({ json: { library_folders: [{ path: "/data/movies", kind: "movie", label: "movies" }] } });
    if (path === "/migrate/snapshots") return route.fulfill({ json: { snapshots: [seerr, snap] } });
    if (path === "/migrate/saved-keys") return route.fulfill({ json: { saved: [] } });
    if (path === "/migrate/moves" && method === "GET") return route.fulfill({ json: { moves: [{ ...done, report: { counts: done.report.counts } }] } });
    if (path === "/migrate/plan") return route.fulfill({ json: { plan, counts: {}, suggested_path_rules: [], plan_hash: "h" } });
    if (path === "/migrate/dry-run") return route.fulfill({ json: { report: dry, plan_hash: "h" } });
    if (path === "/migrate/moves" && method === "POST") return route.fulfill({ json: { move: { id: 7, created_at: "2026-11-03T14:20:00Z", snapshots: [901, 902], state: "running" } } });
    if (path === "/migrate/moves/7") {
      const a = f.moveAnswers?.(n++) ?? { status: 200, json: { move: done, progress: 1, progress_note: "" } };
      return route.fulfill({ status: a.status, json: a.json });
    }
    if (path === "/migrate/moves/8") return route.fulfill({ json: { move: { ...done, id: 8 }, progress: 1, progress_note: "" } });
    return route.fulfill({ status: 404, json: { error: "not in this fixture" } });
  });
  return { done, dry };
}

async function phone(browser: Browser, width: number): Promise<{ ctx: BrowserContext; p: Page }> {
  const ctx = await browser.newContext({
    viewport: { width, height: 800 },
    isMobile: true,
    hasTouch: true,
  });
  const p = await ctx.newPage();
  watchDialogs(p);
  await signIn(p, owner());
  return { ctx, p };
}

test("a person the server cannot match by number gets Same person or Leave out in Before you move", async ({ browser }) => {
  const { ctx, p } = await phone(browser, 390);
  await fixture(p);
  // The old Try it address lands on Choose.
  await p.goto("/settings/move?step=try&read=901,902");
  await expect(current(p)).toHaveAttribute("aria-label", /Step 2 of 4, Choose/);
  await expect(primary(p)).toBeEnabled();
  await primary(p).click();
  const before = p.getByRole("dialog", { name: "Before you import" });
  await expect(before).toBeVisible();
  // Only problems are listed: no group lines here, just the choice.
  await expect(before.getByTestId("problem-row")).toHaveCount(0);
  const row = before.getByRole("region", { name: "Needs a choice" }).getByTestId("choice-row").filter({ hasText: "Pia" });
  await expect(row).toContainText("HolyArrs can't tell if this is the same person.");
  await expect(row.getByRole("radio", { name: "Same person" })).toBeVisible();
  await expect(row.getByRole("radio", { name: "Leave out" })).toBeVisible();
  await expect(before.getByRole("button", { name: "Import anyway" })).toBeEnabled();
  await noSidewaysScroll(p);
  await before.getByRole("button", { name: "Go back" }).click();
  await expect(before).toBeHidden();
  await expect(current(p)).toHaveAttribute("aria-label", /Step 2 of 4, Choose/);
  await ctx.close();
});

test("Before you move lists only the problem groups, one line each, with Show", async ({ browser }) => {
  const { ctx, p } = await phone(browser, 360);
  await fixture(p, { missing: 345 });
  await p.goto("/settings/move?step=choose&read=901,902");
  await expect(primary(p)).toBeEnabled();
  await primary(p).click();
  const before = p.getByRole("dialog", { name: "Before you import" });
  await expect(before).toBeVisible();
  const rows = before.getByTestId("problem-row");
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText("345 titles · no folder here");
  await rows.first().getByRole("button", { name: /^Show Titles/ }).click();
  const list = p.getByRole("dialog", { name: "Titles" });
  await expect(list).toContainText("Missing 0");
  await expect(list).not.toContainText("Title 0");
  await list.getByRole("button", { name: "Close", exact: true }).click();
  await expect(before.getByRole("button", { name: "Import anyway" })).toBeVisible();
  await expect(before.getByRole("button", { name: "Go back" })).toBeVisible();
  await noSidewaysScroll(p);
  await ctx.close();
});

test("no problems: Move tests quietly, confirms once and goes straight to the move", async ({ browser }) => {
  const { ctx, p } = await phone(browser, 390);
  await fixture(p, { clean: true });
  await p.goto("/settings/move?step=choose&read=901,902");
  await expect(primary(p)).toHaveText("Import");
  await expect(primary(p)).toBeEnabled();
  const tested = p.waitForRequest("**/api/v1/migrate/dry-run");
  await primary(p).click();
  await tested;
  const confirm = p.getByRole("dialog", { name: /^Import \d+ items?\?$/ });
  await expect(confirm).toContainText("From Radarr and Seerr.");
  await expect(p.getByRole("dialog", { name: "Before you import" })).toHaveCount(0);
  const started = p.waitForRequest((r) => r.url().endsWith("/api/v1/migrate/moves") && r.method() === "POST");
  await confirm.getByRole("button", { name: "Import", exact: true }).click();
  expect((await started).postDataJSON()).toMatchObject({ plan_hash: "h" });
  await expect(p.getByTestId("done-result")).toContainText("Imported", { timeout: 15_000 });
  await expect(current(p)).toHaveAttribute("aria-label", /Step 4 of 4, Done/);
  await expect(p).toHaveURL(/move=7/);
  await ctx.close();
});

test("poll errors: one soft capsule, no toasts, then it recovers; nothing is asked after leaving", async ({ browser }) => {
  const { ctx, p } = await phone(browser, 360);
  const running = (progress: number) => ({ status: 200, json: { move: { id: 7, created_at: "2026-11-03T14:20:00Z", snapshots: [901], state: "running" }, progress, progress_note: `${Math.round(progress * 10)} of 10 imported` } });
  const { done } = await fixture(p, {
    moveAnswers: (n) =>
      n === 0 ? running(0.2) : n <= 2 ? { status: 500, json: { error: "down" } } : n <= 4 ? running(0.6) : { status: 200, json: { move: done, progress: 1, progress_note: "" } },
  });
  await p.goto("/settings/move?move=7");
  await expect(current(p)).toHaveAttribute("aria-label", /Step 3 of 4, Import/);
  await expect(p.getByTestId("move-progress")).toBeVisible();
  await expect(p.getByTestId("move-progress")).toContainText("Can't reach HolyArrs", { timeout: 10_000 });
  // A lost poll never toasts.
  await expect(p.locator(".ui-toast")).toHaveCount(0);
  await expect(p.getByRole("progressbar", { name: "Import progress" })).toBeVisible({ timeout: 15_000 });
  await expect(p.getByTestId("done-result")).toContainText("Imported", { timeout: 15_000 });
  await expect(current(p)).toHaveAttribute("aria-label", /Step 4 of 4, Done/);

  // Leave mid-poll: nothing is asked after the page is gone.
  await p.unroute(/\/api\/v1\/(migrate|library-folders)/);
  await fixture(p, { moveAnswers: () => running(0.5) });
  await p.goto("/settings/move?move=7");
  await expect(p.getByRole("progressbar", { name: "Import progress" })).toBeVisible();
  await p.goto("/settings");
  const asked: string[] = [];
  p.on("request", (r) => r.url().includes("/migrate/moves/7") && asked.push(r.url()));
  await p.waitForTimeout(3000);
  expect(asked).toEqual([]);
  await ctx.close();
});

test("?move=N: a running move shows Move and follows it; a done move shows Done", async ({ browser }) => {
  const { ctx, p } = await phone(browser, 390);
  const { done } = await fixture(p, {
    moveAnswers: (n) =>
      n < 2
        ? { status: 200, json: { move: { ...done, state: "running", report: undefined }, progress: 0.25 * (n + 1), progress_note: `${n + 1} of 4 imported` } }
        : { status: 200, json: { move: done, progress: 1, progress_note: "" } },
  });
  await p.goto("/settings/move?move=7");
  await expect(current(p)).toHaveAttribute("aria-label", /Step 3 of 4, Import/);
  await expect(p.getByTestId("done-result")).toContainText("Imported", { timeout: 15_000 });
  await p.goto("/settings/move?move=8");
  await expect(current(p)).toHaveAttribute("aria-label", /Step 4 of 4, Done/);
  await expect(p.getByText("Import 8 ·")).toBeVisible();
  await p.goBack();
  await expect(current(p)).toHaveAttribute("aria-label", /Step 4 of 4, Done/);
  await expect(p.getByText("Import 7 ·")).toBeVisible();
  await ctx.close();
});

test("a 2,000-title list opens 100 at a time and searches", async ({ browser }) => {
  const { ctx, p } = await phone(browser, 390);
  await fixture(p, { titles: 2000 });
  await p.goto("/settings/move?step=choose&read=901,902");
  await openAll(p);
  await expect(p.getByTestId("group-row").filter({ hasText: "Titles" })).toContainText("2000");
  await p.getByRole("button", { name: "Show Titles" }).click();
  const sheet = p.getByRole("dialog", { name: "Titles" });
  const rows = sheet.getByTestId("items-list").locator(".srow");
  await expect(rows).toHaveCount(100);
  await expect(sheet.getByTestId("items-count")).toHaveText("Showing 100 of 2000");
  await sheet.getByRole("button", { name: "Show 100 more" }).click();
  await expect(rows).toHaveCount(200);
  await sheet.getByLabel("Search this list").fill("Title 1999");
  await expect(rows).toHaveCount(1);
  await noSidewaysScroll(p);
  await ctx.close();
});

for (const width of [360, 390]) {
  test(`phone ${width}: no sideways scroll, symmetry, 44px targets, the footer above the tab bar`, async ({ browser }) => {
    const { ctx, p } = await phone(browser, width);
    await fixture(p);
    await p.goto("/settings/move");
    // The rail: only the current step shows its word; the others are 44px squares.
    const others = p.getByTestId("move-steps").locator("li:not(.now) .st-cap");
    for (const box of await others.all()) {
      const b = await box.boundingBox();
      expect(Math.round(b?.width ?? 0)).toBe(44);
      expect(Math.round(b?.height ?? 0)).toBe(44);
    }
    for (const url of ["/settings/move", "/settings/move?step=choose&read=901,902", "/settings/move?step=try&read=901,902", "/settings/move?move=8"]) {
      await p.goto(url);
      await expect(p.getByTestId("step-foot")).toBeVisible();
      await noSidewaysScroll(p);
      await measuredSymmetry(p);
      // Tap targets: every button, switch and field in the steps is at least 44px tall.
      const small = await p.evaluate(() =>
        [...document.querySelectorAll("main :is([data-testid=move-steps], .ui-settings, [data-testid=step-foot]) :is(button, select, input:not([type=checkbox]), a.ui-link-btn, a.ui-btn, label.switch)")]
          // Segmented controls draw a 44px hit area around a smaller button.
          .filter((el) => (el as HTMLElement).offsetParent && !el.closest(".ui-seg, dialog, details.advanced:not([open])"))
          .map((el) => ({ el: `${el.tagName.toLowerCase()}#${el.id}.${[...el.classList].join(".")}`, h: Math.round(el.getBoundingClientRect().height) }))
          .filter((x) => x.h < 44),
      );
      expect(small, url).toEqual([]);
      // The sticky footer sits above the tab bar, and never covers the last row.
      // (Done's footer scrolls with the page instead, so it is checked at the bottom only.)
      const sticky = await p.getByTestId("step-foot").evaluate((el) => getComputedStyle(el).position === "sticky");
      const foot = await p.getByTestId("step-foot").boundingBox();
      const bar = await p.locator(".ui-tabbar").boundingBox();
      if (sticky && foot && bar) expect(foot.y + foot.height).toBeLessThanOrEqual(bar.y + 1);
      await p.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
      const footEnd = await p.getByTestId("step-foot").boundingBox();
      const barEnd = await p.locator(".ui-tabbar").boundingBox();
      if (footEnd && barEnd) expect(footEnd.y + footEnd.height, `${url}: footer behind the tab bar`).toBeLessThanOrEqual(barEnd.y + 1);
      // Rows: the settings rows, and the move steps' own rows (Read and Choose).
      const rowsHere = p.locator("main .ui-settings :is(.srow, .mrow, .read)");
      const lb = (await rowsHere.count()) ? await rowsHere.last().boundingBox() : null;
      const fb = await p.getByTestId("step-foot").boundingBox();
      if (lb && fb) expect(lb.y + lb.height).toBeLessThanOrEqual(fb.y + 1);
    }
    await ctx.close();
  });
}

test("desktop 1280: the rail capsules are equal, open sheets fit, and accessibility", async ({ browser }) => {
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const p = await ctx.newPage();
  watchDialogs(p);
  await signIn(p, owner());
  await fixture(p);
  await p.goto("/settings/move?step=choose&read=901,902");
  await expect(primary(p)).toBeEnabled();
  const widths = await p.getByTestId("move-steps").locator(".st-cap").evaluateAll((els) => els.map((e) => Math.round(e.getBoundingClientRect().width)));
  expect(widths).toHaveLength(4);
  expect(Math.max(...widths) - Math.min(...widths)).toBeLessThanOrEqual(1);
  await measuredSymmetry(p);
  await openAll(p);
  await p.getByRole("button", { name: "Show Titles" }).click();
  await noSidewaysScroll(p);
  await p.getByRole("dialog", { name: "Titles" }).getByRole("button", { name: "Close", exact: true }).click();
  await p.goto("/settings/move");
  await expect(p.getByTestId("earlier-move").first()).toBeVisible();
  await measuredSymmetry(p);
  // Earlier moves: the pills and the Open buttons line up.
  const xs = await p.getByTestId("earlier-move").evaluateAll((rows) =>
    rows.map((r) => [Math.round(r.querySelector(".ui-pill")?.getBoundingClientRect().left ?? 0), Math.round(r.querySelector("button")?.getBoundingClientRect().left ?? 0)]),
  );
  for (const [pill, open] of xs) {
    expect(pill).toBe(xs[0][0]);
    expect(open).toBe(xs[0][1]);
  }
  await p.evaluate(() => Promise.all(document.getAnimations().filter((a) => a.effect?.getComputedTiming().iterations !== Infinity).map((a) => a.finished.catch(() => undefined))));
  const a11y = await new AxeBuilder({ page: p }).include("main").analyze();
  expect(a11y.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target).join(" ")}`)).toEqual([]);
  await ctx.close();
});

test("phone accessibility on the read step", async ({ browser }) => {
  const { ctx, p } = await phone(browser, 390);
  await fixture(p);
  await p.goto("/settings/move");
  await expect(p.getByTestId("read-row").first()).toBeVisible();
  await p.evaluate(() => Promise.all(document.getAnimations().filter((a) => a.effect?.getComputedTiming().iterations !== Infinity).map((a) => a.finished.catch(() => undefined))));
  const a11y = await new AxeBuilder({ page: p }).include("main").analyze();
  expect(a11y.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target).join(" ")}`)).toEqual([]);
  await ctx.close();
});
