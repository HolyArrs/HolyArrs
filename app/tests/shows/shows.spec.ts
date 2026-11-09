import { expect, test, type Browser, type Page } from '@playwright/test';
import { join } from 'node:path';
import { moviesDir, noSidewaysScroll, owner, signIn, libraryStatus, openRequests, openSeason, openSeasonNamed, seasonCount, episodeAction, episodeKept, episodeState, libraryKind, libraryView, addFromSearch, openFromSearch, seasonKept, toggleSeasonKept } from '../helpers/helpers.ts';
import { axe } from '../helpers/axe.ts';

// Shows (2.2.5 to 2.2.10): the owner adds a show folder and a show from
// Search (the fake TMDB knows Harbour Lights, Lantern Street and Moss
// Parade), turns seasons and episodes on and off on the show page, finds it
// in the Library next to the movies, and a member asks for two seasons of
// another show from a phone, which the owner approves from a phone. Runs
// last: the owner and the movies exist.
test.describe.configure({ mode: 'serial' });

let own: Page;
let max: Page;
const showsDir = join(moviesDir, '..', 'shows');
const maxPw = `e2e-${Math.random().toString(36).slice(2)}-max`;

async function fresh(browser: Browser, width = 1280): Promise<Page> {
	const ctx = await browser.newContext({ viewport: { width, height: width < 600 ? 915 : 900 } });
	return ctx.newPage();
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
	await own.getByRole('button', { name: 'Add', exact: true }).click();
	await expect(own.getByTestId('folder-kind').filter({ hasText: 'For shows' })).toHaveCount(1);
});

test('the owner finds a show and adds it with only the latest season kept', async () => {
	// Search switches between movies and shows and keeps the choice in the address.
	await own.goto('/search?q=harbour');
	await own.getByRole('radio', { name: 'TV shows', exact: true }).click();
	await expect(own).toHaveURL(/kind=show/);
	await expect(own.getByRole('main').getByRole('link', { name: 'Harbour Lights,' }).first()).toBeVisible();
	await axe(own, 'search shows');
	// A show opens its title page; Add lives there.
	const dialog = await addFromSearch(own, 'harbour', 'Harbour Lights', 'show');
	await expect(dialog.getByLabel('Library folder')).toContainText('Shows');
	await expect(dialog.getByTestId('no-show-search')).toContainText('Tick Search when added to start now');
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
	await expect(own.getByRole('status')).toContainText('Harbour Lights added');
	await expect(own).toHaveURL(/\/library\/\d+/);

	// The show page: seasons and their switches; Search now is there (3.2).
	await expect(own.getByRole('heading', { level: 1, name: /Harbour Lights/ })).toBeVisible();
	await own.getByRole('button', { name: 'More actions', exact: true }).click();
	await expect(own.getByRole('menuitem', { name: 'Search now', exact: true })).toBeVisible();
	await own.keyboard.press('Escape');
	// The Library tab lists all three seasons, each with its keep switch.
	expect(await seasonCount(own)).toBe(3);
	expect(await seasonKept(own, 'Specials')).toBe(false);
	expect(await seasonKept(own, 'Season 1')).toBe(false);
	expect(await seasonKept(own, 'Season 2')).toBe(true);
	// Season 2: one episode aired and missing, one not out yet.
	const s2 = await openSeason(own, 2);
	await expect(s2.locator('.lib-count')).toHaveText('0 / 2');
	await expect(await episodeState(own, 'S02E01')).toHaveText('Missing');
	await expect(await episodeState(own, 'S02E02')).toHaveText('Not out yet');
	await axe(own, 'show page');
});

test('seasons and episodes turn on and off; each episode says its state and why', async () => {
	await toggleSeasonKept(own, 'Season 1');
	expect(await seasonKept(own, 'Season 1')).toBe(true);
	// Both of its episodes are missing, so Get the missing episodes is offered.
	const s1 = await openSeasonNamed(own, 'Season 1');
	await expect(s1.locator('.lib-count')).toHaveText('0 / 2');
	await expect(s1.getByRole('button', { name: 'Get the missing episodes of season 1' })).toBeEnabled();

	const s2 = await openSeason(own, 2);
	const eps = s2.getByRole('button', { name: /^Episode \d+:/ });
	await expect(eps).toHaveCount(2);
	await expect(eps.nth(0)).toHaveAccessibleName('Episode 1: Return of the Fog');
	await expect(s2.locator('.lib-ep-st')).toHaveText(['Missing', 'Not out yet']);
	// An episode not out yet cannot be looked for yet.
	await expect(s2.getByRole('button', { name: 'Look for episode 2' })).toBeDisabled();
	await episodeAction(own, 'S02E01', 'Keep S02E01 up to date');
	await expect.poll(() => episodeKept(own, 'S02E01')).toBe(false);
	await own.reload();
	expect(await episodeKept(own, 'S02E01')).toBe(false);
	await episodeAction(own, 'S02E01', 'Keep S02E01 up to date');
	await expect.poll(() => episodeKept(own, 'S02E01')).toBe(true);
});

