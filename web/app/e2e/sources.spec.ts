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
	await page.getByRole('button', { name: 'Save Source' }).click();
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
	await page.getByRole('button', { name: 'Save Source' }).click();
	await row.getByRole('button', { name: 'Test' }).click();
	await expect(row.getByTestId('source-test')).toContainText('turned down the key');

	// Back to the right key, then pause and resume.
	await row.getByRole('button', { name: 'Change' }).click();
	await page.getByLabel('Key', { exact: true }).fill(indexerKey);
	await page.getByRole('button', { name: 'Save Source' }).click();
	await row.getByRole('button', { name: 'Pause' }).click();
	await expect(row.getByTestId('source-state')).toHaveText('Paused');
	await expect(row.getByTestId('source-text')).toContainText('Paused by you');
	await row.getByRole('button', { name: 'Start' }).click();
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
