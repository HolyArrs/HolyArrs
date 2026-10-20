import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Browser, type Page } from '@playwright/test';
import { join } from 'node:path';
import { moviesDir, noSidewaysScroll, owner, signIn, openApprovals, libraryKind, libraryStatus, titleAction, openAdvanced, openFromSearch } from './helpers.ts';

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

async function axe(p: Page, where: string) {
	await p.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
	const r = await new AxeBuilder({ page: p }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	const bad = r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
	if (bad.length) console.log('AXE', where, bad.map((v) => v.id + ' ' + v.nodes.slice(0, 3).map((n) => n.target.join(' ')).join(' | ')).join(' ;; '));
	expect(bad.map((v) => v.id), where).toEqual([]);
}

// Artists are added and asked for right on Search's cards.
async function searchMusic(p: Page, words: string) {
	await p.goto(`/search?q=${encodeURIComponent(words)}&kind=music`);
	await expect(p.getByRole('radio', { name: 'Music', exact: true })).toHaveAttribute('aria-checked', 'true');
}
const artistCard = (p: Page, name: string) => p.getByRole('main').getByRole('listitem').filter({ hasText: name }).first();

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
	await own.getByRole('button', { name: 'Add and check' }).click();
	await expect(own.getByTestId('folder-kind').filter({ hasText: 'For music' })).toHaveCount(1);

	await own.goto('/settings/music');
	await expect(own.getByLabel('Albums to keep up to date')).toHaveValue('all');
	await expect(own.getByRole('checkbox', { name: 'Album', exact: true })).toBeChecked();
	await expect(own.getByRole('checkbox', { name: 'EP', exact: true })).toBeChecked();
	await openAdvanced(own);
	await expect(own.getByRole('checkbox', { name: 'Live', exact: true })).not.toBeChecked();
	await expect(own.getByLabel('Which Pressing an album follows')).toHaveValue('common');
	await own.getByLabel('Keep answers for (hours)').fill('48');
	await own.getByRole('button', { name: 'Save' }).click();
	await expect(own.getByRole('status')).toContainText('Music settings saved.');
	await own.reload();
	await expect(own.getByLabel('Keep answers for (hours)')).toHaveValue('48');
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
	await expect(dialog.getByTestId('music-search-note')).toContainText('one Release per album');
	await expect(dialog.getByLabel('Albums to keep up to date')).toHaveValue('all');
	await dialog.getByLabel('Albums to keep up to date').selectOption('latest');
	await expect(dialog).toContainText('Only the newest album that is out.');
	await dialog.getByLabel('Albums to keep up to date').selectOption('all');
	await axe(own, 'add artist dialog');
	await dialog.getByRole('button', { name: 'Add to Library' }).click();
	await expect(dialog).toBeHidden(slow);
	await expect(own.getByRole('status')).toContainText('Tin Lanterns is in your Library.');
	await own.getByRole('main').getByRole('link', { name: 'Tin Lanterns,' }).first().click();

	// The artist page: albums with kinds, dates and state, and Search now.
	await expect(own.getByRole('heading', { level: 1, name: 'Tin Lanterns' })).toBeVisible();
	await own.getByRole('button', { name: 'More actions', exact: true }).click();
	await expect(own.getByRole('menuitem', { name: 'Search now', exact: true })).toBeVisible();
	await own.keyboard.press('Escape');
	await expect(own.getByTestId('artist-progress')).toContainText('tracks in the Library', slow);
	const albums = own.getByTestId('album');
	await expect(albums).toHaveCount(3);
	await expect(albums.filter({ hasText: 'Paper Boats' })).toContainText('2099 · EP');
	await expect(albums.filter({ hasText: 'Paper Boats' }).getByTestId('album-state')).toHaveText('Not out yet', slow);
	await expect(albums.filter({ hasText: 'Night Ferry' }).getByTestId('album-state')).toHaveText('Missing', slow);
	await expect(albums.filter({ hasText: 'Night Ferry' }).getByTestId('album-line')).toContainText('0 of 2 tracks in the Library · 2 missing');
	await expect(own.getByRole('checkbox', { name: 'Keep Night Ferry up to date' })).toBeChecked();
	// The live album is left out by the album kinds.
	await own.getByRole('button', { name: /^Not included/ }).click();
	await expect(albums).toHaveCount(1);
	await expect(albums.first()).toContainText('Live at the Pier');
	await expect(albums.first()).toContainText('Album · Live');
	await expect(albums.first().getByTestId('album-state')).toHaveText('Not included');
	await expect(own.getByRole('checkbox', { name: 'Keep Live at the Pier up to date' })).toBeDisabled();
	await own.getByRole('button', { name: 'Included', exact: true }).click();
	await axe(own, 'artist page');

	// An album turns off and on.
	await own.getByRole('checkbox', { name: 'Keep Night Ferry up to date' }).uncheck({ force: true });
	await expect(own.getByRole('status')).toContainText('Night Ferry: HolyArrs stops looking after it.');
	await expect(albums.filter({ hasText: 'Night Ferry' }).getByTestId('album-state')).toHaveText('Not kept up to date');
	await own.getByRole('checkbox', { name: 'Keep Night Ferry up to date' }).check({ force: true });
	await expect(albums.filter({ hasText: 'Night Ferry' }).getByTestId('album-state')).toHaveText('Missing');
});