test('shows sit in the Library next to movies, with kinds and state filters', async () => {
	await own.goto('/library');
	await libraryKind(own, /^Shows/);
	await expect(own).toHaveURL(/kind=show/);
	await libraryView(own, 'Posters');
	const card = own.getByTestId('library-card').filter({ hasText: 'Harbour Lights' });
	// The Shows kind filter is on, so a card here is a show.
	await expect(card).toBeVisible();
	// Posters carry only the status circle now; the Missing filter below proves the state.
	await expect(own.getByTestId('library-card').filter({ hasText: 'Quiet Orchard' })).toHaveCount(0);
	await libraryStatus(own, 'missing');
	await expect(card).toBeVisible();
	// In the Missing list a show folds its missing episodes under one row.
	await libraryView(own, 'Banners');
	const missingRow = own.getByTestId('library-row').filter({ hasText: 'Harbour Lights' });
	await missingRow.getByTestId('missing-fold').click();
	await expect(missingRow.getByTestId('missing-fold')).toHaveAttribute('aria-expanded', 'true');
	await expect(missingRow.getByTestId('missing-episode').first()).toContainText(/S\d\dE\d\d/);
	await libraryView(own, 'Posters');
	await libraryStatus(own, 'not_out_yet');
	await expect(card).toBeVisible();
	await libraryStatus(own, 'better');
	await expect(card).toHaveCount(0);
	await libraryStatus(own, 'all');
	await libraryView(own, 'Banners');
	await expect(own.getByTestId('library-row').filter({ hasText: 'Harbour Lights' })).toContainText(/Missing · \d+|Complete/);
	await libraryView(own, 'Posters');
	await libraryKind(own, /^Movies/);
	await libraryView(own, 'Posters');
	await expect(card).toHaveCount(0);
	await expect(own.getByTestId('library-card').filter({ hasText: 'Quiet Orchard' })).toBeVisible();
	await libraryKind(own, /^Everything/);
	// Cards fade in one after another; let the ones on screen start before checking contrast.
	await own.waitForFunction(() => [...document.querySelectorAll('.lp')].every((e) => { const r = e.getBoundingClientRect(); return e.classList.contains('shown') || !e.getClientRects().length || r.top > innerHeight || r.bottom < 0 || r.left > innerWidth || r.right < 0; }), undefined, { timeout: 5_000 }).catch(() => undefined);
	await axe(own, 'library with shows');
});

