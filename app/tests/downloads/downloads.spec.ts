import AxeBuilder from '@axe-core/playwright';
import { devices, expect, test, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn } from '../helpers/helpers.ts';
import { qbitPort } from '../fakes/index.mjs';

// The Downloads screen and Needs a decision. Runs after
// downloaders.spec.ts: Iron Lullaby's 720p copy is downloading at 40% in
// the fake qBittorrent and its 1080p copy is already turned down.
test.describe.configure({ mode: 'serial' });

let page: Page;
const qbit = `http://127.0.0.1:${qbitPort}`;

test.beforeAll(async ({ browser }) => {
	page = await (await browser.newContext()).newPage();
	await signIn(page, owner());
});

// The Downloads table: the copy still downloading is the one to act on.
const iron = () => page.locator('[data-testid="dl-table-row"][data-phase="downloading"]').filter({ hasText: 'Iron Lullaby' });
// The tabs are All · Waiting · In progress · Problems · Done. Needs a decision
// lives on the rows: Problems › Review, and the row's details panel holds the choices.
const decideChip = () => page.locator('[data-testid="dl-subseg-item"][data-key="decide"]');
const showProblems = () => page.locator('[data-testid="rail-item"][data-key="problems"]').click();
const pane = () => page.getByTestId('dl-pane');
const col = (r: ReturnType<typeof iron>, id: string) => r.locator(`[data-col="${id}"]`);
// A row opens its details panel in place; Mark failed is in the panel's More actions menu, Remove is the bin on the row.
const rowMenu = (r: ReturnType<typeof iron>) => r.getByTestId('dl-row-menu').getByRole('button').first();
const removeFromMenu = async (r: ReturnType<typeof iron>) => { await rowMenu(r).click(); await page.getByRole('menuitem', { name: 'Remove' }).click(); };
const openRow = (r: ReturnType<typeof iron>) => r.getByTestId('dl-line').click();

test('the Downloads screen shows progress, speed and time left, and follows it live', async () => {
	await page.goto('/downloads');
	await expect(page.getByRole('heading', { level: 1, name: 'Downloads' })).toBeVisible();
	const r = iron();
	await expect(col(r, 'status')).toContainText('Downloading');
	await expect(col(r, 'progress')).toContainText('40%');
	await expect(rowMenu(r)).toBeEnabled();
	// Speed and time left are in the row's details (Info).
	await openRow(r);
	await expect(pane()).toBeVisible();
	await expect(pane()).toContainText('/s');
	await expect(pane()).toContainText('Time left');
	// The Downloader moves on; the row and its details follow without a reload.
	await fetch(`${qbit}/set?progress=0.7&eta=120`);
	await expect(col(r, 'progress')).toContainText('70%', { timeout: 15_000 });
	await expect(pane()).toContainText('2m', { timeout: 15_000 });
	// A click opens a row's details; Escape from the row closes them.
	await r.getByTestId('dl-line').focus();
	await page.keyboard.press('Escape');
	await expect(pane()).toBeHidden();
});

test('Seeding rules: the sheet opens from the details, sets a ratio and saves it to qBittorrent', async () => {
	const r = iron();
	await openRow(r);
	await pane().getByTestId('dl-more').getByRole('button').first().click();
	await page.getByRole('menuitem', { name: 'Seeding rules…' }).click();
	const sheet = page.getByRole('dialog');
	await expect(sheet.getByRole('heading', { name: 'Seeding rules' })).toBeVisible();
	await sheet.getByTestId('steer-ratio-set').click();
	await sheet.getByTestId('steer-ratio-value').fill('1.5');
	await sheet.getByTestId('steer-seed-none').click();
	const before = (await (await fetch(`${qbit}/state`)).json()).shareLimits.length;
	await sheet.getByTestId('steer-submit').click();
	await expect(page.getByText('Seeding rules set for this torrent')).toBeVisible();
	await expect(sheet).toBeHidden();
	const got = (await (await fetch(`${qbit}/state`)).json()).shareLimits;
	expect(got.length).toBe(before + 1);
	// Ratio 1.5, no seeding time limit (-1), the idle limit left to qBittorrent's own rule (-2).
	expect(got.at(-1)).toMatchObject({ ratioLimit: '1.5', seedingTimeLimit: '-1', inactiveSeedingTimeLimit: '-2' });
	await r.getByTestId('dl-line').focus();
	await page.keyboard.press('Escape');
	await expect(pane()).toBeHidden();
});

