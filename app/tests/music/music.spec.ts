import { expect, test, type Browser, type Page } from '@playwright/test';
import { join } from 'node:path';
import { moviesDir, noSidewaysScroll, owner, signIn, openRequests, libraryKind, libraryView, openAdvanced, openFromSearch } from '../helpers/helpers.ts';
import { axe } from '../helpers/axe.ts';

// Music (2.3.4 to 2.3.8): the owner adds a music folder, looks at Settings >
// Music, adds Tin Lanterns from Search (the fake MusicBrainz knows it and
// Moss Choir), sees its albums, kinds and states on the artist page, turns
// an album off and picks another Pressing on the album page; a member asks
// for one single of Moss Choir from a phone, the owner approves it from a
// phone, the artist lands with only that single kept up to date, and the
// member is told; History names who asked and who approved. Runs last.
test.describe.configure({ mode: 'serial' });

let own: Page;
let ivy: Page;
const musicDir = join(moviesDir, '..', 'music');
const ivyPw = `e2e-${Math.random().toString(36).slice(2)}-ivy`;
const slow = { timeout: 45_000 };

async function fresh(browser: Browser, width = 1280): Promise<Page> {
	const ctx = await browser.newContext({ viewport: { width, height: width < 600 ? 915 : 900 } });
	return ctx.newPage();
}

// Artists are added and asked for right on Search's cards.
async function searchMusic(p: Page, words: string) {
	await p.goto(`/search?q=${encodeURIComponent(words)}&kind=music`);
	await expect(p.getByRole('radio', { name: 'Music', exact: true })).toHaveAttribute('aria-checked', 'true');
}
const artistCard = (p: Page, name: string) => p.getByRole('main').getByRole('listitem').filter({ hasText: name }).first();

// An owner's artist page from Search keeps its Library as a tab ("In Library"
// switches to it), so the artist's Library page is opened by its id.
async function artistLibraryPage(p: Page, name: string) {
	if (/\/library\/\d+/.test(p.url())) return;
	const { titles } = (await (await p.request.get('/api/v1/titles')).json()) as { titles: { id: number; name: string; kind: string }[] };
	const t = titles.find((x) => x.kind === 'artist' && x.name === name);
	expect(t, `${name} is in the Library`).toBeTruthy();
	await p.goto(`/library/${t!.id}`);
}

/** libraryTab opens an artist's Library tab: its albums (the only album list), their actions and settings. */
async function libraryTab(p: Page) {
	const tab = p.getByTestId('artist-tabs').getByRole('tab', { name: 'Library' });
	await tab.click();
	await expect(tab).toHaveAttribute('aria-selected', 'true');
	return p.getByTestId('artist-library');
}

test.beforeAll(async ({ browser }) => {
	own = await fresh(browser);
	await signIn(own, owner());
});

test('the owner adds a music folder and looks at Settings > Music', async () => {
	await own.goto('/settings/library-folders');
	await own.getByRole('button', { name: 'Add a folder' }).first().click();
	await own.getByLabel('Holds').selectOption('music');
	await own.getByLabel('Folder path').fill(musicDir);
	await own.getByLabel(/^Name/).fill('Music');
	await own.getByRole('button', { name: 'Add', exact: true }).click();
	await expect(own.getByTestId('folder-kind').filter({ hasText: 'For music' })).toHaveCount(1);

	await own.goto('/settings/music');
	await expect(own.getByLabel('Albums to keep up to date')).toHaveValue('all');
	await expect(own.getByRole('checkbox', { name: 'Album', exact: true })).toBeChecked();
	await expect(own.getByRole('checkbox', { name: 'EP', exact: true })).toBeChecked();
	await openAdvanced(own);
	await expect(own.getByRole('checkbox', { name: 'Live', exact: true })).not.toBeChecked();
	await expect(own.getByLabel('Which Pressing an album follows')).toHaveValue('common');
	await own.getByLabel('Cache answers for (hours)').fill('48');
	await own.getByRole('button', { name: 'Save' }).click();
	await expect(own.getByRole('status')).toContainText('Music settings saved');
	await own.reload();
	await expect(own.getByLabel('Cache answers for (hours)')).toHaveValue('48');
	await axe(own, 'settings music');
});

