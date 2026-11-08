import { expect, test, type Browser, type Page } from '@playwright/test';
import { discordPort, jellyKey, jellyfinPort } from '../fakes/index.mjs';
import { noSidewaysScroll, owner, signIn, openAdvanced } from '../helpers/helpers.ts';
import { axe as scan } from '../helpers/axe.ts';

// History, health, the calendar with its phone feed, webhook Alerts and
// media servers (5.1.10 to 5.1.17). Runs after the Organise tests, so the
// Library has movies and every step has left Events.
test.describe.configure({ mode: 'serial' });

let page: Page;

async function fresh(browser: Browser, width = 1280) {
	const ctx = await browser.newContext({ viewport: { width, height: 900 } });
	return ctx.newPage();
}

const axe = (p: Page, where: string, only?: string[]) => scan(p, where, { only, waitAll: true });
interface TitleRow {
	id: number;
	name: string;
	cinema_date?: string;
	digital_date?: string;
	physical_date?: string;
}

test.beforeAll(async ({ browser }) => {
	page = await fresh(browser);
	await signIn(page, owner());
});

test('history: day by day, with a Filter, one title and a details panel', async () => {
	await page.goto('/downloads/history');
	await expect(page.getByRole('heading', { level: 1, name: 'History' })).toBeAttached();
	await expect(page.getByTestId('history-search-bar')).toBeVisible();
	const rows = page.getByTestId('history-row');
	await expect(rows.first()).toBeVisible();
	await expect(page.getByTestId('history-count')).toContainText(' of ');

	// Filter, then Renamed: only renames, kept in the address and shown as a chip.
	await page.getByTestId('history-search-bar').getByRole('button', { name: /^Filter/ }).click();
	await page.getByRole('dialog', { name: 'Filter' }).getByRole('button', { name: 'Renamed', exact: true }).click();
	await expect(page).toHaveURL(/step=renamed/);
	await page.keyboard.press('Escape');
	await expect(page.getByTestId('history-chips')).toContainText('Renamed');
	await expect.poll(async () => (await rows.evaluateAll((els) => els.map((e) => e.getAttribute('data-kind')))).every((k) => /^rename/.test(k ?? ''))).toBe(true);
	// The details panel tells the picked row's reasons and paths.
	await rows.first().click();
	await expect(page.getByTestId('look-pane').locator('dl').first()).toContainText('From');

	// A date range in the future matches nothing.
	await page.goto('/downloads/history?from=2099-01-01');
	await expect(page.getByText('Nothing matches')).toBeVisible();

	// One movie: its rows only, named in a removable chip.
	const titles = (await (await page.request.get('/api/v1/titles')).json()).titles as TitleRow[];
	const cinder = titles.find((t) => t.name === 'Cinder Waltz')!;
	await page.goto(`/downloads/history?title=${cinder.id}`);
	await expect(page.getByTestId('history-chips')).toContainText('Cinder Waltz');
	await expect(rows.first()).toBeVisible();
	for (const t of await page.getByTestId('history-title').allTextContents()) expect(t).toMatch(/^Cinder Waltz/);
});

test('history on the movie page links to the movie in History', async () => {
	await page.goto('/library');
	await page.getByRole('link', { name: /Cinder Waltz/ }).first().click();
	// The movie's History sits in its Library tab; a row opens History kept to the movie.
	await page.getByRole('tab', { name: /^Library/ }).click();
	await expect(page.getByRole('heading', { name: 'History', exact: true })).toBeVisible();
	const row = page.getByTestId('history-row').first();
	await expect(row).toBeVisible();
	await row.click();
	await expect(page).toHaveURL(/\/downloads\/history\?title=\d+/);
	await expect(page.getByTestId('history-chips')).toContainText('Cinder Waltz');
});

test('health: checks run on Health checks, show on Home, and their settings are saved', async () => {
	await page.goto('/settings/this-app');
	await page.getByRole('link', { name: 'Health checks', exact: true }).click();
	await expect(page.getByRole('heading', { name: 'Health checks', level: 1 })).toBeVisible();
	await page.getByRole('button', { name: 'Check again' }).click();
	const checks = page.getByTestId('health-check');
	await expect(checks.first()).toBeVisible();
	await expect(page.getByTestId('health-checks')).toContainText('Library folder');
	await expect(page.getByTestId('health-checks')).toContainText('Free space');
	await openAdvanced(page);
	await page.getByLabel('Alert after this many runs in a row').fill('3');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await expect(page.getByText('Health check settings saved')).toBeVisible();
	await page.reload();
	await expect(page.getByLabel('Alert after this many runs in a row')).toHaveValue('3');
	await page.getByLabel('Alert after this many runs in a row').fill('2');
	await page.getByRole('button', { name: 'Save', exact: true }).click();

});

