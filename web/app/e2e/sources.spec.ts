import { expect, test, type Page } from '@playwright/test';
import { owner, signIn } from './helpers.ts';
import { indexerKey, indexerPort } from './fakes.mjs';

// Sources against the fake Torznab Source and rss feed in fakes.mjs.
test.describe.configure({ mode: 'serial' });

let page: Page;
const base = `http://127.0.0.1:${indexerPort}`;

test.beforeAll(async ({ browser }) => {
	page = await (await browser.newContext()).newPage();
	await signIn(page, owner());
});

async function addSource(name: string, format: string, url: string, key = '') {
	await page.getByRole('button', { name: 'Add a Source' }).first().click();
	await page.getByLabel('Name', { exact: true }).fill(name);
	await page.getByLabel('Kind of feed').selectOption(format);
	await page.getByLabel('Address', { exact: true }).fill(url);
	if (key) await page.getByLabel('Key', { exact: true }).fill(key);
	await page.getByText('May reach the local network', { exact: true }).click();
	await page.getByRole('button', { name: 'Save', exact: true }).click();
}

test('a Source is added, tested, shows a wrong key in plain words, pauses and resumes', async () => {
	await page.goto('/settings/sources');
	await expect(page.getByRole('heading', { level: 1, name: 'Sources' })).toBeVisible();

	await addSource('Lab Torznab', 'torznab', `${base}/api`, indexerKey);
	const row = page.getByTestId('source').filter({ hasText: 'Lab Torznab' });
	await expect(row.getByTestId('source-test')).toContainText('The Source answered');
	await expect(row.getByTestId('source-state')).toHaveText('Working');
	await expect(row).not.toContainText(indexerKey);

	// A wrong key.
	await row.getByRole('button', { name: 'Change' }).click();
	await page.getByLabel('Key', { exact: true }).fill('not-the-right-key');
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await row.getByRole('button', { name: 'Test' }).click();
	await expect(row.getByTestId('source-test')).toContainText('turned down the key');

	// Back to the right key, then pause and resume.
	await row.getByRole('button', { name: 'Change' }).click();
	await page.getByLabel('Key', { exact: true }).fill(indexerKey);
	await page.getByRole('button', { name: 'Save', exact: true }).click();
	await row.getByRole('button', { name: 'Pause' }).click();
	await expect(row.getByTestId('source-state')).toHaveText('Paused');
	await expect(row.getByTestId('source-text')).toContainText('Paused by you');
	await row.getByRole('button', { name: 'Resume' }).click();
	await expect(row.getByTestId('source-state')).toHaveText('Working');
});

test('an rss feed is read and its passkey stays hidden', async () => {
	await addSource('Lab feed', 'rss', `${base}/rss?passkey=hidden-lab-pk`);
	const row = page.getByTestId('source').filter({ hasText: 'Lab feed' });
	await expect(row.getByTestId('source-test')).toContainText('The feed answered with 2 Releases');
	await expect(row).not.toContainText('hidden-lab-pk');
});

test('the health page shows state, last answer and calls over time', async () => {
	const row = page.getByTestId('source').filter({ hasText: 'Lab Torznab' });
	await row.getByRole('link', { name: 'Health' }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'Lab Torznab' })).toBeVisible();
	await expect(page.getByTestId('health-state')).toHaveText('Working');
	await expect(page.getByTestId('last-success')).not.toHaveText('Never');
	await expect(page.getByTestId('call')).toHaveCount(2);
	await expect(page.getByTestId('call').first()).toContainText('Test');
	await page.getByRole('link', { name: 'Sources' }).first().click();
	const row2 = page.getByTestId('source').filter({ hasText: 'Lab feed' });
	page.once('dialog', (d) => d.accept());
	await row2.getByRole('button', { name: 'Remove Lab feed' }).click();
	await expect(page.getByTestId('source').filter({ hasText: 'Lab feed' })).toHaveCount(0);
});

