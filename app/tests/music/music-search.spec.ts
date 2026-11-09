import { expect, test, type Browser, type Locator, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn, openAdvanced, openInLibrary } from '../helpers/helpers.ts';
import { axe } from '../helpers/axe.ts';

// Searching albums (3.3.3, 3.3.4) against the fake Torznab Source in
// fakes/: an album search answers with a FLAC copy, an MP3 320 copy and
// a 128 kbps copy. Choose a release on the album page lists all three with
// their verdicts; the 128 kbps copy is turned down; the FLAC copy is chosen
// for every track with one download; Home and the Turned down list name
// artist and album joined by an en dash; the artist's Library tab has Get all missing; Settings > Search has
// the music choices; a phone.
test.describe.configure({ mode: 'serial' });

let page: Page;

async function fresh(browser: Browser) {
	return (await browser.newContext()).newPage();
}

const release = (scope: Locator, part: string) => scope.getByTestId('release').filter({ has: scope.page().locator('.rel-name', { hasText: part }) });

async function openAlbum(p: Page, artist: string, album: string) {
	await p.goto('/library?kind=artist');
	await p.locator('[data-testid="library-card"], [data-testid="library-row"]', { hasText: artist }).click();
	await expect(p.getByRole('heading', { level: 1, name: artist })).toBeVisible();
	await p.getByRole('link', { name: album, exact: true }).click();
	await expect(p.getByRole('heading', { level: 1, name: new RegExp(album) })).toBeVisible();
	await openInLibrary(p);
	await expect(p.getByRole('heading', { level: 1, name: new RegExp(album) })).toBeVisible();
}

test.beforeAll(async ({ browser }) => {
	page = await fresh(browser);
	await signIn(page, owner());
});

test('Choose a release on the album page: verdicts, a Release turned down, the FLAC copy chosen for every track', async () => {
	await openAlbum(page, 'Tin Lanterns', 'Harbour Songs');
	await page.getByTestId('album-choose').click();
	const panel = page.getByTestId('show-releases');
	await expect(panel.getByRole('heading', { level: 2 })).toHaveText('Choose a release: Tin Lanterns – Harbour Songs');
	await expect(panel.getByTestId('release')).toHaveCount(3, { timeout: 20_000 });
	await expect(panel.getByTestId('search-running')).toHaveText('');
	const flac = release(panel, '[FLAC]');
	await expect(flac.getByTestId('verdict')).toHaveText('Would be chosen');
	await expect(flac).toContainText('4 tracks');
	await expect(release(panel, '[MP3 320]').getByTestId('verdict')).toHaveText('Passed over');
	const low = release(panel, '[128kbps]');
	await expect(low).toContainText('lossy_low is not allowed');
	await axe(page, 'album releases');

	await low.getByRole('button', { name: /^Turn down/ }).click();
	const td = page.getByRole('dialog', { name: 'Turn down this Release?' });
	await expect(td).toContainText('never picks it again for these tracks');
	await td.getByLabel('Why (optional)').fill('too small');
	await td.getByRole('button', { name: 'Turn down' }).click();
	await expect(low.getByTestId('verdict')).toHaveText('Turned down');

	await flac.getByRole('button', { name: /^Choose/ }).click();
	const ch = page.getByRole('dialog', { name: /^Choose (this Release|it anyway)\?$/ });
	await expect(ch).toContainText('For Tin Lanterns – Harbour Songs.');
	await expect(ch.getByTestId('choose-warnings')).toContainText('One download fills 4 tracks of Harbour Songs.');
	await ch.getByRole('button', { name: 'Choose', exact: true }).click();
	await expect(page.getByRole('status')).toContainText('For 4 tracks');
	await expect(page.getByTestId('track-state')).toHaveText(Array(4).fill(/Chosen|Downloading/), { timeout: 20_000 });
});

test('the Turned down list names the artist and the album', async () => {
	await page.goto('/library/turned-down');
	const row = page.getByTestId('listed-release').filter({ hasText: '[128kbps]' });
	await expect(row.getByTestId('listed-for')).toHaveText('Tin Lanterns – Harbour Songs');
	await expect(row).toContainText('too small');
	await axe(page, 'turned down with albums');
	await row.getByRole('button', { name: /^Take off/ }).click();
	await expect(row).toHaveCount(0);
});

test('the artist page searches every missing album, or one', async () => {
	await page.goto('/library?kind=artist');
	await page.locator('[data-testid="library-card"], [data-testid="library-row"]', { hasText: 'Tin Lanterns' }).click();
	// Search now and the per-album search sit on the artist's Library tab.
	await page.getByTestId('artist-tabs').getByRole('tab', { name: 'Library' }).click();
	const lib = page.getByTestId('artist-library');
	await lib.getByRole('button', { name: 'Get all missing' }).click();
	await expect(page.getByRole('status')).toContainText('Searching for all missing tracks');
	await lib.getByRole('button', { name: 'Get the missing tracks of Night Ferry', exact: true }).click();
	await expect(page.getByRole('status')).toContainText('Searching for Night Ferry');
	await axe(page, 'artist page with Search now');
});

test('Settings > Search has the music choices', async () => {
	await page.goto('/settings/search');
	await expect(page.getByTestId('missing-music')).toBeChecked();
	await openAdvanced(page.locator('#music-search'));
	await page.getByLabel('Tracks an album Release may lack').fill('1');
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByRole('status')).toContainText('Search settings saved');
	await page.reload();
	await expect(page.getByLabel('Tracks an album Release may lack')).toHaveValue('1');
	await page.getByLabel('Tracks an album Release may lack').fill('0');
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByRole('status')).toContainText('Search settings saved');
	await axe(page, 'search settings with music');
});

test('the album page searches from a phone', async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 375, height: 812 }, isMobile: true, hasTouch: true });
	const phone = await ctx.newPage();
	await signIn(phone, owner());
	await openAlbum(phone, 'Tin Lanterns', 'Night Ferry');
	await expect(phone.getByTestId('album-search-now')).toBeVisible();
	await phone.getByTestId('album-choose').click();
	const panel = phone.getByTestId('show-releases');
	await expect(panel.getByTestId('release').first()).toBeVisible({ timeout: 20_000 });
	await noSidewaysScroll(phone);
	await axe(phone, 'album releases on a phone');
	await ctx.close();
});
