import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Browser, type Locator, type Page } from '@playwright/test';
import { noSidewaysScroll, owner, signIn } from './helpers.ts';

// Searching shows (3.2.2 to 3.2.5) against the fake Torznab Source in
// fakes.mjs: a show search answers per episode (a 1080p and a 720p web
// copy) or per season (a 1080p season pack and each aired episode).
// Choose a release for a season takes the pack for every episode with one
// download; an episode's Release turned down is never picked again for it;
// the Turned down list and Home name the show and the episode.
test.describe.configure({ mode: 'serial' });

let page: Page;

async function fresh(browser: Browser) {
	return (await browser.newContext()).newPage();
}

async function axe(p: Page, where: string) {
	await p.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
	const r = await new AxeBuilder({ page: p }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	const bad = r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
	if (bad.length) console.log('AXE', where, bad.map((v) => v.id + ' ' + v.nodes.slice(0, 3).map((n) => n.target.join(' ')).join(' | ')).join(' ;; '));
	expect(bad.map((v) => v.id), where).toEqual([]);
}

const release = (scope: Locator, part: string) => scope.getByTestId('release').filter({ has: scope.page().locator('.rel-name', { hasText: part }) });

async function openShow(p: Page, name: string) {
	await p.goto('/library');
	await p.locator('.grid .item', { hasText: name }).click();
	await expect(p.getByRole('heading', { level: 1 })).toContainText(name);
}

// One season shows at a time: pick it in the Season control.
async function openSeason(p: Page, name: string) {
	await expect(p.getByTestId('season')).toBeVisible();
	const radio = p.getByRole('radiogroup', { name: 'Season' }).getByRole('radio', { name, exact: true });
	await radio.click();
	await expect(radio).toHaveAttribute('aria-checked', 'true');
	const s = p.getByTestId('season');
	await expect(s.getByTestId('episode').first()).toBeVisible();
	return s;
}

// Each episode card keeps its searches in its menu.
async function episodeMenu(ep: Locator) {
	await ep.getByRole('button', { name: /^Actions for/ }).click();
	return ep.page().getByRole('menu', { name: /^Actions for/ });
}

async function searchItems(s: Locator): Promise<string[]> {
	const out: string[] = [];
	const eps = s.getByTestId('episode');
	for (let i = 0; i < (await eps.count()); i++) {
		const menu = await episodeMenu(eps.nth(i));
		const item = menu.getByRole('menuitem', { name: /^(Search now|Find a better copy)$/ });
		out.push((await item.count()) ? (await item.innerText()).trim() : '');
		await s.page().keyboard.press('Escape');
	}
	return out;
}

test.beforeAll(async ({ browser }) => {
	page = await fresh(browser);
	await signIn(page, owner());
});

test('the owner adds Moss Parade with every season kept', async () => {
	await page.goto('/search?kind=show');
	await page.getByLabel('Show name or id').fill('moss parade');
	await page.getByRole('button', { name: 'Search', exact: true }).click();
	const hit = page.getByTestId('show-hit').filter({ hasText: 'Moss Parade' });
	await expect(hit).toBeVisible();
	const add = hit.getByRole('button', { name: 'Add' });
	if (await add.count()) {
		await add.click();
		const dialog = page.getByRole('dialog');
		await expect(dialog.getByTestId('no-show-search')).toContainText('Search now on the show page');
		await dialog.getByLabel('Seasons to keep up to date').selectOption('all');
		await dialog.getByRole('button', { name: 'Add to Library' }).click();
		await expect(dialog).toBeHidden();
	}
	await openShow(page, 'Moss Parade');
	const s1 = await openSeason(page, 'Season 1');
	await expect(s1.getByTestId('episode-state')).toHaveText(['Missing', 'Missing']);
	expect(await searchItems(s1)).toEqual(['Search now', 'Search now']);
});

test('Choose a release for a season: the season pack fills both episodes with one download', async () => {
	await openShow(page, 'Moss Parade');
	const s1 = await openSeason(page, 'Season 1');
	await s1.getByTestId('choose-season').click();
	const panel = page.getByTestId('show-releases');
	await expect(panel.getByRole('heading', { level: 2 })).toHaveText('Choose a release: Season 1');
	await expect(panel.getByTestId('release')).toHaveCount(3, { timeout: 20_000 });
	await expect(panel.getByTestId('search-running')).toHaveText('');

	const pack = release(panel, 'Moss.Parade.S01.1080p');
	await expect(pack.getByTestId('verdict')).toHaveText('Would be chosen');
	await expect(pack).toContainText('S01E01, S01E02');
	await expect(release(panel, '.S01E01.').getByTestId('verdict')).toHaveText('Passed over');
	await expect(release(panel, '.S01E01.')).toContainText('lost to Moss.Parade.S01.1080p.WEB-DL.H264-HOLYARRS on season pack');
	await axe(page, 'choose a release for a season');

	await pack.getByRole('button', { name: /^Choose/ }).click();
	const dlg = page.getByRole('dialog');
	await expect(dlg).toContainText('For S01E01, S01E02.');
	await expect(dlg.getByTestId('choose-warnings')).toContainText('One download fills S01E01, S01E02.');
	await dlg.getByRole('button', { name: 'Choose', exact: true }).click();
	await expect(dlg).toBeHidden();
	await expect(page.getByRole('status')).toContainText('for 2 episodes');
	// Both episodes go to the one download.
	await expect(s1.getByTestId('episode-state')).toHaveText([/Chosen|Downloading/, /Chosen|Downloading/], { timeout: 15_000 });
	expect(await searchItems(s1)).toEqual(['', '']);
});

test('a Release turned down for an episode is never picked again for it; Search now takes the next one', async () => {
	await openShow(page, 'Harbour Lights');
	const s2 = await openSeason(page, 'Season 2');
	const ep = s2.getByTestId('episode').filter({ hasText: 'S02E01' });
	await expect(ep.getByTestId('episode-state')).toHaveText('Missing');
	await (await episodeMenu(ep)).getByRole('menuitem', { name: 'Choose a release' }).click();
	const panel = page.getByTestId('show-releases');
	await expect(panel.getByRole('heading', { level: 2 })).toHaveText('Choose a release: S02E01');
	await expect(panel.getByTestId('release')).toHaveCount(2, { timeout: 20_000 });
	const best = release(panel, '.S02E01.1080p.');
	await expect(best.getByTestId('verdict')).toHaveText('Would be chosen');

	await best.getByRole('button', { name: /^Turn down/ }).click();
	const td = page.getByRole('dialog');
	await expect(td).toContainText('never picks it again for S02E01');
	await td.getByLabel('Why (optional)').fill('wrong episode inside');
	await td.getByRole('button', { name: 'Turn down' }).click();
	await expect(best.getByTestId('verdict')).toHaveText('Turned down');

	// Search now looks again: the turned-down copy is refused, the 720p is taken.
	await (await episodeMenu(ep)).getByRole('menuitem', { name: 'Search now' }).click();
	await expect(ep.getByTestId('episode-state')).toHaveText(/Chosen|Downloading/, { timeout: 20_000 });

	// Home names the show and the episode.
	await page.goto('/');
	await expect(page.getByText(/Harbour Lights S02E01: found 2 Releases, 1 turned away, chose Harbour\.Lights\.S02E01\.720p/)).toBeVisible({ timeout: 15_000 });

	// The Turned down list says what it was turned down for; take it off.
	await page.goto('/library/turned-down');
	const row = page.getByTestId('listed-release').filter({ hasText: 'Harbour.Lights.S02E01.1080p' });
	await expect(row.getByTestId('listed-for')).toHaveText('Harbour Lights S02E01');
	await expect(row).toContainText('wrong episode inside');
	await axe(page, 'turned down with episodes');
	await row.getByRole('button', { name: /Take off the list/ }).click();
	await expect(row).toHaveCount(0);
});

test('Settings > Search has the season pack choices', async () => {
	await page.goto('/settings/search');
	await expect(page.getByLabel('A season pack is taken when')).toHaveValue('all');
	await page.getByLabel('Season pack or single episodes').selectOption('always');
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByRole('status')).toContainText('Search settings saved.');
	await page.reload();
	await expect(page.getByLabel('Season pack or single episodes')).toHaveValue('always');
	await page.getByLabel('Season pack or single episodes').selectOption('rank');
	await page.getByRole('button', { name: 'Save' }).click();
	// Scan once the saved note is in, not while it fades in.
	await expect(page.getByRole('status')).toContainText('Search settings saved.');
	await axe(page, 'search settings');
});

test('the show page searches from a phone', async ({ browser }) => {
	const ctx = await browser.newContext({ viewport: { width: 375, height: 812 }, isMobile: true, hasTouch: true });
	const phone = await ctx.newPage();
	await signIn(phone, owner());
	await openShow(phone, 'Lantern Street');
	await expect(phone.getByTestId('search-show')).toBeVisible();
	const s1 = await openSeason(phone, 'Season 1');
	await s1.getByTestId('choose-season').click();
	const panel = phone.getByTestId('show-releases');
	await expect(panel.getByTestId('release').first()).toBeVisible({ timeout: 20_000 });
	await noSidewaysScroll(phone);
	await axe(phone, 'show releases on a phone');
	await ctx.close();
});