test('calendar: cards and list views, a movie on the day it is out to watch', async () => {
	const titles = (await (await page.request.get('/api/v1/titles')).json()).titles as TitleRow[];
	const dated = titles.find((t) => t.cinema_date)!;
	// A movie shows one date: the day it is out to watch (the earlier of digital and disc), else its cinema date.
	const home = [dated.digital_date, dated.physical_date].filter((d): d is string => !!d).map((d) => d.slice(0, 10)).sort()[0];
	const day = home ?? dated.cinema_date!.slice(0, 10);
	await page.goto(`/calendar?month=${day.slice(0, 7)}`);
	// Cards: a day per row, from the linked month.
	const cards = page.getByTestId('cal-cards');
	await expect(cards).toBeVisible();
	const card = cards.locator(`[data-day="${day}"]`).getByRole('link', { name: new RegExp(dated.name) }).first();
	await card.scrollIntoViewIfNeeded();
	await expect(card).toBeVisible();
	// The arrows step from the week in view, even on a linked month far back (loading further as
	// needed): Week after names a later week, Week before an earlier one. The heading carries the
	// week's first day; read it once it has stopped moving.
	const controls = page.getByRole('region', { name: 'Calendar controls' });
	const weekHead = controls.getByTestId('cal-month');
	const settled = async () => {
		let last = '';
		await expect.poll(async () => { const now = (await weekHead.getAttribute('data-week')) ?? ''; const same = now === last; last = now; return same; }, { intervals: [300, 300, 300, 500, 1000] }).toBe(true);
		return last;
	};
	const before = await settled();
	await controls.getByRole('button', { name: 'Week after' }).click();
	await expect(weekHead).not.toHaveAttribute('data-week', before);
	const after = await settled();
	expect(after > before, `Week after: ${after} after ${before}`).toBe(true);
	await controls.getByRole('button', { name: 'Week before' }).click();
	await expect(weekHead).not.toHaveAttribute('data-week', after);
	const back = await settled();
	expect(back < after, `Week before: ${back} before ${after}`).toBe(true);
	await page.getByTestId('cal-view').getByRole('radio', { name: 'List', exact: true }).click();
	const list = page.getByTestId('cal-list');
	const row = list.locator(`[data-day="${day}"]`).getByRole('link', { name: new RegExp(dated.name) });
	await row.scrollIntoViewIfNeeded();
	await expect(row).toBeVisible();
	// The kind filter: Movies keeps it, TV shows hides it.
	await controls.getByRole('radio', { name: 'Movies', exact: true }).click();
	await expect(list.getByRole('link', { name: new RegExp(dated.name) })).toHaveCount(1);
	await controls.getByRole('radio', { name: 'TV shows', exact: true }).click();
	await expect(list.getByRole('link', { name: new RegExp(dated.name) })).toHaveCount(0);
	await controls.getByRole('radio', { name: 'All', exact: true }).click();
});

