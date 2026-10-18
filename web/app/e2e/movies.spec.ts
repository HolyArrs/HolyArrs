import { expect, test, type Browser, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { owner, signIn, libraryStatus } from './helpers.ts';

// Add-movie screen, Library screen and Movie page against the fake TMDB in
// fakes.mjs. Runs after the other projects, so the owner and the Movies
// folder exist and Driftwood Moon is already in the Library.
test.describe.configure({ mode: 'serial' });

let page: Page;
const memberPw = `e2e-${Math.random().toString(36).slice(2)}-viewer`;

async function fresh(browser: Browser) {
	return (await browser.newContext()).newPage();
}

test.beforeAll(async ({ browser }) => {
	page = await fresh(browser);
	await signIn(page, owner());
	// Point HolyArrs at the fake TMDB: any key works there.
	await page.goto('/settings/tmdb');
	await page.getByLabel(/the key/).fill('stand-in-for-tests');
	await page.getByRole('button', { name: 'Save the key' }).click();
	await expect(page.getByText('Saved', { exact: true })).toBeVisible();
});

test('add a movie by name with folder, Pick rules and Release gate in under a minute', async () => {
	const started = Date.now();
	await page.goto('/search');
	await page.getByLabel('Movie name or id').fill('lantern keepers');
	await page.getByRole('button', { name: 'Search', exact: true }).click();
	const hit = page.locator('li.hit', { hasText: 'The Lantern Keepers' });
	await expect(hit).toBeVisible();
	await expect(hit.locator('.poster')).toBeVisible();
	await hit.getByRole('button', { name: 'Add' }).click();

	const dialog = page.getByRole('dialog');
	await expect(dialog.getByLabel('Library folder')).toHaveValue(/\d+/);
	await expect(dialog.getByLabel('Pick rules').locator('option')).not.toHaveCount(0);
	await dialog.getByLabel('Release gate').selectOption('in_cinemas');
	await expect(dialog.getByText('Look from the cinema date.')).toBeVisible();
	await dialog.getByLabel('Search now').uncheck();
	await dialog.getByRole('button', { name: 'Add to Library' }).click();
	await expect(hit.getByRole('link', { name: 'In your Library' })).toBeVisible();
	expect(Date.now() - started).toBeLessThan(60_000);

	// The movie page shows the choices and why it waits.
	await hit.getByRole('link', { name: 'In your Library' }).click();
	await expect(page.getByRole('heading', { level: 1 })).toContainText('The Lantern Keepers');
	await expect(page.getByTestId('gate-reason')).toContainText('closed until 2031-02-01');
	await expect(page.getByText('Not out yet').first()).toBeVisible();
});

test('add by tmdb: and imdb: id; the last choices are remembered', async () => {
	await page.goto('/search');
	await page.getByLabel('Movie name or id').fill('tmdb:900002');
	await page.getByRole('button', { name: 'Search', exact: true }).click();
	const hit = page.locator('li.hit', { hasText: 'Paper Harbour' });
	await hit.getByRole('button', { name: 'Add' }).click();
	const dialog = page.getByRole('dialog');
	await expect(dialog.getByLabel('Release gate')).toHaveValue('in_cinemas');
	await expect(dialog.getByLabel('Search now')).not.toBeChecked();
	await dialog.getByLabel('Release gate').selectOption('released');
	await dialog.getByRole('button', { name: 'Add to Library' }).click();
	await expect(hit.getByRole('link', { name: 'In your Library' })).toBeVisible();
	// The add sheet finishes closing before the next search is typed.
	await expect(dialog).toBeHidden();

	await page.getByLabel('Movie name or id').fill('imdb:tt9000030');
	await page.getByRole('button', { name: 'Search', exact: true }).click();
	const orchard = page.locator('li.hit', { hasText: 'Quiet Orchard' });
	await orchard.getByRole('button', { name: 'Add' }).click();
	await page.getByRole('dialog').getByRole('button', { name: 'Add to Library' }).click();
	await expect(orchard.getByRole('link', { name: 'In your Library' })).toBeVisible();
});

test('Library filters, find by name, posters and list', async () => {
	await page.goto('/library');
	const grid = page.locator('.grid .item');
	await expect(grid).toHaveCount(4);
	await libraryStatus(page, 'not_out_yet');
	await expect(page).toHaveURL(/show=not_out_yet/);
	await expect(grid).toHaveCount(1);
	await expect(grid.first()).toContainText('The Lantern Keepers');

	await libraryStatus(page, 'missing');
	await expect(grid).toHaveCount(3);
	await expect(page.locator('.grid')).not.toContainText('The Lantern Keepers');

	await libraryStatus(page, 'better');
	await expect(page.getByText('Nothing matches this filter.')).toBeVisible();

	await libraryStatus(page, 'all');
	await page.getByLabel('Find in the Library').fill('harbour');
	await expect(grid).toHaveCount(1);
	await page.getByLabel('Find in the Library').fill('');

	await page.getByRole('radio', { name: 'List', exact: true }).click();
	await expect(page.getByTestId('library-row')).toHaveCount(4);
	await page.reload();
	// remembered
	await expect(page.getByRole('radio', { name: 'List', exact: true })).toHaveAttribute('aria-checked', 'true');
	await expect(page.getByTestId('library-row')).toHaveCount(4);
	await page.getByRole('radio', { name: 'Posters' }).click();
	await expect(grid).toHaveCount(4);
});

test('Movie page: dates, why, file, refresh and edit', async () => {
	await page.goto('/library');
	await page.locator('.grid .item', { hasText: 'Paper Harbour' }).click();
	await expect(page.getByRole('heading', { level: 1 })).toContainText('Paper Harbour');
	await expect(page.getByTestId('gate-reason')).toContainText('open since 2023-07-01');
	await expect(page.getByRole('heading', { name: 'About' })).toBeVisible();
	await expect(page.getByText('Out on digital or disc', { exact: true })).toBeVisible();
	await expect(page.getByRole('heading', { name: 'Versions and files' })).toBeVisible();
	await expect(page.getByText('No file yet.')).toBeVisible();

	await page.getByRole('button', { name: 'More actions' }).click();
	await page.getByRole('menuitem', { name: 'Refresh from TMDB' }).click();
	await expect(page.getByText('Refreshing names, dates and posters')).toBeVisible();

	await page.getByRole('button', { name: 'Edit', exact: true }).click();
	const dialog = page.getByRole('dialog');
	await dialog.getByLabel('Release gate').selectOption('announced');
	await dialog.getByLabel('Keep up to date').uncheck();
	await dialog.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByTestId('gate-reason')).toContainText('Keep up to date is off');
	await expect(page.getByTestId('gate-reason')).toContainText('announced');
});

