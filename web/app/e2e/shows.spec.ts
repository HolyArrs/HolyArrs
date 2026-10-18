import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Browser, type Page } from '@playwright/test';
import { join } from 'node:path';
import { moviesDir, noSidewaysScroll, owner, signIn, libraryStatus, openApprovals, openSeason, seasonCount, episodeAction, episodeKept, libraryKind } from './helpers.ts';

// Shows (2.2.5 to 2.2.10): the owner adds a show folder and a show from
// Search (the fake TMDB knows Harbour Lights, Lantern Street and Moss
// Parade), turns seasons and episodes on and off on the show page, finds it
// in the Library next to the movies, and a member asks for two seasons of
// another show from a phone, which the owner approves from a phone. Runs
// last: the owner, the TMDB key and the movies exist.
test.describe.configure({ mode: 'serial' });

let own: Page;
let max: Page;
const showsDir = join(moviesDir, '..', 'shows');
const maxPw = `e2e-${Math.random().toString(36).slice(2)}-max`;

async function fresh(browser: Browser, width = 1280): Promise<Page> {
	const ctx = await browser.newContext({ viewport: { width, height: width < 600 ? 915 : 900 } });
	return ctx.newPage();
}

async function axe(p: Page, where: string) {
	await p.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
	const r = await new AxeBuilder({ page: p }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	const bad = r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
	if (bad.length) console.log('AXE', where, bad.map((v) => v.id + ' ' + v.nodes.slice(0, 3).map((n) => n.target.join(' ')).join(' | ')).join(' ;; '));
	expect(bad.map((v) => v.id), where).toEqual([]);
}

async function searchShows(p: Page, name: string) {
	await p.goto('/search?kind=show');
	await expect(p.getByRole('radio', { name: 'TV shows', exact: true })).toHaveAttribute('aria-checked', 'true');
	await p.getByLabel('Show name or id').fill(name);
	await p.getByRole('button', { name: 'Search', exact: true }).click();
}

test.beforeAll(async ({ browser }) => {
	own = await fresh(browser);
	await signIn(own, owner());
});

test('the owner adds a Library folder for shows', async () => {
	await own.goto('/settings/library-folders');
	await own.getByRole('button', { name: 'Add a folder' }).first().click();
	await own.getByLabel('Holds').selectOption('show');
	await own.getByLabel('Folder path').fill(showsDir);
	await own.getByLabel(/^Name/).fill('Shows');
	await own.getByRole('button', { name: 'Add and check' }).click();
	await expect(own.getByTestId('folder-kind').filter({ hasText: 'For shows' })).toHaveCount(1);
});

test('the owner finds a show and adds it with only the latest season kept', async () => {
	// Search switches between movies and shows and keeps the choice in the address.
	await own.goto('/search');
	await own.getByRole('radio', { name: 'TV shows', exact: true }).click();
	await expect(own).toHaveURL(/kind=show/);
	await own.getByLabel('Show name or id').fill('harbour');
	await own.getByRole('button', { name: 'Search', exact: true }).click();
	const hit = own.getByTestId('show-hit').filter({ hasText: 'Harbour Lights' });
	await expect(hit).toContainText('Lab TV');
	await axe(own, 'search shows');
	await hit.getByRole('button', { name: 'Add' }).click();

	const dialog = own.getByRole('dialog');
	await expect(dialog.getByLabel('Library folder')).toContainText('Shows');
	await expect(dialog.getByTestId('no-show-search')).toContainText('use Search now on the show page');
	await expect(dialog.getByRole('checkbox', { name: /Search now/ })).toHaveCount(0);
	// Search when added starts off for shows while no Movies and TV default says otherwise.
	await expect(dialog.getByRole('checkbox', { name: /^Search when added/ })).not.toBeChecked();
	await dialog.getByLabel('Seasons to keep up to date').selectOption('latest');
	await expect(dialog).toContainText('Only the highest-numbered season.');
	await dialog.getByLabel('Episode order').selectOption('aired');
	await dialog.getByLabel('Details from').selectOption('tmdb');
	await axe(own, 'add show dialog');
	await dialog.getByRole('button', { name: 'Add to Library' }).click();
	await expect(dialog).toBeHidden();
	await expect(own.getByRole('status')).toContainText('Harbour Lights is in your Library.');
	await hit.getByRole('link', { name: 'In your Library' }).click();

	// The show page: seasons and their switches; Search now is there (3.2).
	await expect(own.getByRole('heading', { level: 1, name: /Harbour Lights/ })).toBeVisible();
	await expect(own.getByTestId('search-show')).toBeVisible();
	// One season shows at a time; the Season picker holds all three.
	expect(await seasonCount(own)).toBe(3);
	await openSeason(own, 0);
	await expect(own.getByRole('checkbox', { name: 'Keep Specials up to date' })).not.toBeChecked();
	await openSeason(own, 1);
	await expect(own.getByRole('checkbox', { name: 'Keep Season 1 up to date' })).not.toBeChecked();
	const s2 = await openSeason(own, 2);
	await expect(own.getByRole('checkbox', { name: 'Keep Season 2 up to date' })).toBeChecked();
	await expect(s2.getByTestId('season-line')).toContainText('0 of 1 aired in the Library · 1 missing · 1 not out yet');
	await axe(own, 'show page');
});

test('seasons and episodes turn on and off; each episode says its state and why', async () => {
	await openSeason(own, 1);
	await own.getByText('Keep Season 1 up to date', { exact: true }).click();
	await expect(own.getByRole('status')).toContainText('Season 1: HolyArrs keeps it up to date.');
	await expect(own.getByRole('checkbox', { name: 'Keep Season 1 up to date' })).toBeChecked();
	await expect(own.getByTestId('season').getByTestId('season-line')).toContainText('2 missing');

	const eps = (await openSeason(own, 2)).getByTestId('episode');
	await expect(eps).toHaveCount(2);
	await expect(eps.nth(0)).toContainText('S02E01');
	await expect(eps.nth(0)).toContainText('Return of the Fog');
	await expect(eps.nth(0)).toContainText('Missing');
	await expect(eps.nth(1)).toContainText('Not out yet');
	await expect(eps.nth(1).getByTestId('episode-why')).toContainText('Counts as missing once it airs');
	await episodeAction(own, 'S02E01', 'Keep S02E01 up to date');
	await expect.poll(() => episodeKept(own, 'S02E01')).toBe(false);
	await own.reload();
	await openSeason(own, 2);
	expect(await episodeKept(own, 'S02E01')).toBe(false);
	await expect(own.getByTestId('season').getByTestId('episode').nth(0).getByTestId('episode-why')).toContainText('Keep up to date is off for this episode.');
	await episodeAction(own, 'S02E01', 'Keep S02E01 up to date');
	await expect.poll(() => episodeKept(own, 'S02E01')).toBe(true);
});

test('shows sit in the Library next to movies, with kinds and state filters', async () => {
	await own.goto('/library');
	await libraryKind(own, /^Shows/);
	await expect(own).toHaveURL(/kind=show/);
	const card = own.getByTestId('library-card').filter({ hasText: 'Harbour Lights' });
	await expect(card).toContainText('Show');
	await expect(card).toContainText('missing');
	await expect(own.getByTestId('library-card').filter({ hasText: 'Quiet Orchard' })).toHaveCount(0);
	await libraryStatus(own, 'missing');
	await expect(card).toBeVisible();
	// In the Missing list a show folds its missing episodes under one row.
	await own.getByRole('radiogroup', { name: 'Layout' }).getByRole('radio', { name: 'List' }).click();
	const missingRow = own.getByTestId('library-row').filter({ hasText: 'Harbour Lights' });
	await missingRow.getByTestId('missing-fold').click();
	await expect(missingRow.getByTestId('missing-fold')).toHaveAttribute('aria-expanded', 'true');
	await expect(missingRow.getByTestId('missing-episode').first()).toContainText(/S\d\dE\d\d/);
	await own.getByRole('radiogroup', { name: 'Layout' }).getByRole('radio', { name: 'Posters' }).click();
	await libraryStatus(own, 'not_out_yet');
	await expect(card).toBeVisible();
	await libraryStatus(own, 'better');
	await expect(card).toHaveCount(0);
	await libraryStatus(own, 'all');
	await own.getByRole('radiogroup', { name: 'Layout' }).getByRole('radio', { name: 'List' }).click();
	await expect(own.getByTestId('library-row').filter({ hasText: 'Harbour Lights' })).toContainText('aired episodes in the Library');
	await own.getByRole('radiogroup', { name: 'Layout' }).getByRole('radio', { name: 'Posters' }).click();
	await libraryKind(own, /^Movies/);
	await expect(card).toHaveCount(0);
	await expect(own.getByTestId('library-card').filter({ hasText: 'Quiet Orchard' })).toBeVisible();
	await libraryKind(own, /^Everything/);
	await axe(own, 'library with shows');
});

test('a member asks for two seasons of a show from a phone', async ({ browser }) => {
	await own.goto('/settings/users');
	await own.getByLabel('Note for yourself').fill('Max');
	await own.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await own.locator('.fresh code').textContent())!.trim();
	max = await fresh(browser, 412);
	await max.goto(new URL(link).pathname);
	await max.getByLabel('Name', { exact: true }).fill('max');
	await max.getByLabel('Password', { exact: true }).fill(maxPw);
	await max.getByLabel('Password again').fill(maxPw);
	await max.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(max).toHaveURL(/\/$/);

	await searchShows(max, 'lantern');
	const hit = max.getByTestId('show-hit').filter({ hasText: 'Lantern Street' });
	await expect(max.getByTestId('ask-limit')).toContainText('show asks left this week');
	await noSidewaysScroll(max);
	await hit.getByRole('button', { name: 'Ask', exact: true }).click();
	const dialog = max.getByRole('dialog');
	await dialog.getByRole('radio', { name: /Only some seasons/ }).check();
	await expect(dialog.getByRole('button', { name: 'Pick a season' })).toBeDisabled();
	await dialog.getByRole('checkbox', { name: 'Season 1' }).check();
	await dialog.getByRole('checkbox', { name: 'Season 2' }).check();
	await noSidewaysScroll(max);
	await axe(max, 'ask show dialog phone');
	await dialog.getByRole('button', { name: 'Ask for seasons 1 and 2' }).click();
	await expect(dialog).toBeHidden();
	await expect(hit.getByRole('link', { name: /^Requested/ })).toBeVisible();

	// A show already in the Library offers to ask for the seasons it does not keep.
	await searchShows(max, 'harbour');
	await expect(max.getByTestId('show-hit').filter({ hasText: 'Harbour Lights' }).getByRole('button', { name: 'Ask for more' })).toBeVisible();

	await max.locator('nav.tabbar').getByRole('link', { name: 'Wishlist' }).click();
	const ask = max.getByTestId('ask').filter({ hasText: 'Lantern Street' });
	await expect(ask.getByTestId('ask-state')).toHaveText('Requested');
	await expect(ask.getByTestId('ask-kind')).toHaveText('Show');
	await expect(ask.getByTestId('ask-seasons')).toHaveText('Asked for seasons 1 and 2');
	await expect(ask.getByRole('button', { name: 'Approve' })).toHaveCount(0);
	await noSidewaysScroll(max);
	await axe(max, 'wishlist member phone');
});