test('alerts: add a webhook with Library moments, send a test, the receiver gets a signed message', async () => {
	await page.goto('/settings/alerts');
	await page.getByRole('button', { name: 'Add an Alert' }).first().click();
	await page.getByLabel('Name', { exact: true }).fill('Home server hook');
	await page.getByLabel('Kind').selectOption('webhook');
	await page.getByLabel('Webhook link').fill(`http://127.0.0.1:${discordPort}/hook`);
	await page.getByLabel('Secret', { exact: true }).fill(['e2e', 'hook'].join('-'));
	await expect(page.getByLabel('What the secret does')).toHaveValue('sign');
	await page.getByTestId('moments-requests').getByRole('button', { name: 'None of these' }).click();
	await expect(page.getByTestId('moments-requests').getByRole('checkbox', { name: /New request/ })).not.toBeChecked();
	await expect(page.getByTestId('moments-library').getByRole('checkbox', { name: /Sent to download/ })).toBeChecked();
	await page.getByRole('checkbox', { name: /Reach the local network/ }).check();
	await page.locator('form.sc-form').getByRole('button', { name: 'Save', exact: true }).click();
	const card = page.getByTestId('alert').filter({ hasText: 'Home server hook' });
	await expect(card).toContainText('Webhook');
	await expect(card).toContainText('Sent to download · In the Library · Renamed');
	await expect(card).not.toContainText('New request');
	await card.getByRole('button', { name: 'Send a test' }).click();
	await expect(card.getByTestId('alert-test')).toHaveText('Webhook');
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

test('alerts: add a Telegram bot, send a test, the chat gets the text', async () => {
	await page.goto('/settings/alerts');
	await page.getByRole('button', { name: 'Add an Alert' }).first().click();
	await page.getByLabel('Name', { exact: true }).fill('Family Telegram');
	await page.getByLabel('Kind').selectOption('telegram');
	await page.getByLabel('Bot token').fill(['123456789', 'AAHstandInTokenForE2eTests_0123'].join(':'));
	await page.getByLabel('Chat ID').fill('-1001234567890');
	await page.getByRole('checkbox', { name: /Reach the local network/ }).check();
	await page.locator('form.sc-form').getByRole('button', { name: 'Save', exact: true }).click();
	const card = page.getByTestId('alert').filter({ hasText: 'Family Telegram' });
	await expect(card).toContainText('Telegram');
	await card.getByRole('button', { name: 'Send a test' }).click();
	await expect(card.getByTestId('alert-test')).toHaveText('Telegram');
	const sent = (await (await page.request.get(`http://127.0.0.1:${discordPort}/telegram`)).json()) as { chat_id: string; text: string; disable_web_page_preview: boolean }[];
	const last = sent.at(-1)!;
	expect(last.chat_id).toBe('-1001234567890');
	expect(last.text).toContain('HolyArrs test');
	expect(last.disable_web_page_preview).toBe(true);
	// The token comes back masked; the chat ID is shown as saved.
	await card.getByRole('button', { name: 'Change' }).click();
	await expect(page.getByLabel('Bot token')).toHaveValue('');
	await expect(page.getByLabel('Chat ID')).toHaveValue('-1001234567890');
	await page.getByRole('button', { name: 'Cancel' }).click();
});

test('alerts: add an ntfy topic, send a test, the topic gets the text', async () => {
	await page.goto('/settings/alerts');
	await page.getByRole('button', { name: 'Add an Alert' }).first().click();
	await page.getByLabel('Name', { exact: true }).fill('Phone ntfy');
	await page.getByLabel('Kind').selectOption('ntfy');
	await page.getByLabel('Server address').fill(`http://127.0.0.1:${discordPort}`);
	await page.getByLabel('Topic').fill('holyarrs-e2e');
	await page.getByRole('checkbox', { name: /Reach the local network/ }).check();
	await page.locator('form.sc-form').getByRole('button', { name: 'Save', exact: true }).click();
	const card = page.getByTestId('alert').filter({ hasText: 'Phone ntfy' });
	await expect(card).toContainText('ntfy');
	await card.getByRole('button', { name: 'Send a test' }).click();
	await expect(card.getByTestId('alert-test')).toHaveText('ntfy');
	const sent = (await (await page.request.get(`http://127.0.0.1:${discordPort}/apps`)).json()) as { app: string; path: string; title: string; auth: boolean; body: string }[];
	const last = sent.filter((x) => x.app === 'ntfy').at(-1)!;
	expect(last.path).toBe('/holyarrs-e2e');
	expect(last.title).toBe('HolyArrs test');
	expect(last.body).toContain('Phone ntfy works');
	expect(last.auth).toBe(false);
	// The server and topic are shown as saved; no token was stored.
	await card.getByRole('button', { name: 'Change' }).click();
	await expect(page.getByLabel('Server address')).toHaveValue(`http://127.0.0.1:${discordPort}`);
	await expect(page.getByLabel('Topic')).toHaveValue('holyarrs-e2e');
	await expect(page.getByLabel('Access token')).toHaveValue('');
	await page.getByRole('button', { name: 'Cancel' }).click();
});

test('media servers: add Jellyfin, test it, a wrong key is said plainly', async () => {
	await page.goto('/settings');
	// The Linked apps card opens on Media servers.
	await page.getByTestId('settings-group').filter({ hasText: 'Linked apps' }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'Media servers' })).toBeVisible();
	await page.getByRole('button', { name: 'Add a media server' }).first().click();
	await page.getByLabel('Name', { exact: true }).fill('Lab Jellyfin');
	await page.getByLabel('Kind').selectOption('jellyfin');
	await page.getByLabel('Address').fill(`http://127.0.0.1:${jellyfinPort}`);
	await page.getByLabel('API key').fill(jellyKey);
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	const card = page.getByTestId('media-server').filter({ hasText: 'Lab Jellyfin' });
	await expect(card).toContainText('Told about imports, renames, deletes');
	await card.getByRole('button', { name: 'Test' }).click();
	await expect(card.getByTestId('media-test')).toHaveText('Jellyfin v10.9.0');
	await expect(card.getByTestId('media-result')).toContainText('Last checked');
	await card.getByRole('button', { name: 'Change' }).click();
	await page.getByLabel('API key').fill('wrong-key');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await card.getByRole('button', { name: 'Test' }).click();
	await expect(card.getByTestId('media-test')).toHaveText('Not connected');
	await expect(card.getByTestId('media-result')).toContainText(/the server turned down the token/i);
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