test('a member sees the movie but none of the staff controls', async ({ browser }) => {
	await page.goto('/settings/users');
	await page.getByLabel('Note for yourself').fill('Viewer');
	await page.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await page.locator('.fresh code').textContent())!.trim();
	const m = await fresh(browser);
	await m.goto(new URL(link).pathname);
	await m.getByLabel('Name').fill('viewer');
	await m.getByLabel('Password', { exact: true }).fill(memberPw);
	await m.getByLabel('Password again').fill(memberPw);
	await m.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(m).toHaveURL(/\/$/);

	await m.goto('/library');
	await expect(m.locator('.grid .item')).toHaveCount(4);
	await expect(m.getByRole('link', { name: 'Add a movie' })).toHaveCount(0);
	await m.locator('.grid .item', { hasText: 'The Lantern Keepers' }).click();
	await expect(m.getByTestId('gate-reason')).toBeVisible();
	for (const b of ['Search now', 'Keep up to date', 'Edit', 'Library', 'More actions']) await expect(m.getByRole('button', { name: b, exact: true })).toHaveCount(0);
	await expect(m.getByRole('heading', { name: 'Releases' })).toHaveCount(0);

	await m.goto('/search');
	await m.getByLabel('Movie name or id').fill('orchard');
	await m.getByRole('button', { name: 'Search', exact: true }).click();
	await expect(m.locator('li.hit', { hasText: 'Quiet Orchard' }).getByRole('link', { name: 'In your Library' })).toBeVisible();
	await expect(m.getByRole('button', { name: 'Add' })).toHaveCount(0);
});