test('the owner finds an artist and adds it from Search', async () => {
	await own.goto('/search?q=lantern');
	await own.getByRole('radio', { name: 'Music', exact: true }).click();
	await expect(own).toHaveURL(/kind=music/);
	const hit = artistCard(own, 'Tin Lanterns');
	await expect(hit).toContainText('lab folk band');
	await axe(own, 'search music');
	await hit.getByRole('button', { name: 'Add' }).click();

	const dialog = own.getByRole('dialog');
	await expect(dialog.getByLabel('Library folder')).toContainText('Music');
	await expect(dialog.getByLabel('Library folder')).not.toContainText('Shows');
	await expect(dialog.getByTestId('music-search-note')).toContainText('searched once their tracks are in');
	await expect(dialog.getByLabel('Albums to keep up to date')).toHaveValue('all');
	await dialog.getByLabel('Albums to keep up to date').selectOption('latest');
	await expect(dialog).toContainText('Only the newest album that is out.');
	await dialog.getByLabel('Albums to keep up to date').selectOption('all');
	await axe(own, 'add artist dialog');
	await dialog.getByRole('button', { name: 'Add to Library' }).click();
	await expect(dialog).toBeHidden(slow);
	await expect(own.getByRole('status')).toContainText('Tin Lanterns added');
	await own.getByRole('main').getByRole('link', { name: 'Tin Lanterns,' }).first().click();

	// The artist page: the hero, then the Library tab with the albums, their kinds, dates and state, and Search.
	await expect(own.getByRole('heading', { level: 1, name: 'Tin Lanterns' })).toBeVisible();
	await artistLibraryPage(own, 'Tin Lanterns');
	await expect(own.getByRole('heading', { level: 1, name: 'Tin Lanterns' })).toBeVisible();
	await expect(own.getByTestId('artist-progress')).toContainText('tracks in the Library', slow);
	// Overview has no album list below it and no More menu: the albums show once, in the Library tab.
	await expect(own.getByTestId('album')).toHaveCount(0);
	await expect(own.getByRole('button', { name: 'More actions', exact: true })).toHaveCount(0);
	// The hero's In Library button opens the Library tab (the last copy is the one under the artist's name).
	await own.getByTestId('hero-library').last().click();
	await expect(own.getByTestId('artist-tabs').getByRole('tab', { name: 'Library' })).toHaveAttribute('aria-selected', 'true');
	const lib = own.getByTestId('artist-library');
	await expect(lib.getByRole('button', { name: 'Get all missing' })).toBeVisible(slow);
	const albums = own.getByTestId('album');
	await expect(albums).toHaveCount(3, slow);
	await expect(lib.getByTestId('album')).toHaveCount(3);
	await expect(albums.filter({ hasText: 'Paper Boats' })).toContainText('2099');
	await expect(albums.filter({ hasText: 'Paper Boats' }).getByTestId('album-state')).toHaveText('Not out yet', slow);
	await expect(albums.filter({ hasText: 'Night Ferry' }).getByTestId('album-state')).toHaveText('Missing', slow);
	await expect(albums.filter({ hasText: 'Night Ferry' }).getByTestId('album-count')).toHaveText('0 / 2');
	await expect(lib.getByRole('button', { name: 'Keep Night Ferry up to date' })).toHaveAttribute('aria-pressed', 'true');
	// The live album is left out by the album kinds.
	await lib.getByRole('button', { name: /^Not included/ }).click();
	await expect(albums).toHaveCount(1);
	await expect(albums.first()).toContainText('Live at the Pier');
	await expect(lib.getByText(/^Live\s*1$/)).toBeVisible();
	await expect(albums.first().getByTestId('album-state')).toHaveText('Not included');
	await expect(lib.getByRole('button', { name: 'Keep Live at the Pier up to date' })).toBeDisabled();
	await lib.getByRole('button', { name: 'Included', exact: true }).click();
	await expect(albums).toHaveCount(3);
	await axe(own, 'artist page');

	// An album turns off and on.
	await lib.getByRole('button', { name: 'Keep Night Ferry up to date' }).click();
	await expect(own.getByRole('status')).toContainText('Stopped updating Night Ferry');
	await expect(albums.filter({ hasText: 'Night Ferry' }).getByTestId('album-state')).toHaveText('Not kept');
	await expect(lib.getByRole('button', { name: 'Keep Night Ferry up to date' })).toHaveAttribute('aria-pressed', 'false');
	await lib.getByRole('button', { name: 'Keep Night Ferry up to date' }).click();
	await expect(albums.filter({ hasText: 'Night Ferry' }).getByTestId('album-state')).toHaveText('Missing');

	// Releases: a ready-made choice on the row; Custom opens the full list (extra kinds under More).
	const kindsRow = lib.getByTestId('kinds-row');
	await expect(kindsRow.getByRole('radiogroup', { name: 'Releases' })).toBeVisible();
	await kindsRow.getByRole('button', { name: 'Release kinds' }).click();
	await own.getByRole('menuitem', { name: 'Choose kinds…' }).click();
	const kinds = own.getByRole('dialog', { name: 'Album kinds' });
	await expect(kinds.getByLabel('New albums')).toHaveValue('all');
	await expect(kinds.getByRole('checkbox', { name: 'Album', exact: true })).toBeChecked();
	await kinds.getByRole('button', { name: 'Live, compilations and more' }).click();
	await expect(kinds.getByRole('checkbox', { name: 'Live', exact: true })).not.toBeChecked();
	await axe(own, 'artist album kinds');
	await kinds.getByRole('button', { name: 'Cancel' }).click();
	await expect(kinds).toBeHidden();
	await expect(lib.getByRole('switch', { name: /^Keep up to date/ })).toBeVisible();
	for (const b of [/^Refresh details/, /^Delete artist/]) await expect(lib.getByRole('button', { name: b })).toBeVisible();
	await expect(lib.getByTestId('quality-row')).toBeVisible();
	await expect(lib.getByRole('heading', { name: 'History' })).toBeVisible();
});