// Connect Jackett (#96): the guided setup tests the address and key, brings
// the site in as a Source, checks it again, and disconnects (asked first);
// a Source behind a bot check points to it. Only the fake Jackett and the
// fake indexer on this machine answer.
test('Jackett is connected in one step, kept in step, and disconnected; a bot check points to it', async () => {
	test.setTimeout(90_000);
	const me = (await (await page.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	const call = (method: string, path: string, data?: unknown) => page.request.fetch(`/api/v1${path}`, { method, data, headers: { 'X-CSRF-Token': me.csrf } });
	try {
		await page.goto('/settings/sources');
		const card = page.locator('#connect-bridge');
		await card.getByTestId('bridge-start').click();
		await card.getByLabel('Address').fill(`${base}/jackett`);
		await card.getByLabel('Key').fill('not-the-key');
		await card.getByTestId('bridge-test').click();
		await expect(card.getByTestId('bridge-result')).toContainText('Jackett turned down the key');
		await card.getByLabel('Key').fill(indexerKey);
		await card.getByTestId('bridge-test').click();
		await expect(card.getByTestId('bridge-result')).toContainText('Jackett · 1 site');
		await card.getByTestId('bridge-connect').click();
		await expect(page.getByText('Jackett connected: 1 Source added.')).toBeVisible();
		const row = card.getByTestId('bridge');
		await expect(row).toContainText('1 site, 1 Source');
		// The fake Jackett runs on this machine: its Sources may reach the local network, and the card says so.
		await expect(row.getByTestId('bridge-local')).toContainText('Can reach your home network');
		const src = page.getByTestId('source').filter({ hasText: 'Lab Jackett Site' });
		await expect(src).toContainText('/jackett/api/v2.0/indexers/labsite/results/torznab/');
		await expect(src).toContainText('may reach the local network');
		await row.getByTestId('bridge-check').click();
		await expect(page.getByText('Already in step.')).toBeVisible();

		// A site behind a bot check says so, and points here.
		await addSource('Lab Guarded', 'torznab', `${base}/guarded/api`, indexerKey);
		const guarded = page.getByTestId('source').filter({ hasText: 'Lab Guarded' });
		await expect(guarded.getByTestId('source-test')).toContainText("Blocked by the site's bot check. Reach this site through Jackett or Prowlarr.");
		await expect(guarded.getByTestId('source-bridge-link')).toHaveAttribute('href', '#connect-bridge');

		await row.getByTestId('bridge-remove').click();
		const sheet = page.getByRole('dialog', { name: 'Disconnect Jackett?' });
		await expect(sheet).toContainText('Remove its 1 Source');
		await sheet.getByRole('button', { name: 'Disconnect', exact: true }).click();
		await expect(page.getByText('Jackett disconnected.')).toBeVisible();
		await expect(card.getByTestId('bridge')).toHaveCount(0);
		await expect(page.getByTestId('source').filter({ hasText: 'Lab Jackett Site' })).toHaveCount(0);
	} finally {
		const bs = ((await (await call('GET', '/bridges')).json()) as { bridges: { id: number }[] }).bridges;
		for (const b of bs) await call('DELETE', `/bridges/${b.id}`);
		const list = ((await (await call('GET', '/sources')).json()) as { sources: { id: number; name: string }[] }).sources;
		for (const s of list.filter((x) => x.name === 'Lab Guarded' || x.name.startsWith('Lab Jackett Site'))) await call('DELETE', `/sources/${s.id}`);
	}
});

test('the Connect card fits a phone and passes the accessibility check', async () => {
	await page.goto('/settings/sources');
	const card = page.locator('#connect-bridge');
	await card.getByTestId('bridge-start').click();
	await card.getByText('Prowlarr', { exact: true }).click();
	// Opened at this machine's own address, which Prowlarr can't reach: the field starts empty.
	await expect(card.getByLabel('HolyArrs address')).toHaveValue('');
	const AxeBuilder = (await import('@axe-core/playwright')).default;
	expect((await new AxeBuilder({ page }).include('#connect-bridge').analyze()).violations).toEqual([]);
	await page.setViewportSize({ width: 360, height: 760 });
	await expect(card.getByTestId('bridge-connect')).toBeVisible();
	expect(await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBeLessThanOrEqual(0);
	await page.setViewportSize({ width: 1280, height: 720 });
});