test('add a movie with a second 4K Version; the Movie page shows both and can add and remove one', async () => {
	await page.goto('/search');
	await page.getByLabel('Movie name or id').fill('glass meridian');
	await page.getByRole('button', { name: 'Search', exact: true }).click();
	const hit = page.locator('li.hit', { hasText: 'Glass Meridian' });
	await hit.getByRole('button', { name: 'Add' }).click();
	const dialog = page.getByRole('dialog');
	await dialog.getByLabel('Also keep a second Version').check();
	await expect(dialog.getByLabel('Version name')).toHaveValue('4K');
	await expect(dialog.getByLabel('Pick rules for this Version').locator('option:checked')).toHaveText('4K');
	await expect(dialog.getByText('it gets its own folder named with')).toBeVisible();
	await dialog.getByLabel('Search now').uncheck();
	await dialog.getByRole('button', { name: 'Add to Library' }).click();
	await hit.getByRole('link', { name: 'In your Library' }).click();

	const versions = page.getByTestId('version');
	await expect(versions).toHaveCount(2);
	await expect(versions.nth(0)).toContainText('(main)');
	await expect(versions.nth(1)).toContainText('4K');
	await expect(versions.nth(1)).toContainText('Pick rules: 4K');
	await expect(versions.nth(1)).toContainText('[4K]');

	// A third Version from the Movie page, then take it off again.
	await page.getByRole('button', { name: 'Add a Version' }).click();
	const vd = page.getByRole('dialog');
	await vd.getByLabel('Version name').fill('Small');
	await vd.getByRole('button', { name: 'Add Version' }).click();
	await expect(versions).toHaveCount(3);
	page.once('dialog', (d) => d.accept());
	await page.getByRole('button', { name: 'Remove Small' }).click();
	await expect(versions).toHaveCount(2);
	await expect(page.getByRole('button', { name: 'Remove Main' })).toHaveCount(0);
});

test('the search palette shows Library and movie results; a movie not in the Library opens its details with Add', async () => {
	await page.goto('/');
	await page.keyboard.press('Control+k');
	const box = page.getByRole('combobox', { name: 'Search titles or go to a place' });
	await box.fill('Paper Harbour');
	const library = page.getByTestId('palette-library');
	await expect(library.getByRole('option', { name: /Paper Harbour/ })).toContainText('In Library');
	await expect(page.getByRole('option', { name: 'See all results for “Paper Harbour”' })).toBeVisible();
	await box.press('ArrowDown');
	await box.press('Enter');
	await expect(page).toHaveURL(/\/library\/\d+$/);

	// Iron Lullaby is not in the Library yet (downloaders.spec adds it later).
	await page.goto('/title/movie/900011');
	const detail = page.getByTestId('title-detail');
	await expect(detail.getByRole('heading', { level: 1, name: 'Iron Lullaby' })).toBeVisible();
	await expect(detail).toContainText('A lullaby forged for a sleeping city.');
	await detail.getByRole('button', { name: 'Add Iron Lullaby' }).click();
	const dialog = page.getByRole('dialog', { name: 'Add Iron Lullaby' });
	await expect(dialog.getByLabel('Library folder')).toBeVisible();
	await dialog.getByRole('button', { name: 'Cancel' }).click();
	await expect(dialog).toBeHidden();
	await page.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
	const r = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	expect(r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').map((v) => v.id), 'title details').toEqual([]);
});
