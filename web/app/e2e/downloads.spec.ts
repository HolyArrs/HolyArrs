import AxeBuilder from '@axe-core/playwright';
import { devices, expect, test, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn } from './helpers.ts';
import { qbitPort } from './fakes.mjs';

// The Downloads screen and Needs a decision (stage 4.1). Runs after
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
// Mark failed and Remove live in the row's ⋯ menu.
async function rowAction(r: ReturnType<typeof iron>, name: string) {
	await r.getByRole('button', { name: /^More for / }).click();
	await r.getByRole('menuitem', { name }).click();
}

test('the Downloads screen shows progress, speed and time left, and follows it live', async () => {
	await page.goto('/downloads');
	await expect(page.getByRole('heading', { level: 1, name: 'Downloads' })).toBeVisible();
	const r = iron();
	await expect(r.getByTestId('row-state')).toHaveText('Downloading');
	await expect(r.getByTestId('row-percent')).toHaveText('40%');
	await expect(r.locator('td.c-speed')).toContainText('/s');
	await expect(r.locator('td.c-eta')).not.toHaveText('–');
	await r.getByRole('button', { name: /^More for / }).click();
	await expect(r.getByRole('menuitem', { name: 'Mark failed' })).toBeVisible();
	await expect(r.getByRole('menuitem', { name: 'Remove' })).toBeVisible();
	await page.keyboard.press('Escape');
	await expect(r.getByRole('menuitem', { name: 'Mark failed' })).toBeHidden();
	// The Downloader moves on; the row follows without a reload.
	await fetch(`${qbit}/set?progress=0.7&eta=120`);
	await expect(r.getByTestId('row-percent')).toHaveText('70%', { timeout: 15_000 });
	await expect(r.locator('td.c-eta')).toHaveText('2m');
});

test('Remove asks first, starts from the settings and can turn the Release down and search again', async () => {
	const r = iron();
	await rowAction(r, 'Remove');
	const dialog = page.getByRole('dialog');
	await expect(dialog.getByRole('heading', { name: 'Remove this download?' })).toBeVisible();
	await expect(dialog.getByLabel('Delete its files')).toBeChecked();
	await expect(dialog.getByLabel('Add its Release to Turned down')).not.toBeChecked();
	await expect(dialog.getByLabel('Search again')).toBeDisabled();
	await dialog.getByRole('button', { name: 'Cancel' }).click();
	await expect(dialog).toBeHidden();

	const before = (await (await fetch(`${qbit}/state`)).json()).deleted.length;
	await rowAction(r, 'Remove');
	await dialog.getByLabel('Add its Release to Turned down').check();
	await expect(dialog.getByLabel('Search again')).toBeChecked();
	await dialog.getByRole('button', { name: 'Remove', exact: true }).click();
	await expect(page.getByText('Removed. HolyArrs looks for the next Release.')).toBeVisible();
	await expect.poll(async () => (await (await fetch(`${qbit}/state`)).json()).deleted.length).toBe(before + 1);
	const gone = page.getByTestId('dl-table-row').filter({ hasText: 'Iron Lullaby' }).filter({ has: page.getByTestId('row-state').getByText('Removed', { exact: true }) });
	await expect(gone).toHaveCount(1);
	// Why it was removed reads in the row's details.
	await gone.locator('.nm').click();
	const details = page.getByRole('dialog', { name: 'Download details' });
	await expect(details.getByTestId('row-text')).toContainText('Removed by');
	await page.keyboard.press('Escape');
	await expect(details).toBeHidden();
	// The cam copy is the only one left and the Pick rules refuse it: the
	// movie now needs a decision, and the menu says so.
	await expect(page.getByTestId('decide-count')).toHaveText('1', { timeout: 30_000 });
});

test('Needs a decision says why and offers every choice; stopping clears the badge', async () => {
	await page.locator('main').getByRole('link', { name: /Needs a decision/ }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'Needs a decision' })).toBeVisible();
	const card = page.getByTestId('decision').filter({ hasText: 'Iron Lullaby' });
	await expect(card.getByTestId('decision-why')).toHaveText('HolyArrs ran out of Releases that pass the Pick rules.');
	await expect(card.getByTestId('decision-reason')).toContainText('no other Release passed the Pick rules');
	for (const name of ['Search again', 'Add to Turned down', 'Stop looking']) await expect(card.getByRole('button', { name })).toBeVisible();
	await expect(card.getByRole('link', { name: 'Choose a release by hand' })).toHaveAttribute('href', /\/library\/\d+#h-rel$/);
	await expect(card.getByRole('link', { name: 'Add another Version' })).toHaveAttribute('href', /\/library\/\d+#h-versions$/);

	// Search again: nothing new passes, so it comes back here.
	await card.getByRole('button', { name: 'Search again' }).click();
	await expect(page.getByText('Searching again for Iron Lullaby.')).toBeVisible();
	await expect(card).toBeVisible({ timeout: 30_000 });
	await expect(page.getByTestId('decide-count')).toHaveText('1', { timeout: 30_000 });

	page.once('dialog', (d) => d.accept());
	await card.getByRole('button', { name: 'Stop looking' }).click();
	await expect(page.getByText('Stopped looking for Iron Lullaby.')).toBeVisible();
	await expect(page.getByText('Nothing needs you')).toBeVisible();
	await expect(page.getByTestId('decide-count')).toBeHidden();
});

test('both screens pass the accessibility scan and fit a phone', async ({ browser }) => {
	for (const scheme of ['light', 'dark'] as const) {
		const ctx = await browser.newContext({ colorScheme: scheme });
		await ctx.addInitScript((t) => localStorage.setItem('holyarrs-theme', t), scheme);
		const p = await ctx.newPage();
		await signIn(p, owner());
		for (const where of ['/downloads', '/downloads/decide']) {
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
	await expect(p.getByTestId('dl-table-list').locator('li').first()).toBeVisible();
	await noSidewaysScroll(p);
	await p.goto('/downloads/decide');
	await expect(p.getByRole('heading', { level: 1, name: 'Needs a decision' })).toBeVisible();
	await noSidewaysScroll(p);
	await phone.close();
});
