import { expect, test, type Page } from '@playwright/test';
import { owner, signIn } from '../helpers/helpers.ts';
import { traktKey } from '../fakes/index.mjs';

// Outside lists settings against the fake TMDB and fake Trakt in fakes/.
// Runs after the movie tests, so the owner, the Movies folder and the TMDB
// key exist.
test.describe.configure({ mode: 'serial' });

let page: Page;

test.beforeAll(async ({ browser }) => {
	page = await (await browser.newContext()).newPage();
	await signIn(page, owner());
});

test('a TMDB list adds its movies by itself and skips the Never-add list', async () => {
	await page.goto('/settings');
	await page.getByTestId('settings-group').filter({ hasText: 'Library' }).click();
	await page.getByRole('navigation', { name: 'Settings sections' }).getByRole('link', { name: 'Outside lists', exact: true }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'Outside lists' })).toBeVisible();
	await expect(page.getByText('No Outside lists', { exact: true })).toBeVisible();

	// Salt Choir must never be added.
	await page.getByLabel('TMDB number').fill('900005');
	await page.getByLabel('Name', { exact: true }).fill('Salt Choir');
	await page.getByLabel('Reason').fill('not for this house');
	await page.getByRole('button', { name: 'Add to the Never-add list' }).click();
	await expect(page.getByTestId('never-add')).toContainText('Salt Choir');

	await page.getByRole('button', { name: 'Add a list' }).first().click();
	await page.getByLabel('Name', { exact: true }).first().fill('Lab list');
	await page.getByLabel('Which list').fill('https://www.themoviedb.org/list/501-lab-list');
	await page.locator('form.sc-form').getByRole('button', { name: 'Save', exact: true }).click();
	const row = page.getByTestId('outside-list');
	await expect(row).toContainText('Lab list');
	await expect(row).toContainText('read every 12 hours');
	await expect(row.getByTestId('list-result')).toContainText('1 added', { timeout: 15_000 });
	await expect(row.getByTestId('list-result')).toContainText('1 on the Never-add list');

	await page.goto('/library');
	await expect(page.locator('[data-testid="library-card"], [data-testid="library-row"]', { hasText: 'Ember Road' })).toBeVisible();
	await expect(page.locator('[data-testid="library-card"], [data-testid="library-row"]', { hasText: 'Salt Choir' })).toHaveCount(0);
});

test('a Trakt list needs the Trakt key, then adds its movie', async () => {
	await page.goto('/settings/outside-lists');
	await page.getByRole('button', { name: 'Add a list' }).first().click();
	await page.getByLabel('Name', { exact: true }).first().fill('Trakt favourites');
	await page.getByLabel('Kind').selectOption('trakt_list');
	await expect(page.getByText('Add a Trakt key below first')).toBeVisible();
	await page.getByLabel('Which list').fill('lab/favourites');
	await page.locator('form.sc-form').getByRole('button', { name: 'Save', exact: true }).click();
	const row = page.getByTestId('outside-list').filter({ hasText: 'Trakt favourites' });
	await expect(row).toContainText('Trakt key', { timeout: 15_000 });

	await page.getByLabel('Paste the Trakt key').fill(traktKey);
	await page.locator('form').filter({ has: page.locator('#trakt') }).getByRole('button', { name: 'Save', exact: true }).click();
	await expect(page.getByText('Trakt key saved').first()).toBeVisible();
	await expect(page.locator('body')).not.toContainText(traktKey);
	await row.getByRole('button', { name: 'Read now' }).click();
	await expect(row.getByTestId('list-result')).toContainText('1 added', { timeout: 15_000 });
	await page.goto('/library');
	await expect(page.locator('[data-testid="library-card"], [data-testid="library-row"]', { hasText: 'Northern Kettle' })).toBeVisible();
});

test('change, turn off and remove a list; take a movie off the Never-add list', async () => {
	await page.goto('/settings/outside-lists');
	const row = page.getByTestId('outside-list').filter({ hasText: 'Lab list' });
	await row.getByRole('button', { name: 'Change' }).click();
	await page.getByLabel('Read every (hours)').fill('48');
	await page.getByRole('checkbox', { name: /^Read this list/ }).uncheck();
	await page.locator('form.sc-form').getByRole('button', { name: 'Save', exact: true }).click();
	await expect(row).toContainText('every 2 days');
	await expect(row).toContainText('Off');
	await expect(row.getByRole('button', { name: 'Read now' })).toBeDisabled();

	page.once('dialog', (d) => d.accept());
	await row.getByRole('button', { name: 'Remove' }).click();
	await expect(page.getByTestId('outside-list').filter({ hasText: 'Lab list' })).toHaveCount(0);

	await page.getByRole('button', { name: 'Take off Salt Choir' }).click();
	await expect(page.getByTestId('never-add')).toHaveCount(0);
});
