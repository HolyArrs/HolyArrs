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

test('history: every step is listed with filters by kind, movie and date, and paging', async () => {
	await page.goto('/downloads');
	await page.locator('main').getByRole('link', { name: 'History', exact: true }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'History' })).toBeVisible();
	const rows = page.getByTestId('history-row');
	await expect(rows.first()).toBeVisible();
	await expect(page.getByTestId('history-count')).toContainText(' of ');

	// What happened: only renames.
	await page.getByRole('checkbox', { name: 'Renamed' }).check();
	await expect(page).toHaveURL(/kinds=rename/);
	await expect(rows.first()).toBeVisible();
	for (const k of await rows.evaluateAll((els) => els.map((e) => e.getAttribute('data-kind')))) expect(k).toMatch(/^rename/);
	// Why and where: the reasons and paths of a row.
	await rows.first().getByRole('button', { name: 'Why and where' }).click();
	await expect(rows.first().locator('dl')).toContainText('From');
	await page.getByRole('checkbox', { name: 'Renamed' }).uncheck();

	// A date range in the future matches nothing.
	await page.getByLabel('From').fill('2099-01-01');
	await page.getByLabel('From').dispatchEvent('change');
	await expect(page.getByText('Nothing matches')).toBeVisible();
	await page.getByRole('button', { name: 'Show everything' }).click();
	await expect(rows.first()).toBeVisible();

	// One movie.
	const titles = (await (await page.request.get('/api/v1/titles')).json()).titles as TitleRow[];
	const cinder = titles.find((t) => t.name === 'Cinder Waltz')!;
	await page.getByLabel('Movie').selectOption(String(cinder.id));
	await expect(page.getByTestId('history-count')).toContainText('for Cinder Waltz');
	for (const t of await rows.locator('a').allTextContents()) expect(t).toBe('Cinder Waltz');
});

test('history on the movie page links to the movie in History', async () => {
	await page.goto('/library');
	await page.getByRole('link', { name: /Cinder Waltz/ }).first().click();
	const card = page.getByTestId('title-history');
	await expect(card.getByRole('heading', { name: 'History' })).toBeVisible();
	await expect(card.locator('li').first()).toBeVisible();
	await card.getByRole('link', { name: /All \d+ rows/ }).click();
	await expect(page).toHaveURL(/\/downloads\/history\?title=\d+/);
	await expect(page.getByTestId('history-count')).toContainText('for Cinder Waltz');
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

	await page.goto('/');
	const home = page.getByTestId('home-health');
	// Health is a fact in the This HolyArrs panel; History links from Latest Events.
	await expect(home.getByRole('heading', { name: 'This HolyArrs' })).toBeVisible();
	await expect(home.getByText('Health', { exact: true })).toBeVisible();
	await expect(home.getByRole('link', { name: 'All health checks' })).toBeVisible();
	await expect(page.getByRole('region', { name: 'Latest Events' }).getByRole('link', { name: 'History' })).toBeVisible();
});

test('calendar: month and list views with cinema, digital and disc dates', async () => {
	const titles = (await (await page.request.get('/api/v1/titles')).json()).titles as TitleRow[];
	const dated = titles.find((t) => t.cinema_date)!;
	const month = dated.cinema_date!.slice(0, 7);
	await page.goto(`/calendar?month=${month}`);
	const grid = page.getByTestId('cal-grid');
	await expect(grid).toBeVisible();
	await expect(grid.locator(`[data-day="${dated.cinema_date!.slice(0, 10)}"]`).getByRole('link', { name: new RegExp(dated.name) })).toBeVisible();
	const heading = await page.getByTestId('cal-month').textContent();
	await page.getByRole('button', { name: 'Month after' }).click();
	await expect(page.getByTestId('cal-month')).not.toHaveText(heading!);
	await page.getByRole('button', { name: 'Month before' }).click();
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

test('calendar feed: a link with its own key, shown once, renewed and turned off', async ({ playwright }) => {
	await page.goto('/calendar');
	const box = page.getByTestId('cal-feed');
	await expect(box.getByTestId('feed-state')).toHaveText('The feed is off.');
	await box.getByRole('button', { name: 'Make a feed link' }).click();
	const link = await box.getByLabel('Your feed link (shown this once)').inputValue();
	expect(link).toMatch(/\/api\/v1\/calendar\/feed\.ics\?key=[\w-]{40,}$/);
	// A phone has no session: the key alone opens the feed.
	const phone = await playwright.request.newContext();
	const r = await phone.get(link);
	expect(r.status()).toBe(200);
	expect(r.headers()['content-type']).toContain('text/calendar');
	expect(await r.text()).toContain('BEGIN:VCALENDAR');
	// Shown once: after a reload only "on" is said.
	await page.reload();
	await expect(box.getByTestId('feed-state')).toContainText('A feed link is on');
	await expect(box.getByLabel('Your feed link (shown this once)')).toHaveCount(0);
	// A new link ends the old one.
	page.once('dialog', (d) => d.accept());
	await box.getByRole('button', { name: 'Make a new link' }).click();
	await expect(box.getByLabel('Your feed link (shown this once)')).toBeVisible();
	expect((await phone.get(link)).status()).toBe(401);
	const second = await box.getByLabel('Your feed link (shown this once)').inputValue();
	page.once('dialog', (d) => d.accept());
	await box.getByRole('button', { name: 'Turn the feed off' }).click();
	await expect(box.getByTestId('feed-state')).toHaveText('The feed is off.');
	expect((await phone.get(second)).status()).toBe(401);
	await phone.dispose();
});

test('alerts: add a webhook with Library moments, send a test, the receiver gets a signed message', async () => {
	await page.goto('/settings/alerts');
	await page.getByRole('button', { name: 'Add an Alert' }).click();
	await page.getByLabel('Name', { exact: true }).fill('Home server hook');
	await page.getByLabel('Kind').selectOption('webhook');
	await page.getByLabel('Webhook link').fill(`http://127.0.0.1:${discordPort}/hook`);
	await page.getByLabel('Secret', { exact: true }).fill(['e2e', 'hook'].join('-'));
	await expect(page.getByLabel('What the secret does')).toHaveValue('sign');
	await page.getByTestId('moments-wishlist').getByRole('button', { name: 'None of these' }).click();
	await expect(page.getByTestId('moments-wishlist').getByRole('checkbox', { name: /Someone asks/ })).not.toBeChecked();
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
