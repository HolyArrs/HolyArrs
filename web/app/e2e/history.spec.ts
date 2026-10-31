import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Browser, type Page } from '@playwright/test';
import { discordPort, jellyKey, jellyfinPort } from './fakes.mjs';
import { noSidewaysScroll, owner, signIn, openAdvanced } from './helpers.ts';

// History, health, the calendar with its phone feed, webhook Alerts and
// media servers (5.1.10 to 5.1.17). Runs after the Organise tests, so the
// Library has movies and every step has left Events.
test.describe.configure({ mode: 'serial' });

let page: Page;

async function fresh(browser: Browser, width = 1280) {
	const ctx = await browser.newContext({ viewport: { width, height: 900 } });
	return ctx.newPage();
}

async function axe(p: Page, where: string, only?: string[]) {
	await p.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
	let b = new AxeBuilder({ page: p }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']);
	if (only) b = b.withRules(only);
	const r = await b.analyze();
	const bad = r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
	if (bad.length) console.log('AXE', where, bad.map((v) => v.id + ' ' + v.nodes.slice(0, 3).map((n) => n.target.join(' ')).join(' | ')).join(' ;; '));
	expect(bad.map((v) => v.id), where).toEqual([]);
}

interface TitleRow {
	id: number;
	name: string;
	cinema_date?: string;
}

test.beforeAll(async ({ browser }) => {
	page = await fresh(browser);
	await signIn(page, owner());
});

test('history: day by day, with tabs, one title and a details panel', async () => {
	await page.goto('/downloads');
	await page.locator('main').getByRole('link', { name: 'History', exact: true }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'History' })).toBeVisible();
	const rows = page.getByTestId('history-row');
	await expect(rows.first()).toBeVisible();
	await expect(page.getByTestId('history-count')).toContainText(' of ');

	// Changes, then Renamed: only renames, kept in the address.
	await page.getByTestId('look-tabs').getByRole('button', { name: /^Changes/ }).click();
	await page.getByTestId('look-subtabs').getByRole('button', { name: /^Renamed/ }).click();
	await expect(page).toHaveURL(/show=changes\.renamed/);
	await expect.poll(async () => (await rows.evaluateAll((els) => els.map((e) => e.getAttribute('data-kind')))).every((k) => /^rename/.test(k ?? ''))).toBe(true);
	// The details panel tells the picked row's reasons and paths.
	await rows.first().click();
	await expect(page.getByTestId('look-pane').locator('dl').first()).toContainText('From');
	// The panel hides and comes back, and a picked row brings it back too.
	const toggle = page.getByTestId('history-details-toggle');
	await toggle.click();
	await expect(page.getByTestId('look-days')).toHaveAttribute('data-panel', 'hidden');
	await rows.first().click();
	await expect(page.getByTestId('look-days')).toHaveAttribute('data-panel', 'open');

	// A date range in the future matches nothing.
	await page.goto('/downloads/history?from=2099-01-01');
	await expect(page.getByText('Nothing matches')).toBeVisible();

	// One movie.
	const titles = (await (await page.request.get('/api/v1/titles')).json()).titles as TitleRow[];
	const cinder = titles.find((t) => t.name === 'Cinder Waltz')!;
	await page.goto(`/downloads/history?title=${cinder.id}`);
	await expect(page.getByTestId('look-title')).toContainText('Cinder Waltz');
	await expect(rows.first()).toBeVisible();
	for (const t of await page.getByTestId('history-title').allTextContents()) expect(t).toMatch(/^Cinder Waltz/);
});

test('history on the movie page links to the movie in History', async () => {
	await page.goto('/library');
	await page.getByRole('link', { name: /Cinder Waltz/ }).first().click();
	const card = page.getByTestId('title-history');
	await expect(card.getByRole('heading', { name: 'History' })).toBeVisible();
	await expect(card.locator('li').first()).toBeVisible();
	await card.getByRole('link', { name: /All \d+ rows/ }).click();
	await expect(page).toHaveURL(/\/downloads\/history\?title=\d+/);
	await expect(page.getByTestId('look-title')).toContainText('Cinder Waltz');
});

test('health: checks run on Health checks, show on Home, and their settings are saved', async () => {
	await page.goto('/settings/this-app');
	await expect(page.getByTestId('health-summary')).toBeVisible();
	await page.getByRole('link', { name: 'See the health checks' }).click();
	await expect(page.getByRole('heading', { name: 'Health checks', level: 1 })).toBeVisible();
	await page.getByRole('button', { name: 'Check again' }).click();
	const checks = page.getByTestId('health-check');
	await expect(checks.first()).toBeVisible();
	await expect(page.getByTestId('health-checks')).toContainText('Library folder');
	await expect(page.getByTestId('health-checks')).toContainText('Free space');
	await openAdvanced(page);
	await page.getByLabel('Alert after this many runs in a row').fill('3');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await expect(page.getByText('Health check settings saved.')).toBeVisible();
	await page.reload();
	await expect(page.getByLabel('Alert after this many runs in a row')).toHaveValue('3');
	await page.getByLabel('Alert after this many runs in a row').fill('2');
	await page.getByRole('button', { name: 'Save', exact: true }).click();

});