test('the owner approves from a phone: the show is added with those seasons and max is told', async ({ browser }) => {
	const phone = await fresh(browser, 412);
	await signIn(phone, owner());
	await openApprovals(phone);
	const ask = phone.getByTestId('ask').filter({ hasText: 'Lantern Street' });
	await expect(ask.getByTestId('asked-by')).toContainText('Asked by max');
	await ask.getByRole('button', { name: 'Approve' }).click();
	const dialog = phone.getByRole('dialog');
	await expect(dialog).toContainText('for seasons 1 and 2');
	await expect(dialog.getByLabel('Library folder')).toContainText('Shows');
	await expect(dialog.getByLabel('Library folder')).not.toContainText('Movies');
	await expect(dialog).toContainText('keeps seasons 1 and 2 up to date');
	await dialog.getByLabel('Note (optional)').fill('Seasons 1 and 2 coming up');
	await noSidewaysScroll(phone);
	await dialog.getByRole('button', { name: 'Approve and add' }).click();
	await expect(dialog).toBeHidden();
	await phone.getByRole('button', { name: /^Everyone's/ }).click();
	await expect(ask.getByTestId('ask-state')).toHaveText('Approved');
	await expect(ask.getByTestId('decided-by')).toContainText('Approved by luis');

	// The show page names who asked and keeps exactly the asked seasons.
	await ask.getByRole('link', { name: /Lantern Street/ }).click();
	await expect(phone.getByTestId('title-asks')).toContainText('max');
	await expect(phone.getByTestId('title-asks')).toContainText('seasons 1 and 2');
	await openSeason(phone, 0);
	await expect(phone.getByRole('checkbox', { name: 'Keep Season 1 up to date' })).toBeChecked();
	await openSeason(phone, 1);
	await expect(phone.getByRole('checkbox', { name: 'Keep Season 2 up to date' })).toBeChecked();
	await openSeason(phone, 2);
	await expect(phone.getByRole('checkbox', { name: 'Keep Season 3 up to date' })).not.toBeChecked();
	await noSidewaysScroll(phone);
	await axe(phone, 'show page phone');

	// History names who asked and who approved.
	await own.goto('/downloads/history');
	await expect(own.getByText('max asked for Lantern Street (2023), seasons 1 and 2').first()).toBeVisible();
	await expect(own.getByText(/Lantern Street \(2023\), seasons 1 and 2: approved by luis/).first()).toBeVisible();

	// max reads the message; members see no switches on the show page.
	await max.goto('/wishlist');
	await expect(max.getByTestId('message').first()).toContainText('Lantern Street (2023), seasons 1 and 2 was approved by luis');
	await expect(max.getByTestId('message').first()).toContainText('Seasons 1 and 2 coming up');
	await max.getByTestId('ask').filter({ hasText: 'Lantern Street' }).getByRole('link', { name: /Lantern Street/ }).click();
	expect(await seasonCount(max)).toBe(3);
	await expect(max.getByRole('checkbox')).toHaveCount(0);
	await expect(max.getByRole('button', { name: /^Episode actions for/ })).toHaveCount(0);
	await expect(max.getByRole('button', { name: 'Edit' })).toHaveCount(0);
	await noSidewaysScroll(max);
});
