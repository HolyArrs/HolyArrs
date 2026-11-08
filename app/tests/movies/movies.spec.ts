import { expect, test, type Browser, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { owner, signIn, libraryStatus, libraryView, addFromSearch, openFromSearch, openLibraryTab, ensureInLibrary } from '../helpers/helpers.ts';

// Add-movie screen, Library screen and Movie page against the fake TMDB in
// fakes/. Runs after the other projects, so the owner and the Movies
// folder exist and Driftwood Moon is already in the Library.
test.describe.configure({ mode: 'serial' });

let page: Page;
// Other projects in the chain add their own titles; counts look only at this file's four.
const ours = /Driftwood Moon|The Lantern Keepers|Paper Harbour|Quiet Orchard/;
const memberPw = `e2e-${Math.random().toString(36).slice(2)}-viewer`;

async function fresh(browser: Browser) {
	return (await browser.newContext()).newPage();
}

test.beforeAll(async ({ browser }) => {
	// Run on setup alone, nobody added Driftwood Moon yet; the counts below expect it.
	await ensureInLibrary(browser, 'Driftwood Moon', 'Driftwood Moon');
	page = await fresh(browser);
	await signIn(page, owner());
	// Lookups go through the fake metadata server with no key to set.
});

test('add a movie by name with folder, Pick rules and Release gate in under a minute', async () => {
	const started = Date.now();
	const dialog = await addFromSearch(page, 'lantern keepers', 'The Lantern Keepers');
	await expect(dialog.getByLabel('Library folder')).toHaveValue(/\d+/);
	await expect(dialog.getByLabel('Pick rules').locator('option')).not.toHaveCount(0);
	await dialog.getByLabel('Release gate').selectOption('in_cinemas');
	await expect(dialog.getByText('Look from the cinema date.')).toBeVisible();
	await dialog.getByLabel('Search when added').uncheck();
	await dialog.getByRole('button', { name: 'Add to Library' }).click();
	// Adding opens the movie's Library page.
	await expect(page).toHaveURL(/\/library\/\d+/);
	expect(Date.now() - started).toBeLessThan(60_000);

	// The movie page shows the choices and why it waits (its Library tab).
	await expect(page.getByRole('heading', { level: 1 })).toContainText('The Lantern Keepers');
	await openLibraryTab(page);
	await expect(page.getByTestId('gate-reason')).toContainText('closed until 2031-02-01');
	await expect(page.getByText('Not out yet').first()).toBeVisible();
});

test('add by tmdb: and imdb: id; the last Release gate is remembered, the rest start from the Movies and TV defaults', async () => {
	const dialog = await addFromSearch(page, 'tmdb:900002', 'Paper Harbour');
	await expect(dialog.getByLabel('Release gate')).toHaveValue('in_cinemas');
	// Search when added is not remembered: it starts from the defaults (on for movies when nothing is saved).
	await expect(dialog.getByLabel('Search when added')).toBeChecked();
	await dialog.getByLabel('Search when added').uncheck();
	await dialog.getByLabel('Release gate').selectOption('released');
	await dialog.getByRole('button', { name: 'Add to Library' }).click();
	await expect(page).toHaveURL(/\/library\/\d+/);

	const orchard = await addFromSearch(page, 'imdb:tt9000030', 'Quiet Orchard');
	await orchard.getByLabel('Search when added').uncheck();
	await orchard.getByRole('button', { name: 'Add to Library' }).click();
	await expect(page).toHaveURL(/\/library\/\d+/);
});

test('Library filters, find by name, posters and list', async () => {
	await page.goto('/library');
	await libraryView(page, 'Posters');
	const grid = page.locator('[data-testid="library-card"], [data-testid="library-row"]').filter({ hasText: ours });
	const rows = page.getByTestId('library-row').filter({ hasText: ours });
	await expect(grid).toHaveCount(4);
	await libraryStatus(page, 'not_out_yet');
	await expect(page).toHaveURL(/show=not_out_yet/);
	await expect(grid).toHaveCount(1);
	await expect(grid.first()).toContainText('The Lantern Keepers');

	await libraryStatus(page, 'missing');
	await expect(grid).toHaveCount(3);
	await expect(page.locator('.grid')).not.toContainText('The Lantern Keepers');

	await libraryStatus(page, 'better');
	await expect(grid).toHaveCount(0);

	await libraryStatus(page, 'all');
	await page.getByTestId('library-find-input').fill('harbour');
	await expect(grid).toHaveCount(1);
	await page.getByTestId('library-find-input').fill('');

	await libraryView(page, 'Banners');
	await expect(rows).toHaveCount(4);
	await page.reload();
	// remembered
	await expect(page.getByRole('radio', { name: 'Banners', exact: true })).toHaveAttribute('aria-checked', 'true');
	await expect(rows).toHaveCount(4);
	await libraryView(page, 'Posters');
	await expect(grid).toHaveCount(4);
});

test('Movie page: dates, why, file, refresh and edit', async () => {
	await page.goto('/library');
	await page.locator('[data-testid="library-card"], [data-testid="library-row"]', { hasText: 'Paper Harbour' }).click();
	await expect(page.getByRole('heading', { level: 1 })).toContainText('Paper Harbour');
	await expect(page.getByRole('heading', { name: 'About' })).toBeVisible();
	// The Library tab holds the file; a movie's Versions are its File cards (84547ff28).
	const lib = await openLibraryTab(page);
	// Out and kept up to date: HolyArrs is looking, so no gate line (it shows only while HolyArrs waits).
	await expect(page.getByTestId('gate-reason')).toHaveCount(0);
	await expect(lib.getByRole('heading', { name: 'File', exact: true })).toBeVisible();
	await expect(lib.getByText(/no file yet/i).first()).toBeVisible();

	await page.getByRole('button', { name: 'More actions' }).click();
	await page.getByRole('menuitem', { name: 'Refresh details' }).click();
	await expect(page.getByText('Names, dates and posters')).toBeVisible();

	await page.getByRole('button', { name: 'More actions' }).click();
	// The Edit panel left the movie page (6e4a82053); Keep up to date is a More actions item.
	await page.getByRole('menuitem', { name: 'Stop keeping up to date' }).click();
	await expect(page.getByTestId('gate-reason')).toContainText('Keep up to date is off');
	// There is no Release gate control; the gate is changed through the API and the reason follows it.
	const id = page.url().match(/\/library\/(\d+)/)![1];
	const me = (await (await page.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	const put = await page.request.put(`/api/v1/titles/${id}`, { headers: { 'X-CSRF-Token': me.csrf }, data: { release_gate: 'announced' } });
	expect(put.ok()).toBe(true);
	await page.reload();
	await openLibraryTab(page);
	await expect(page.getByTestId('gate-reason')).toContainText('Keep up to date is off');
	await expect(page.getByTestId('gate-reason')).toContainText('announced');
});

test('a member sees the movie but none of the staff controls', async ({ browser }) => {
	await page.goto('/settings/users');
	await page.getByRole('button', { name: 'Invite someone' }).click();
	await page.getByLabel('Note for yourself').fill('Viewer');
	await page.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await page.locator('.fresh code').textContent())!.trim();
	const m = await fresh(browser);
	await m.goto(new URL(link).pathname);
	await m.getByLabel('Name').fill('viewer');
	await m.getByLabel('Password', { exact: true }).fill(memberPw);
	await m.getByLabel('Confirm password').fill(memberPw);
	await m.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(m).toHaveURL(/\/$/);

	await m.goto('/library');
	await expect(m.locator('[data-testid="library-card"], [data-testid="library-row"]').filter({ hasText: ours })).toHaveCount(4);
	await expect(m.getByRole('link', { name: 'Add a movie' })).toHaveCount(0);
	await m.locator('[data-testid="library-card"], [data-testid="library-row"]', { hasText: 'The Lantern Keepers' }).click();
	await expect(m.getByRole('heading', { level: 1 })).toContainText('The Lantern Keepers');
	// The Library section (gate, Versions, Releases) is for people who manage the Library.
	await expect(m.getByTestId('gate-reason')).toHaveCount(0);
	for (const b of ['Search now', 'Keep up to date', 'Edit', 'Library', 'More actions']) await expect(m.getByRole('button', { name: b, exact: true })).toHaveCount(0);
	await expect(m.getByRole('heading', { name: 'Releases' })).toHaveCount(0);

	// Search: a Library movie opens its Library page, with no Add for a member.
	await openFromSearch(m, 'orchard', 'Quiet Orchard');
	await expect(m).toHaveURL(/\/library\/\d+/);
	await expect(m.getByRole('button', { name: /^Add/ })).toHaveCount(0);
});

test('add a movie with a second 4K Version; the Movie page shows both and can add and remove one', async () => {
	const dialog = await addFromSearch(page, 'glass meridian', 'Glass Meridian');
	await dialog.getByLabel('Also keep a second Version').check();
	await expect(dialog.getByLabel('Version name')).toHaveValue('4K');
	await expect(dialog.getByLabel('Pick rules for this Version').locator('option:checked')).toHaveText('4K');
	await expect(dialog.getByText('it gets its own folder named with')).toBeVisible();
	await dialog.getByLabel('Search when added').uncheck();
	await dialog.getByRole('button', { name: 'Add to Library' }).click();
	await expect(page).toHaveURL(/\/library\/\d+/);
	await openLibraryTab(page);

	// Every Version is a File card on the Library tab (the main one reads Main copy; one without a file says so).
	const versions = page.getByTestId('version-card');
	await expect(versions).toHaveCount(2);
	await expect(versions.nth(0)).toContainText('Main copy');
	await expect(versions.nth(1)).toContainText('4K');
	await expect(versions.nth(1)).toContainText('no file yet');

	// A third Version from Add a version beside the File cards, then take it off again.
	await page.getByRole('button', { name: 'Add a version', exact: true }).click();
	const vd = page.getByRole('dialog');
	await vd.getByLabel('Version name').fill('Small');
	await vd.getByRole('button', { name: 'Add version' }).click();
	await expect(versions).toHaveCount(3);
	page.once('dialog', (d) => d.accept());
	await page.getByRole('button', { name: 'Version actions for Small' }).click();
	await page.getByRole('menuitem', { name: 'Remove Small' }).click();
	await expect(versions).toHaveCount(2);
	await expect(page.getByRole('button', { name: 'Remove Main' })).toHaveCount(0);
	await expect(page.getByRole('button', { name: /^Version actions for Main/ })).toHaveCount(0);
});

test('the top-bar search opens Search with Library results; a movie not in the Library opens its details with Add', async () => {
	await page.goto('/');
	// The one search field sits in the top bar; typing opens the Search page with the words.
	const box = page.getByTestId('search-box');
	await box.click();
	await expect(page).toHaveURL(/\/search/);
	await box.fill('Paper Harbour');
	await expect(page).toHaveURL(/q=Paper/);
	const card = page.getByRole('main').getByRole('link', { name: /^Paper Harbour,/ }).first();
	await expect(card).toBeVisible();
	await card.click();
	await expect(page.getByRole('heading', { level: 1 })).toContainText('Paper Harbour');

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
