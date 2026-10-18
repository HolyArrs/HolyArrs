import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Browser, type Page } from '@playwright/test';
import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { subsPort } from './fakes.mjs';
import { moviesDir, noSidewaysScroll, owner, signIn, libraryTool, titleAction, openSeasonNamed, episodeAction } from './helpers.ts';

// Subtitles (4.16): the owner sets English and Spanish as the default
// languages, reads what the Pass mark means, adds the lab's fake Subtitle
// site (and one that needs an account, which waits for its key), sees every
// movie missing Spanish and searches them all, then on a movie page picks
// the movie's own languages, chooses a subtitle by hand, fixes its timing
// and turns one down; an episode gets its own languages from the show page.
// Phone width, accessibility, and members see none of it. Every subtitle
// comes from cmd/holyarrs-fakesubs; no real site is reached.
test.describe.configure({ mode: 'serial' });

let page: Page;
const showsDir = join(moviesDir, '..', 'shows');
const files = {
	movie: join(moviesDir, 'Quill Atlas (2011)', 'Quill.Atlas.2011.1080p.BluRay.x264-LAB.mkv'),
	episode: join(showsDir, 'Fennel Clock (2020)', 'Season 1', 'Fennel.Clock.S01E01.1080p.WEB-DL.H264-LAB.mkv')
};
const siteURL = `http://127.0.0.1:${subsPort}`;

async function fresh(browser: Browser, width = 1280) {
	const ctx = await browser.newContext({ viewport: { width, height: 900 } });
	return ctx.newPage();
}

async function axe(p: Page, where: string, include?: string) {
	await p.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => undefined))));
	let b = new AxeBuilder({ page: p }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']);
	if (include) b = b.include(include);
	const r = await b.analyze();
	const bad = r.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
	if (bad.length) console.log('AXE', where, bad.map((v) => v.id + ' ' + v.nodes.slice(0, 3).map((n) => n.target.join(' ')).join(' | ')).join(' ;; '));
	expect(bad.map((v) => v.id), where).toEqual([]);
}

async function adopt(folderLabel: string, name: string, button: string, doneText: string) {
	await page.goto('/library/import');
	const lf = page.getByLabel('Library folder');
	await lf.selectOption((await lf.locator('option', { hasText: folderLabel }).first().getAttribute('value')) ?? '');
	await page.getByRole('button', { name: 'Scan (nothing is moved)' }).click();
	const plan = page.getByTestId('adopt-plan');
	await expect(plan).toBeVisible({ timeout: 30_000 });
	await expect(plan.getByRole('checkbox', { name: `Adopt ${name}` })).toBeChecked();
	for (const box of await plan.getByRole('checkbox', { checked: true }).all()) {
		if (!(await box.getAttribute('aria-label'))?.includes(name)) await box.uncheck();
	}
	await page.getByRole('button', { name: button }).click();
	await expect(page.getByTestId('adopt-results')).toContainText(doneText, { timeout: 30_000 });
}

test.beforeAll(async ({ browser }) => {
	for (const f of Object.values(files)) {
		mkdirSync(dirname(f), { recursive: true });
		writeFileSync(f, `video ${f}`);
	}
	page = await fresh(browser);
	page.on('dialog', (d) => void d.accept());
	await signIn(page, owner());
});

test('Settings > Subtitles: English and Spanish become the default languages; the Pass marks say what they mean', async () => {
	await page.goto('/settings');
	await page.getByRole('navigation', { name: 'Settings sections' }).getByRole('link', { name: 'Subtitles' }).click();
	await expect(page.getByRole('heading', { level: 1, name: 'Subtitles' })).toBeVisible();
	const editor = page.getByTestId('wish-editor').first();
	await expect(editor).toContainText('No languages: no subtitles are looked for.');
	await page.getByRole('button', { name: 'Add a language' }).click();
	await page.getByRole('button', { name: 'Add a language' }).click();
	await expect(editor.getByTestId('wish-row')).toHaveCount(2);
	await page.locator('#def-lang-0').selectOption('en');
	await page.locator('#def-lang-1').selectOption('es');
	await expect(page.getByLabel('Episodes')).toHaveValue('90');
	await expect(page.getByLabel('Movies')).toHaveValue('70');
	await page.getByTestId('pass-mark-help').locator('summary').click();
	await expect(page.getByTestId('pass-mark-help')).toContainText('match score from 0 to 100');
	await expect(page.getByLabel('Frame rate from')).toHaveValue('25');
	await expect(page.getByLabel('Frame rate to')).toHaveValue('23.976');
	await page.getByRole('button', { name: 'Save settings' }).click();
	await expect(page.getByText('Subtitle settings saved.')).toBeVisible();
	await page.reload();
	await expect(page.locator('#def-lang-0')).toHaveValue('en');
	await expect(page.locator('#def-lang-1')).toHaveValue('es');
	await axe(page, 'settings subtitles');
});