test('the album page lists tracks and Pressings, and the owner picks another Pressing', async () => {
	await own.getByTestId('album').filter({ hasText: 'Harbour Songs' }).getByRole('link', { name: 'Harbour Songs' }).click();
	await expect(own.getByRole('heading', { level: 1, name: /Harbour Songs/ })).toBeVisible();
	await expect(own.getByTestId('pressing')).toHaveCount(3);
	await expect(own.getByTestId('track')).toHaveCount(3);
	await expect(own.getByTestId('track').first()).toContainText('Song 1.1');
	await expect(own.getByTestId('track').first()).toContainText('3:00');
	await expect(own.getByTestId('track-state').first()).toHaveText('Missing');
	await expect(own.getByTestId('chosen-pressing')).toContainText('Digital Media · 3 tracks');
	await expect(own.getByTestId('chosen-pressing')).toContainText('picked by the rule');
	await axe(own, 'album page');
	await own.getByLabel('Pressing to follow').selectOption({ label: 'Harbour Songs (deluxe) · 2019 · GB · CD · 4 tracks' });
	await own.getByRole('button', { name: 'Use this Pressing' }).click();
	await expect(own.getByRole('status')).toContainText('The tracks now follow Harbour Songs.');
	await expect(own.getByTestId('track')).toHaveCount(4);
	await expect(own.getByTestId('chosen-pressing')).toContainText('picked by hand');
	await own.getByRole('link', { name: 'Tin Lanterns', exact: true }).first().click();
	await expect(own.getByTestId('album').filter({ hasText: 'Harbour Songs' }).getByTestId('album-line')).toContainText('of 4 tracks');
});

test('artists sit in the Library with a Music kind', async () => {
	await own.goto('/library');
	await libraryKind(own, /^Music/);
	await expect(own).toHaveURL(/kind=artist/);
	const card = own.getByTestId('library-card').filter({ hasText: 'Tin Lanterns' });
	await expect(card).toContainText('Artist');
	// Posters carry only the status circle now.
	await expect(own.getByTestId('library-card').filter({ hasText: 'Harbour Lights' })).toHaveCount(0);
	await own.getByRole('radiogroup', { name: 'View' }).getByRole('radio', { name: 'Table' }).click();
	await expect(own.getByTestId('library-row').filter({ hasText: 'Tin Lanterns' })).toContainText('tracks in the Library');
	await own.getByRole('radiogroup', { name: 'View' }).getByRole('radio', { name: 'Grid' }).click();
	await libraryKind(own, /^Everything/);
	await axe(own, 'library with music');
});

test('a member asks for one single from a phone', async ({ browser }) => {
	await own.goto('/settings/users');
	await own.getByLabel('Note for yourself').fill('Ivy');
	await own.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await own.locator('.fresh code').textContent())!.trim();
	ivy = await fresh(browser, 412);
	await ivy.goto(new URL(link).pathname);
	await ivy.getByLabel('Name', { exact: true }).fill('ivy');
	await ivy.getByLabel('Password', { exact: true }).fill(ivyPw);
	await ivy.getByLabel('Password again').fill(ivyPw);
	await ivy.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(ivy).toHaveURL(/\/$/);

	await ivy.goto('/wishlist');
	await expect(ivy.getByTestId('ask-limit')).toContainText('music asks left this week');
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
	await expect(ivy.getByRole('status')).toContainText('Asked for Moss Choir');

	// An artist in the Library offers to ask for the albums it does not keep (on its page).
	await openFromSearch(ivy, 'lantern', 'Tin Lanterns');
	await expect(ivy.getByTestId('ask-albums')).toBeVisible();

	await ivy.locator('nav.tabbar').getByRole('link', { name: 'Wishlist' }).click();
	const ask = ivy.getByTestId('ask').filter({ hasText: 'Moss Choir' });
	await expect(ask.getByTestId('ask-state')).toHaveText('Requested');
	await expect(ask.getByTestId('ask-kind')).toHaveText('Music');
	await expect(ask.getByTestId('ask-albums')).toHaveText('Asked for Fern Hymns');
	await expect(ask.getByRole('button', { name: 'Approve' })).toHaveCount(0);
	await noSidewaysScroll(ivy);
	await axe(ivy, 'wishlist member phone music');
});