test('calendar: cards and list views with cinema, digital and disc dates', async () => {
	const titles = (await (await page.request.get('/api/v1/titles')).json()).titles as TitleRow[];
	const dated = titles.find((t) => t.cinema_date)!;
	const month = dated.cinema_date!.slice(0, 7);
	await page.goto(`/calendar?month=${month}`);
	// Cards: a day per row, from the month's first week.
	const cards = page.getByTestId('cal-cards');
	await expect(cards).toBeVisible();
	await expect(cards.locator(`[data-day="${dated.cinema_date!.slice(0, 10)}"]`).getByRole('link', { name: new RegExp(dated.name) }).first()).toBeVisible();
	const heading = await page.getByTestId('cal-month').textContent();
	await page.getByRole('button', { name: 'Week after' }).click();
	await expect(page.getByTestId('cal-month')).not.toHaveText(heading!);
	await page.getByRole('button', { name: 'Week before' }).click();
	await expect(page.getByTestId('cal-month')).toHaveText(heading!);
	await page.getByRole('radio', { name: 'List', exact: true }).click();
	const list = page.getByTestId('cal-list');
	await expect(list.getByRole('link', { name: new RegExp(dated.name) })).toBeVisible();
	await expect(list).toContainText('In cinemas');
	// Turning cinema dates off hides them.
	await page.getByRole('checkbox', { name: 'In cinemas' }).uncheck();
	await expect(list.getByText('In cinemas')).toHaveCount(0);
	await page.getByRole('checkbox', { name: 'In cinemas' }).check();
});

test('alerts: add a webhook with Library moments, send a test, the receiver gets a signed message', async () => {
	await page.goto('/settings/alerts');
	await page.getByRole('button', { name: 'Add an Alert' }).click();
	await page.getByLabel('Name', { exact: true }).fill('Home server hook');
	await page.getByLabel('Kind').selectOption('webhook');
	await page.getByLabel('Webhook link').fill(`http://127.0.0.1:${discordPort}/hook`);
	await page.getByLabel('Secret', { exact: true }).fill(['e2e', 'hook'].join('-'));
	await expect(page.getByLabel('What the secret does')).toHaveValue('sign');
	await page.getByTestId('moments-requests').getByRole('button', { name: 'None of these' }).click();
	await expect(page.getByTestId('moments-requests').getByRole('checkbox', { name: /Someone asks/ })).not.toBeChecked();
	await expect(page.getByTestId('moments-library').getByRole('checkbox', { name: /Sent to download/ })).toBeChecked();
	await page.getByRole('checkbox', { name: /May reach the local network/ }).check();
	await page.getByRole('button', { name: 'Save Alert' }).click();
	const card = page.getByTestId('alert').filter({ hasText: 'Home server hook' });
	await expect(card).toContainText('Webhook');
	await expect(card).toContainText('Sent to download · In the Library · Renamed');
	await expect(card).not.toContainText('Someone asks');
	await card.getByRole('button', { name: 'Send a test' }).click();
	await expect(page.getByText('Test sent through Home server hook.')).toBeVisible();
	const hooks = (await (await page.request.get(`http://127.0.0.1:${discordPort}/hooks`)).json()) as { body: { moment: string }; event: string; signature: string }[];
	const last = hooks.at(-1)!;
	expect(last.event).toBe('test');
	expect(last.body.moment).toBe('test');
	expect(last.signature).toMatch(/^sha256=[0-9a-f]{64}$/);
	// The link and the secret come back masked.
	await card.getByRole('button', { name: 'Change' }).click();
	await expect(page.getByLabel('Webhook link')).not.toHaveValue(new RegExp(String(discordPort)));
	await expect(page.getByLabel('Secret', { exact: true })).toHaveValue('');
	await page.getByRole('button', { name: 'Cancel' }).click();
});

test('media servers: add Jellyfin, test it, a wrong key is said plainly', async () => {
	await page.goto('/settings');
	await page.getByRole('link', { name: 'Media servers' }).first().click();
	await expect(page.getByRole('heading', { level: 1, name: 'Media servers' })).toBeVisible();
	await page.getByRole('button', { name: 'Add a media server' }).click();
	await page.getByLabel('Name', { exact: true }).fill('Lab Jellyfin');
	await page.getByLabel('Kind').selectOption('jellyfin');
	await page.getByLabel('Address').fill(`http://127.0.0.1:${jellyfinPort}`);
	await page.getByLabel('API key').fill(jellyKey);
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	const card = page.getByTestId('media-server').filter({ hasText: 'Lab Jellyfin' });
	await expect(card).toContainText('Told about imports, renames, deletes');
	await card.getByRole('button', { name: 'Test' }).click();
	await expect(page.getByText('Lab Jellyfin answered: Fake Jellyfin 10.9.0.')).toBeVisible();
	await expect(card.getByTestId('media-result')).toContainText('Last answered');
	await card.getByRole('button', { name: 'Change' }).click();
	await page.getByLabel('API key').fill('wrong-key');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await card.getByRole('button', { name: 'Test' }).click();
	await expect(page.getByText('Lab Jellyfin: the server turned down the token')).toBeVisible();
	await expect(card.getByTestId('media-result')).toContainText('Last try failed: the server turned down the token');
});

test('red buttons keep their contrast on hover', async () => {
	await page.goto('/settings/media-servers');
	await page.getByRole('button', { name: 'Add a media server' }).hover();
	await axe(page, 'red button hover', ['color-contrast']);
});

test('the new screens at phone width: no sideways scroll, no serious accessibility issues', async ({ browser }) => {
	const ph = await fresh(browser, 390);
	await signIn(ph, owner());
	for (const url of ['/downloads/history', '/calendar', '/calendar?month=2014-02', '/settings/this-app', '/settings/alerts', '/settings/media-servers', '/']) {
		await ph.goto(url);
		await ph.waitForLoadState('networkidle');
		await noSidewaysScroll(ph);
		await axe(ph, url + ' (phone)');
	}
	await ph.goto('/calendar?month=2014-02');
	await ph.getByRole('radio', { name: 'List', exact: true }).click();
	await noSidewaysScroll(ph);
	await ph.context().close();
});