test('Subtitle sites: the lab site is added and tested; a site that needs your account waits for its key; pause and remove', async () => {
	await page.getByRole('button', { name: 'Add a Subtitle site' }).click();
	await page.getByRole('radio', { name: /^Lab subtitle site \(/ }).check();
	await page.getByRole('textbox', { name: 'Address' }).fill(siteURL);
	await page.getByText('Limits').click();
	await page.getByLabel('Wait between calls (ms)').fill('10');
	await page.getByLabel('Calls per window, at most').fill('5000');
	await page.getByLabel(/May reach the local network/).check();
	await page.getByRole('button', { name: 'Save Subtitle site' }).click();
	const lab = page.getByTestId('subtitle-site').filter({ hasText: 'Lab subtitle site' }).first();
	await expect(lab.getByTestId('subtitle-site-state')).toHaveText('On');
	await expect(lab.getByTestId('subtitle-site-test')).toContainText('The site answered', { timeout: 15_000 });

	// A site that needs your account: refused until the key is there; the key field is masked.
	await page.getByRole('button', { name: 'Add a Subtitle site' }).click();
	const acctRadio = page.getByRole('radio', { name: /with an account/ });
	await acctRadio.check();
	await expect(page.locator('form').getByText('Needs your account').first()).toBeVisible();
	const key = page.getByTestId('site-account').getByLabel('API key');
	await expect(key).toHaveAttribute('type', 'password');
	await page.getByRole('textbox', { name: 'Address' }).fill(siteURL);
	await page.getByLabel(/May reach the local network/).check();
	await page.getByRole('button', { name: 'Save Subtitle site' }).click();
	await expect(page.getByRole('alert')).toContainText('needs your account');
	await key.fill('xxxx-xxxx');
	await page.getByRole('button', { name: 'Save Subtitle site' }).click();
	const acct = page.getByTestId('subtitle-site').filter({ hasText: 'with an account' });
	await expect(acct).toContainText('account saved');
	await expect(acct.getByTestId('subtitle-site-test')).toContainText('The site answered', { timeout: 15_000 });
	await expect(page.locator('body')).not.toContainText('xxxx-xxxx');
	await acct.getByRole('button', { name: /^Pause / }).click();
	await expect(acct.getByTestId('subtitle-site-state')).toHaveText('Paused');
	await expect(acct.getByTestId('subtitle-site-text')).toContainText('Paused by you');
	await acct.getByRole('button', { name: /^Remove / }).click();
	await expect(page.getByTestId('subtitle-site')).toHaveCount(1);
});

test('Missing subtitles: every movie missing Spanish, by show too, and Search all with its progress', async () => {
	await adopt(moviesDir, 'Quill Atlas', 'Adopt 1 movie', '1 movie is now in the Library');
	await adopt('Shows: ', 'Fennel Clock', 'Adopt 1 show', '1 show is now in the Library');
	await page.goto('/library');
	await libraryTool(page, 'Missing subtitles');
	await expect(page.getByRole('heading', { level: 1, name: 'Missing subtitles' })).toBeVisible();
	await page.getByLabel('Language').selectOption('es');
	await page.getByLabel('Kind').selectOption('movie');
	await expect(page).toHaveURL(/language=es&kind=movie/);
	const rows = page.getByTestId('missing-subtitle');
	await expect(async () => {
		await page.reload();
		await expect(rows.filter({ hasText: 'Quill Atlas' })).toHaveCount(1, { timeout: 2000 });
	}).toPass({ timeout: 30_000 });
	for (const r of await rows.all()) await expect(r.getByTestId('missing-language')).toHaveText('Spanish');
	await expect(rows.filter({ hasText: /S\d\dE\d\d/ })).toHaveCount(0);

	// One show: its episode misses English and Spanish.
	await page.getByLabel('Kind').selectOption('');
	await page.getByLabel('Language').selectOption('');
	await page.getByLabel('Movie or show').selectOption({ label: 'Fennel Clock (2)' });
	await expect(rows).toHaveCount(2);
	await expect(rows.first()).toContainText('Fennel Clock S01E01');
	await axe(page, 'missing subtitles');

	// Back to every movie missing Spanish; Search all follows its Jobs.
	await page.getByLabel('Movie or show').selectOption({ label: 'All of them' });
	await page.getByLabel('Language').selectOption('es');
	await page.getByLabel('Kind').selectOption('movie');
	await page.getByTestId('search-all-subtitles').click();
	const progress = page.getByTestId('search-all-progress');
	await expect(progress).toContainText(/Done: searched \d+ files?\./, { timeout: 60_000 });
	await expect(async () => {
		await page.reload();
		await expect(rows.filter({ hasText: 'Quill Atlas' })).toHaveCount(0, { timeout: 2000 });
	}).toPass({ timeout: 30_000 });
});

test('Movie page: its own languages, every result with score and reasons, Choose a subtitle, Sync fix, Turn down, History', async () => {
	await page.goto('/library');
	await page.getByRole('link', { name: /Quill Atlas/ }).first().click();
	const card = page.getByTestId('subtitles');
	await expect(card.getByTestId('subtitle-wish')).toContainText('English · Spanish (the default languages)');
	await expect(card.getByTestId('subtitle-row').filter({ hasText: 'Spanish' })).toContainText('From Lab subtitle site');
	await expect(card.getByTestId('subtitle-row').filter({ hasText: 'Spanish' })).toContainText(/\d+\/100/);

	// Its own languages: English and French.
	await card.getByTestId('subtitle-wish-change').click();
	await card.getByRole('radio', { name: /Its own languages/ }).check();
	await card.getByRole('button', { name: 'Take Spanish off' }).click();
	await card.getByRole('button', { name: 'Add a language' }).click();
	await card.locator('select[id$="-lang-1"]').selectOption('fr');
	await card.getByRole('button', { name: 'Save languages' }).click();
	await expect(page.getByText('Subtitle languages saved.')).toBeVisible();
	await expect(card.getByTestId('subtitle-wish')).toContainText('English · French (set for this movie)');
	await expect(card.getByTestId('subtitle-missing')).toContainText('French');

	// Choose a subtitle by hand: every result with its score and reasons.
	await card.getByTestId('subtitle-choose').click();
	const choices = card.getByTestId('subtitle-choices');
	await expect(choices.getByRole('heading', { name: 'French' })).toBeVisible({ timeout: 30_000 });
	await expect(choices.getByTestId('subtitle-choice').first()).toContainText('matches');
	await expect(choices.getByTestId('subtitle-choice-score').first()).toHaveText(/\d+\/100/);
	await axe(page, 'choose a subtitle');
	await choices.locator('ul[data-lang="fr"]').getByRole('button', { name: /^Choose / }).first().click();
	await expect(page.getByText(/Saved .*\.fr\.srt\./)).toBeVisible({ timeout: 30_000 });
	const fr = card.getByTestId('subtitle-row').filter({ hasText: 'French' });
	await expect(fr).toContainText('Chosen by hand from Lab subtitle site');

	// Sync fix: one second later.
	await fr.getByRole('button', { name: 'Sync fix for French' }).click();
	await fr.getByRole('button', { name: '500 ms later' }).click();
	await fr.getByRole('button', { name: '500 ms later' }).click();
	await expect(fr).toContainText('1 second later');
	await axe(page, 'sync fix');
	await fr.getByRole('button', { name: 'Fix the timing' }).click();
	await expect(page.getByText('Timing fixed. The old file is in the holding folder.')).toBeVisible();

	// Turn down: it goes to the holding folder and is never chosen again.
	const fr2 = card.getByTestId('subtitle-row').filter({ hasText: 'French' });
	await fr2.getByRole('button', { name: 'Turn down French' }).click();
	await fr2.getByLabel('Why (optional)').fill('out of sync');
	await fr2.getByTestId('subtitle-turn-down-yes').click();
	await expect(page.getByText('Subtitle turned down; HolyArrs looks for another one.')).toBeVisible();

	// History lists the subtitle Events.
	await page.reload();
	const hist = page.getByTestId('title-history');
	await expect(hist).toContainText('Subtitles');
	await expect(hist).toContainText(/turned down|Turned down/);
});

test('Show page: the show follows the default languages; one episode gets its own from its row', async () => {
	await page.goto('/library');
	await page.getByRole('link', { name: /Fennel Clock/ }).first().click();
	const showCard = page.getByTestId('subtitles');
	await expect(showCard.getByTestId('subtitle-wish')).toContainText('English · Spanish (the default languages)');
	await expect(showCard.getByTestId('subtitle-missing-link')).toContainText('2 subtitles missing');
	await openSeasonNamed(page, 'Season 1');
	await episodeAction(page, 'S01E01', 'Subtitles');
	const dialog = page.getByTestId('episode-subtitles-dialog');
	await expect(dialog.getByRole('heading', { name: 'Subtitles of S01E01' })).toBeVisible();
	await expect(dialog.getByTestId('subtitle-missing')).toHaveCount(2);
	await dialog.getByTestId('subtitle-wish-change').click();
	await dialog.getByRole('radio', { name: /Its own languages/ }).check();
	await dialog.getByRole('button', { name: 'Take Spanish off' }).click();
	await dialog.getByRole('button', { name: 'Save languages' }).click();
	await expect(dialog.getByTestId('subtitle-wish')).toContainText('English (set for this episode)');
	await expect(dialog.getByTestId('subtitle-missing')).toHaveCount(1);
	await axe(page, 'episode subtitles', 'dialog[open]');
	await dialog.getByTestId('subtitle-search').click();
	await expect(page.getByText('Looking for subtitles now.')).toBeVisible();
	await dialog.getByRole('button', { name: 'Close' }).click();
});

test('the subtitle screens fit a phone and pass the accessibility scan', async ({ browser }) => {
	const ph = await fresh(browser, 390);
	await signIn(ph, owner());
	for (const url of ['/settings/subtitles', '/library/missing-subtitles']) {
		await ph.goto(url);
		await expect(ph.getByRole('heading', { level: 1 })).toBeVisible();
		await ph.waitForLoadState('networkidle');
		await noSidewaysScroll(ph);
		await axe(ph, `phone ${url}`);
	}
	await ph.goto('/library');
	await ph.getByRole('link', { name: /Quill Atlas/ }).first().click();
	const card = ph.getByTestId('subtitles');
	await card.getByTestId('subtitle-choose').click();
	await expect(card.getByTestId('subtitle-choice').first()).toBeVisible({ timeout: 30_000 });
	await card.getByTestId('subtitle-wish-change').click();
	await card.getByRole('radio', { name: /Its own languages/ }).check();
	await noSidewaysScroll(ph);
	await axe(ph, 'phone movie subtitles');
	await ph.context().close();
});

test('members see none of it', async ({ browser }) => {
	await page.goto('/settings/users');
	await page.getByLabel('Note for yourself').fill('Uma');
	await page.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await page.locator('.fresh code').textContent())!.trim();
	const uma = await fresh(browser);
	const pw = `e2e-${Math.random().toString(36).slice(2)}-pw`;
	await uma.goto(new URL(link).pathname);
	await uma.getByLabel('Name').fill('uma');
	await uma.getByLabel('Password', { exact: true }).fill(pw);
	await uma.getByLabel('Password again').fill(pw);
	await uma.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(uma).toHaveURL(/\/$/);
	await uma.goto('/library');
	await expect(uma.getByRole('link', { name: 'Missing subtitles' })).toHaveCount(0);
	await uma.getByRole('link', { name: /Quill Atlas/ }).first().click();
	await expect(uma.getByRole('heading', { level: 1 })).toContainText('Quill Atlas');
	await expect(uma.getByTestId('subtitles')).toHaveCount(0);
	for (const url of ['/library/missing-subtitles', '/settings/subtitles']) {
		await uma.goto(url);
		await expect(uma).not.toHaveURL(new RegExp(url.replace(/\//g, '\\/') + '$'));
	}
	for (const path of ['/api/v1/subtitles/missing', '/api/v1/subtitle-sites', '/api/v1/settings/subtitles']) {
		expect((await uma.request.get(path)).status(), path).toBe(403);
	}
	await uma.context().close();
});