test('Remove asks first, starts from the settings and can turn the Release down and search again', async () => {
	const r = iron();
	await removeFromMenu(r);
	const dialog = page.getByRole('dialog');
	await expect(dialog.getByRole('heading', { name: 'Remove this download?' })).toBeVisible();
	await expect(dialog.getByLabel('Also delete files')).toBeChecked();
	await expect(dialog.getByLabel('Add its Release to Turned down')).not.toBeChecked();
	await expect(dialog.getByLabel('Search again')).toBeDisabled();
	await dialog.getByRole('button', { name: 'Cancel' }).click();
	await expect(dialog).toBeHidden();

	const before = (await (await fetch(`${qbit}/state`)).json()).deleted.length;
	await removeFromMenu(r);
	await dialog.getByLabel('Add its Release to Turned down').check();
	await expect(dialog.getByLabel('Search again')).toBeChecked();
	// The confirm button names what goes: "Remove and delete files" here.
	await dialog.getByTestId('dl-remove-confirm').click();
	await expect(page.getByText('Download removed')).toBeVisible();
	await expect.poll(async () => (await (await fetch(`${qbit}/state`)).json()).deleted.length).toBe(before + 1);
	// The copy is no longer downloading; it turns into Needs a decision once the search runs out.
	await expect(iron()).toHaveCount(0);
	// The cam copy is the only one left and the Pick rules refuse it: the
	// movie now needs a decision, and Problems › Needs a decision says so.
	await showProblems();
	await expect(decideChip()).toHaveAttribute('data-count', '1', { timeout: 30_000 });
});

test('Needs a decision says why and offers every choice in the row\'s details; stopping clears it', async () => {
	await decideChip().click();
	const r = page.getByTestId('dl-table-row').filter({ hasText: 'Iron Lullaby' }).filter({ hasText: 'Review' });
	await r.getByTestId('dl-line').click();
	// The choices are in the row's details pane.
	const card = pane().getByTestId('decision');
	await card.getByTestId('decision-why').click();
	await expect(card.getByTestId('decision-why-box')).toContainText('HolyArrs ran out of Releases that pass the Pick rules.');
	await expect(card.getByTestId('decision-reason')).toContainText('no other Release passed the Pick rules');
	// Search again is the main choice; the others are under More.
	await expect(card.getByRole('button', { name: 'Search again' })).toBeVisible();
	await card.getByRole('button', { name: /^More/ }).click();
	for (const name of ['Add to Turned down', 'Stop looking']) await expect(page.getByRole('menuitem', { name })).toBeVisible();
	await expect(page.getByRole('menuitem', { name: 'Choose a release by hand' })).toHaveAttribute('href', /\/library\/\d+\?tab=library&pick=1#h-rel$/);
	await expect(page.getByRole('menuitem', { name: 'Add another Version' })).toHaveAttribute('href', /\/library\/\d+\?tab=library#h-versions$/);
	await page.keyboard.press('Escape');

	// Search again: nothing new passes, so it comes back here.
	await card.getByRole('button', { name: 'Search again' }).click();
	await expect(page.getByText('Searching again for Iron Lullaby')).toBeVisible();
	await expect(card).toBeVisible({ timeout: 30_000 });
	await expect(decideChip()).toHaveAttribute('data-count', '1', { timeout: 30_000 });

	page.once('dialog', (d) => d.accept());
	// The card may draw once more as the search's last news lands: open the menu again if it went.
	await expect(async () => {
		if (!(await page.getByRole('menuitem', { name: 'Stop looking' }).isVisible())) await card.getByRole('button', { name: /^More/ }).click();
		await page.getByRole('menuitem', { name: 'Stop looking' }).click({ timeout: 2000 });
	}).toPass({ timeout: 20_000 });
	await expect(page.getByText('Stopped looking for Iron Lullaby')).toBeVisible();
	await expect(card).toHaveCount(0, { timeout: 30_000 });
	await expect(decideChip()).toHaveAttribute('data-count', '0', { timeout: 30_000 });
});

test('both screens pass the accessibility scan and fit a phone', async ({ browser }) => {
	for (const scheme of ['light', 'dark'] as const) {
		const ctx = await browser.newContext({ colorScheme: scheme });
		await ctx.addInitScript((t) => localStorage.setItem('holyarrs-theme', t), scheme);
		const p = await ctx.newPage();
		await signIn(p, owner());
		for (const where of ['/downloads', '/downloads?show=problems.decide']) {
			await p.goto(where);
			await expect(p.getByRole('heading', { level: 1 })).toBeVisible();
			await expect(p.locator('.sk')).toHaveCount(0);
			await p.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
			const r = await new AxeBuilder({ page: p }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
			const bad = r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
			expect.soft(bad.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(' ')).slice(0, 3).join(', ')}`), `${where} (${scheme})`).toEqual([]);
		}
		await ctx.close();
	}
	const phone = await browser.newContext({ ...devices['Pixel 7'] });
	const p = await phone.newPage();
	await signIn(p, owner());
	await p.goto('/downloads');
	await expect(p.getByTestId('dl-list').locator('li').first()).toBeVisible();
	await noSidewaysScroll(p);
	// The old Needs a decision address lands on Problems › Needs a decision.
	await p.goto('/downloads/decide');
	await expect(p).toHaveURL(/\/downloads\?show=problems\.decide$/);
	await expect(p.getByRole('heading', { level: 1, name: 'Downloads' })).toBeVisible();
	await noSidewaysScroll(p);
	await phone.close();
});