test('the album page lists tracks and Pressings, and the owner picks another Pressing', async () => {
	// An album row opens to its tracks and a link to its album page.
	const row = own.getByTestId('album').filter({ hasText: 'Harbour Songs' });
	await row.getByRole('button', { expanded: false }).click();
	await row.getByRole('link', { name: 'Open the album page of Harbour Songs' }).click();
	// The Library artist page's album links open the album's Library page straight away.
	await expect(own).toHaveURL(/\/library\/\d+\/albums\//);
	await expect(own.getByRole('heading', { level: 1, name: /Harbour Songs/ })).toBeVisible();
	await expect(own.getByTestId('track')).toHaveCount(3);
	await expect(own.getByTestId('track').first()).toContainText('Song 1.1');
	await expect(own.getByTestId('track').first()).toContainText('3:00');
	await expect(own.getByTestId('track-state').first()).toHaveText('Missing');
	await expect(own.getByTestId('chosen-pressing')).toContainText('Digital Media · 3 tracks');
	await expect(own.getByTestId('chosen-pressing')).toContainText('By the rule');
	await axe(own, 'album page');
	await own.getByTestId('chosen-pressing').getByRole('button', { name: 'Edition' }).click();
	await own.getByRole('menuitemradio', { name: 'Harbour Songs (deluxe) · 2019 · GB · CD · 4 tracks' }).or(own.getByRole('menuitem', { name: 'Harbour Songs (deluxe) · 2019 · GB · CD · 4 tracks' })).click();
	await own.getByRole('dialog', { name: 'Follow this edition?' }).getByRole('button', { name: 'Use this edition' }).click();
	await expect(own.getByRole('status')).toContainText('Pressing changed');
	await expect(own.getByTestId('track')).toHaveCount(4);
	await expect(own.getByTestId('chosen-pressing')).toContainText('By hand');
	await own.getByRole('link', { name: 'Tin Lanterns', exact: true }).first().click();
	const lib = await libraryTab(own);
	await expect(lib.getByTestId('album').filter({ hasText: 'Harbour Songs' }).getByTestId('album-count')).toContainText('/ 4');
});

test('artists sit in the Library with a Music kind', async () => {
	await own.goto('/library');
	await libraryKind(own, /^Music/);
	await expect(own).toHaveURL(/kind=artist/);
	await libraryView(own, 'Posters');
	const card = own.getByTestId('library-card').filter({ hasText: 'Tin Lanterns' });
	await expect(card).toBeVisible();
	// Posters carry only the status circle now.
	await expect(own.getByTestId('library-card').filter({ hasText: 'Harbour Lights' })).toHaveCount(0);
	await libraryView(own, 'Banners');
	await expect(own.getByTestId('library-row').filter({ hasText: 'Tin Lanterns' })).toContainText(/Missing · \d+|Complete/);
	await libraryView(own, 'Posters');
	await libraryKind(own, /^Everything/);
	// Cards fade in one after another; let the ones on screen start before checking contrast.
	await own.waitForFunction(() => [...document.querySelectorAll('.lp')].every((e) => { const r = e.getBoundingClientRect(); return e.classList.contains('shown') || !e.getClientRects().length || r.top > innerHeight || r.bottom < 0 || r.left > innerWidth || r.right < 0; }), undefined, { timeout: 5_000 }).catch(() => undefined);
	await axe(own, 'library with music');
});

test('a member asks for one single from a phone', async ({ browser }) => {
	await own.goto('/settings/users');
	await own.getByRole('button', { name: 'Invite someone' }).click();
	await own.getByLabel('Note for yourself').fill('Ivy');
	await own.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await own.locator('.fresh code').textContent())!.trim();
	ivy = await fresh(browser, 412);
	await ivy.goto(new URL(link).pathname);
	await ivy.getByLabel('Name', { exact: true }).fill('ivy');
	await ivy.getByLabel('Password', { exact: true }).fill(ivyPw);
	await ivy.getByLabel('Confirm password').fill(ivyPw);
	await ivy.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(ivy).toHaveURL(/\/$/);

	await ivy.goto('/requests');
	await expect(ivy.getByTestId('ask-limit')).toContainText('music requests left this week');
	await searchMusic(ivy, 'moss');
	const hit = artistCard(ivy, 'Moss Choir');
	await expect(hit).toBeVisible();
	await expect(hit.getByRole('button', { name: 'Add' })).toHaveCount(0);
	await noSidewaysScroll(ivy);
	await hit.getByRole('button', { name: 'Ask', exact: true }).click();
	const dialog = ivy.getByRole('dialog');
	await dialog.getByRole('radio', { name: /Only some albums/ }).check(slow);
	await expect(dialog.getByRole('button', { name: 'Pick an album' })).toBeDisabled();
	await expect(dialog.getByTestId('ask-album')).toHaveCount(2);
	await dialog.getByRole('checkbox', { name: /Fern Hymns/ }).check();
	await noSidewaysScroll(ivy);
	await axe(ivy, 'ask artist dialog phone');
	await dialog.getByRole('button', { name: 'Ask for Fern Hymns' }).click();
	await expect(dialog).toBeHidden();
	await expect(ivy.getByRole('status')).toContainText('Moss Choir (Fern Hymns) requested');

	// An artist in the Library offers to ask for the albums it does not keep (on its page).
	await openFromSearch(ivy, 'lantern', 'Tin Lanterns');
	// The hero and the slim bar that follows it on scroll both offer it.
	await expect(ivy.getByTestId('ask-albums').first()).toBeVisible();
	// At 390 the artist page, with its Coming soon row (Paper Boats, 2099), never scrolls sideways.
	await ivy.setViewportSize({ width: 390, height: 844 });
	await expect(ivy.getByRole('heading', { name: 'Coming soon' })).toBeVisible();
	await noSidewaysScroll(ivy);
	await ivy.setViewportSize({ width: 412, height: 915 });

	await ivy.locator('nav.tabbar').getByRole('link', { name: 'Requests' }).click();
	const ask = ivy.getByTestId('ask').filter({ hasText: 'Moss Choir' });
	await expect(ask.getByTestId('ask-state')).toHaveText('Requested');
	await expect(ask.getByTestId('ask-kind')).toHaveText('Music');
	await expect(ask.getByTestId('ask-albums')).toHaveText('Asked for Fern Hymns');
	await expect(ask.getByRole('button', { name: 'Approve' })).toHaveCount(0);
	await noSidewaysScroll(ivy);
	await axe(ivy, 'requests member phone music');
});