test('the owner approves from a phone: the artist is added with that single and ivy is told', async ({ browser }) => {
	const phone = await fresh(browser, 412);
	await signIn(phone, owner());
	await openApprovals(phone);
	const ask = phone.getByTestId('ask').filter({ hasText: 'Moss Choir' });
	await expect(ask.getByTestId('asked-by')).toContainText('Asked by ivy');
	// The row's Approve is one tap with Undo; Review holds Approve with options.
	await ask.getByRole('button', { name: 'Review Moss Choir' }).click();
	await phone.getByRole('button', { name: 'Approve with options' }).click();
	const dialog = phone.getByRole('dialog', { name: /^Approve Moss Choir/ });
	await expect(dialog).toContainText('for Fern Hymns');
	await expect(dialog.getByLabel('Library folder')).toContainText('Music');
	await expect(dialog.getByLabel('Library folder')).not.toContainText('Shows');
	await expect(dialog).toContainText('with Fern Hymns kept up to date');
	await dialog.getByLabel('Note (optional)').fill('Enjoy the hymns');
	await noSidewaysScroll(phone);
	await dialog.getByRole('button', { name: 'Approve and add' }).click();
	await expect(dialog).toBeHidden(slow);
	await phone.getByRole('button', { name: /^Approved/ }).click();
	await expect(ask.getByTestId('ask-state')).toHaveText('Approved');
	await expect(ask.getByTestId('decided-by')).toContainText('Approved by luis');

	// The artist page names who asked and keeps only the asked single.
	await ask.getByRole('link', { name: /Moss Choir/ }).click();
	await expect(phone.getByTestId('title-asks')).toContainText('ivy');
	await expect(phone.getByTestId('title-asks')).toContainText('Fern Hymns');
	await expect(phone.getByRole('checkbox', { name: 'Keep Fern Hymns up to date' })).toBeChecked(slow);
	await expect(phone.getByRole('checkbox', { name: 'Keep Slow Green up to date' })).not.toBeChecked();
	await noSidewaysScroll(phone);
	await axe(phone, 'artist page phone');

	// History names who asked and who approved.
	await own.goto('/downloads/history');
	await expect(own.getByText('ivy asked for Moss Choir, Fern Hymns').first()).toBeVisible();
	await expect(own.getByText(/Moss Choir, Fern Hymns: approved by luis/).first()).toBeVisible();

	// ivy reads the message; members see no switches on the artist page.
	await ivy.goto('/wishlist');
	await expect(ivy.getByTestId('message').first()).toContainText('Moss Choir, Fern Hymns was approved by luis');
	await expect(ivy.getByTestId('message').first()).toContainText('Enjoy the hymns');
	await ivy.getByTestId('ask').filter({ hasText: 'Moss Choir' }).getByRole('link', { name: /Moss Choir/ }).click();
	// Members see the albums as cards; the Albums list with its switches is for managers.
	await expect(ivy.getByRole('heading', { level: 1, name: /Moss Choir/ })).toBeVisible();
	await expect(ivy.getByTestId('album')).toHaveCount(0);
	await expect(ivy.getByRole('checkbox')).toHaveCount(0);
	await expect(ivy.getByRole('button', { name: 'Refresh' })).toHaveCount(0);
	await expect(ivy.getByRole('button', { name: 'Edit album kinds' })).toHaveCount(0);
	await expect(ivy.getByRole('button', { name: 'More actions' })).toHaveCount(0);
	await expect(ivy.getByTestId('ask-albums')).toBeVisible();
	await noSidewaysScroll(ivy);
	await ivy.getByRole('link', { name: /^Fern Hymns/ }).first().click();
	await expect(ivy.getByRole('heading', { level: 1, name: /Fern Hymns/ })).toBeVisible();
	await expect(ivy.getByRole('button', { name: 'Use this Pressing' })).toHaveCount(0);
	await expect(ivy.getByRole('checkbox')).toHaveCount(0);
	await noSidewaysScroll(ivy);
});