test('a member asks for two seasons of a show from a phone', async ({ browser }) => {
	await own.goto('/settings/users');
	await own.getByRole('button', { name: 'Invite someone' }).click();
	await own.getByLabel('Note for yourself').fill('Max');
	await own.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await own.locator('.fresh code').textContent())!.trim();
	max = await fresh(browser, 412);
	await max.goto(new URL(link).pathname);
	await max.getByLabel('Name', { exact: true }).fill('max');
	await max.getByLabel('Password', { exact: true }).fill(maxPw);
	await max.getByLabel('Confirm password').fill(maxPw);
	await max.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(max).toHaveURL(/\/$/);

	await max.goto('/requests');
	await expect(max.getByTestId('ask-limit')).toContainText('show requests left this week');
	await openFromSearch(max, 'lantern', 'Lantern Street', 'show');
	await noSidewaysScroll(max);
	const askLantern = max.getByTestId('title-detail').getByRole('button', { name: /^Ask for Lantern Street/ });
	await askLantern.click();
	const dialog = max.getByRole('dialog');
	await dialog.getByRole('radio', { name: /Choose seasons/ }).check();
	await expect(dialog.getByRole('button', { name: 'Pick a season' })).toBeDisabled();
	await dialog.getByRole('checkbox', { name: 'Season 1' }).check();
	await dialog.getByRole('checkbox', { name: 'Season 2' }).check();
	await noSidewaysScroll(max);
	await axe(max, 'ask show dialog phone');
	await dialog.getByRole('button', { name: 'Ask for seasons 1 and 2' }).click();
	await expect(dialog).toBeHidden();
	await expect(max.getByRole('status')).toContainText('requested');

	// A show already in the Library opens as a Library title for a member (no Add, no tools):
	// More holds only Report a problem.
	await openFromSearch(max, 'harbour', 'Harbour Lights', 'show');
	await expect(max.getByTestId('hero-library')).toBeVisible();
	const more = max.getByRole('button', { name: 'More actions', exact: true });
	if (await more.count()) {
		await more.click();
		await expect(max.getByRole('menuitem')).toHaveText(['Report a problem']);
		await max.keyboard.press('Escape');
	}

	await max.locator('nav.tabbar').getByRole('link', { name: 'Requests' }).click();
	const ask = max.getByTestId('ask').filter({ hasText: 'Lantern Street' });
	await expect(ask.getByTestId('ask-state')).toHaveText('Requested');
	await expect(ask.getByTestId('ask-kind')).toHaveText('TV show');
	await expect(ask.getByTestId('ask-seasons')).toHaveText('Asked for seasons 1 and 2');
	await expect(ask.getByRole('button', { name: 'Approve' })).toHaveCount(0);
	await noSidewaysScroll(max);
	await axe(max, 'requests member phone');
});

test('the owner approves from a phone: the show is added with those seasons and max is told', async ({ browser }) => {
	const phone = await fresh(browser, 412);
	await signIn(phone, owner());
	await openRequests(phone);
	// Approvers get the request cards: who asked is on the card, Approve is one tap.
	await expect(phone.getByRole('button', { name: 'Open Lantern Street, asked by max' })).toBeVisible();
	await noSidewaysScroll(phone);
	// Approve sends after a short beat; wait for it before leaving the page.
	const approved = phone.waitForResponse((r) => /\/api\/v1\/asks\/\d+\/approve$/.test(new URL(r.url()).pathname) && r.request().method() === 'POST');
	await phone.getByRole('button', { name: 'Approve Lantern Street' }).click();
	expect((await approved).ok()).toBe(true);
	await expect(phone.getByRole('button', { name: 'Approve Lantern Street' })).toHaveCount(0);

	// The show page names who asked and keeps exactly the asked seasons.
	// Opened by its id: the phone Library only draws the cards near the screen.
	const lanternId: number = await phone.evaluate(async () => ((await (await fetch('/api/v1/titles')).json()).titles as { id: number; name: string }[]).find((t) => t.name === 'Lantern Street')!.id);
	await phone.goto(`/library/${lanternId}`);
	await expect(phone.getByTestId('asked-by-line')).toContainText(/max/i);
	expect(await seasonKept(phone, 'Season 1')).toBe(true);
	expect(await seasonKept(phone, 'Season 2')).toBe(true);
	expect(await seasonKept(phone, 'Season 3')).toBe(false);
	await noSidewaysScroll(phone);
	await axe(phone, 'show page phone');

	// History names who asked and who approved.
	await own.goto('/downloads/history');
	const lantern = own.getByTestId('history-row').filter({ hasText: 'Lantern Street' });
	await expect(lantern.filter({ hasText: 'Asked' }).filter({ hasText: 'max' }).first()).toBeVisible();
	await expect(lantern.filter({ hasText: 'Approved' }).filter({ hasText: 'luis' }).first()).toBeVisible();

	// max reads the message; members see no switches on the show page.
	expect(((await (await max.request.get('/api/v1/messages')).json()) as { messages: { text: string }[] }).messages.map((m) => m.text)).toContainEqual(expect.stringContaining('Lantern Street (2023), seasons 1 and 2 was approved by an admin'));
	await max.goto('/requests');
	await max.getByTestId('ask').filter({ hasText: 'Lantern Street' }).getByRole('link', { name: /Lantern Street/ }).click();
	expect(await seasonCount(max)).toBe(3);
	await expect(max.getByRole('checkbox')).toHaveCount(0);
	await expect(max.getByRole('button', { name: /^Keep .* up to date$/ })).toHaveCount(0);
	await expect(max.getByRole('button', { name: /^Episode actions for/ })).toHaveCount(0);
	await expect(max.getByRole('button', { name: 'Edit' })).toHaveCount(0);
	await noSidewaysScroll(max);
});