test('the owner approves from a phone: the artist is added with that single and ivy is told', async ({ browser }) => {
	const phone = await fresh(browser, 412);
	await signIn(phone, owner());
	await openRequests(phone);
	// Approvers open on Pending; the card names who asked.
	await expect(phone.getByRole('button', { name: /^Waiting/ })).toHaveAttribute('aria-pressed', 'true');
	const ask = phone.getByRole('list', { name: 'Requests' }).getByRole('listitem').filter({ hasText: 'Moss Choir' });
	await expect(ask).toContainText('ivy');
	await noSidewaysScroll(phone);
	// The card opens its window straight into What to add: only the asked single, ticked.
	await ask.getByRole('button', { name: 'Open Moss Choir, asked by ivy' }).click();
	const win = phone.getByRole('dialog', { name: 'Moss Choir' });
	await expect(win.getByTestId('ask-banner')).toContainText('ivy asked');
	const sheet = win.getByRole('region', { name: 'What to add' });
	await expect(sheet).toBeVisible(slow);
	await expect(sheet.getByTestId('ask-album')).toHaveCount(1, slow);
	await expect(sheet.getByTestId('ask-album')).toContainText('Fern Hymns');
	await expect(sheet.getByTestId('ask-album')).toHaveAttribute('data-tick', 'all');
	await expect(sheet).toContainText('Approving adds fern hymns');
	await noSidewaysScroll(phone);
	await sheet.getByRole('button', { name: 'Approve', exact: true }).click();
	await expect(win).toBeHidden(slow);
	// The window goes on to the next waiting ask if there is one; close it so the tabs are free.
	const still = phone.locator('dialog.win[open]');
	if (await still.count()) {
		await phone.keyboard.press('Escape');
		await expect(still).toHaveCount(0);
	}
	await expect(phone.getByRole('status').filter({ hasText: 'Moss Choir approved' }).getByRole('button', { name: 'Undo' })).toBeVisible();
	// Sent once the Undo time is over.
	await expect
		.poll(async () => ((await (await phone.request.get('/api/v1/asks?state=approved')).json()) as { asks: { name: string }[] }).asks.some((a) => a.name === 'Moss Choir'), slow)
		.toBe(true);

	// Processing holds it; its window says it is being got for ivy and lists what was approved.
	await phone.getByRole('button', { name: /^Processing/ }).click();
	await ask.getByRole('button', { name: /^Open Moss Choir/ }).click();
	await expect(win.getByTestId('ask-status')).toContainText('Getting it for ivy');
	const kept = win.getByRole('region', { name: /What was approved/ });
	await expect(kept.getByTestId('ask-album').filter({ hasText: 'Fern Hymns' })).toHaveAttribute('data-tick', 'all', slow);
	await expect(kept.getByTestId('ask-album').filter({ hasText: 'Slow Green' })).toHaveAttribute('data-tick', 'none');

	// The artist page names who asked, who approved, and keeps only the asked single.
	await kept.getByTestId('ask-title-page-sheet').click();
	await expect(phone).toHaveURL(/\/library\/\d+/);
	await expect(phone.getByTestId('title-asks')).toContainText('ivy');
	await expect(phone.getByTestId('title-asks')).toContainText('Fern Hymns');
	await expect(phone.getByTestId('title-asks')).toContainText('Approved by luis');
	const lib = await libraryTab(phone);
	await expect(lib.getByRole('button', { name: 'Keep Fern Hymns up to date' })).toHaveAttribute('aria-pressed', 'true', slow);
	await expect(lib.getByRole('button', { name: 'Keep Slow Green up to date' })).toHaveAttribute('aria-pressed', 'false');
	await noSidewaysScroll(phone);
	await axe(phone, 'artist page phone');

	// History names who asked and who approved (each row shows the person and the step).
	await own.goto('/downloads/history');
	await expect(own.getByTestId('history-row').filter({ hasText: 'Moss Choir' }).filter({ hasText: /ivy[\s\S]*Asked/ }).first()).toBeVisible();
	await expect(own.getByTestId('history-row').filter({ hasText: 'Moss Choir' }).filter({ hasText: /luis[\s\S]*Approved/ }).first()).toBeVisible();

	// ivy is told (no inbox card on Requests; the news is on her card); members see no switches on the artist page.
	expect(((await (await ivy.request.get('/api/v1/messages')).json()) as { messages: { text: string }[] }).messages.map((m) => m.text)).toContainEqual(expect.stringContaining('Moss Choir, Fern Hymns was approved by an admin'));
	await ivy.goto('/requests');
	const mine = ivy.getByTestId('ask').filter({ hasText: 'Moss Choir' });
	await expect(mine.getByTestId('ask-state')).toHaveText('Approved');
	// Members read who decided by display name only; luis set none.
	await expect(mine.getByTestId('decided-by')).toContainText('Approved by an admin');
	await ivy.getByTestId('ask').filter({ hasText: 'Moss Choir' }).getByRole('link', { name: /Moss Choir/ }).click();
	// Members see the albums as cards; the Albums list with its switches is for managers.
	await expect(ivy.getByRole('heading', { level: 1, name: /Moss Choir/ })).toBeVisible();
	await expect(ivy.getByTestId('album')).toHaveCount(0);
	await expect(ivy.getByRole('checkbox')).toHaveCount(0);
	await expect(ivy.getByRole('button', { name: 'Refresh' })).toHaveCount(0);
	await expect(ivy.getByTestId('artist-tabs')).toHaveCount(0);
	await expect(ivy.getByTestId('kinds-row')).toHaveCount(0);
	await expect(ivy.getByRole('button', { name: 'More actions' })).toHaveCount(0);
	await expect(ivy.getByTestId('ask-albums').first()).toBeVisible();
	await noSidewaysScroll(ivy);
	// The single's card names its state first ("Missing: … Fern Hymns 2021 · Single").
	await ivy.getByRole('group', { name: 'Singles & EPs' }).getByRole('link', { name: /Fern Hymns/ }).first().click();
	await expect(ivy.getByRole('heading', { level: 1, name: /Fern Hymns/ })).toBeVisible();
	await expect(ivy.getByTestId('chosen-pressing')).toHaveCount(0);
	await expect(ivy.getByRole('checkbox')).toHaveCount(0);
	await noSidewaysScroll(ivy);
});
