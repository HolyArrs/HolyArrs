import { expect, test, type BrowserContext, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { moviesDir, newOwner, owner, setupCode, signIn, signOut, noSidewaysScroll, libraryStatus, addFromSearch } from './helpers.ts';

test.describe.configure({ mode: 'serial' });

let page: Page;
let context: BrowserContext;

async function scanFirstRun() {
	await expect(page.locator('main')).toHaveCount(1);
	await expect(page.locator('#main')).toHaveCount(1);
	const result = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	expect(result.violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
}

test.beforeAll(async ({ browser }) => {
	context = await browser.newContext();
	page = await context.newPage();
});
test.afterAll(async () => {
	await context.close();
});

test('first run makes the owner, adds the key and folder, then returns Home', async () => {
	await page.goto('/');
	await expect(page).toHaveURL(/\/login/);
	await expect(page.getByRole('heading', { name: 'Set up HolyArrs' })).toBeVisible();

	const o = newOwner();
	await page.getByLabel('Setup code').fill('not-the-code');
	await page.getByLabel('Name').fill(o.name);
	await page.getByLabel('Password', { exact: true }).fill(o.password);
	await page.getByLabel('Password again').fill(o.password);
	await page.getByRole('button', { name: 'Make the owner and sign in' }).click();
	await expect(page.getByRole('alert')).toContainText('wrong setup code');

	await page.getByLabel('Setup code').fill(setupCode());
	await page.getByLabel('Password', { exact: true }).fill(o.password);
	await page.getByLabel('Password again').fill(o.password);
	await page.getByRole('button', { name: 'Make the owner and sign in' }).click();

	// The first-run step: Movies and Shows ticked, Music not; the browser
	// tests use every part, so Music and the Wishlist go on too.
	await expect(page).toHaveURL(/\/welcome\?first=1$/);
	await expect(page.getByRole('heading', { name: 'What do you use?', level: 1 })).toBeVisible();
	await expect(page.getByLabel('Movies', { exact: true })).toBeChecked();
	await expect(page.getByLabel('Shows', { exact: true })).toBeChecked();
	await expect(page.getByLabel('Music', { exact: true })).not.toBeChecked();
	await expect(page.getByLabel('No, just me')).toBeChecked();
	await scanFirstRun();
	await expect(page.getByTestId('welcome')).toContainText('Off: Family and friends can no longer ask');
	await page.getByLabel('Music', { exact: true }).check();
	await page.getByLabel('Subtitles', { exact: true }).check();
	await page.getByLabel('Yes', { exact: true }).check();
	await page.getByRole('button', { name: 'Continue' }).click();

	// First run stays outside the app frame until one folder saves; metadata
	// needs no key, so there is no key step.
	await expect(page).toHaveURL(/\/welcome\?first=1$/);
	await expect(page.getByRole('heading', { name: 'Library folder', level: 1 })).toBeVisible();
	await expect(page.getByText(/TMDB|read access token/)).toHaveCount(0);
	await expect(page.getByRole('navigation', { name: 'Main' })).toHaveCount(0);
	await scanFirstRun();
	await page.getByLabel('Folder path').fill(moviesDir);
	await page.getByLabel(/^Name/).fill('Movies');
	await page.getByRole('button', { name: 'Add folder and open Home' }).click();

	await expect(page).toHaveURL(/\/$/);
	await expect(page.getByRole('heading', { level: 1, name: 'Home' })).toBeVisible();
	await expect(page.locator('header.ui-head')).toContainText('luis');
	await expect(page.getByText('Live', { exact: true })).toBeVisible();
	const gettingStarted = page.getByRole('region', { name: 'Welcome to HolyArrs' });
	await expect(gettingStarted.getByText('1 of 4 done', { exact: true })).toBeVisible();
	await expect(gettingStarted.getByText(/TMDB/)).toHaveCount(0);
	await expect(page.getByText('Metadata server is down')).toHaveCount(0);
	await expect(gettingStarted.getByRole('link', { name: /Choose a Library folder.*\(done\)/ })).toBeVisible();
});

test('sign out, wrong password, then sign in', async () => {
	await signOut(page);
	await expect(page).toHaveURL(/\/login/);
	await page.getByLabel('Name').fill('luis');
	await page.getByLabel('Password', { exact: true }).fill('wrong password');
	await page.getByRole('button', { name: 'Sign in', exact: true }).click();
	await expect(page.getByRole('alert')).toContainText('wrong name or password');
	await signIn(page, owner());
});

test('the desktop menu reaches every destination and Back works', async () => {
	const nav = page.getByRole('navigation', { name: 'Main' }).first();
	const places: [string, RegExp, string][] = [
		['Movies', /\/library\?kind=movie$/, 'Library'],
		['Downloads', /\/downloads$/, 'Downloads'],
		['Calendar', /\/calendar$/, 'Calendar'],
		['History', /\/downloads\/history$/, 'History'],
		['Library tools', /\/library\/held$/, 'Held'],
		['Settings', /\/settings$/, 'Settings'],
		['Home', /\/$/, 'Home']
	];
	for (const [label, url, heading] of places) {
		await nav.getByRole('link', { name: label, exact: true }).click();
		await expect(page).toHaveURL(url);
		await expect(page.getByRole('heading', { level: 1 })).toContainText(heading);
		await expect(nav.getByRole('link', { name: label, exact: true })).toHaveAttribute('aria-current', 'page');
		await expect(page.locator('.ui-sidebar a[aria-current="page"]')).toHaveCount(1);
	}
	// Search lives in the top bar; the rest are one level down.
	await page.getByRole('button', { name: /^Search…/ }).first().click();
	await page.getByRole('combobox', { name: 'Search titles or go to a place' }).fill('Search');
	await page.getByRole('dialog').getByRole('option', { name: /^Search/ }).first().click();
	await expect(page).toHaveURL(/\/search/);
	await nav.getByRole('link', { name: 'Downloads', exact: true }).click();
	await page.locator('main').getByRole('link', { name: 'Needs a decision' }).first().click();
	await expect(page).toHaveURL(/\/downloads\/decide$/);
	await expect(page.getByRole('heading', { level: 1 })).toContainText('Needs a decision');
	await nav.getByRole('link', { name: 'Settings', exact: true }).click();
	await page.getByRole('navigation', { name: 'Settings sections' }).getByRole('link', { name: 'Alerts' }).click();
	await expect(page).toHaveURL(/\/settings\/alerts$/);
	await expect(page.getByRole('heading', { level: 1 })).toContainText('Alerts');
	await page.goBack();
	await expect(page).toHaveURL(/\/settings$/);
	await page.goBack();
	await expect(page).toHaveURL(/\/downloads\/decide$/);
});

test('empty pages say what to do next', async () => {
	await page.goto('/library');
	await expect(page.getByRole('heading', { name: 'Your Library is empty' })).toBeVisible();
	await page.goto('/downloads');
	await expect(page.getByRole('heading', { name: 'No downloads yet' })).toBeVisible();
	await page.goto('/calendar');
	await expect(page.getByRole('heading', { name: 'Nothing coming up' })).toBeVisible();
	await page.goto('/search');
	await expect(page.getByRole('heading', { name: 'Search', level: 1 })).toBeVisible();
	await expect(page.getByRole('searchbox', { name: 'Search for a title, an artist or a person' })).toBeVisible();
});

test('settings pages render their forms', async () => {
	const pages: [string, string][] = [
		['/settings/library-folders', 'Library folders'],
		['/settings/sources', 'Sources'],
		['/settings/downloaders', 'Downloaders'],
		['/settings/tmdb', 'Metadata'],
		['/settings/backups', 'Backups'],
		['/settings/sign-in', 'Sign-in'],
		['/settings/account', 'My account'],
		['/settings/this-app', 'This app']
	];
	for (const [url, heading] of pages) {
		await page.goto(url);
		await expect(page.getByRole('heading', { level: 1, name: heading })).toBeVisible();
	}
	await page.goto('/settings/tmdb');
	await expect(page.getByText('Answering', { exact: true })).toBeVisible();
	await expect(page.locator('input[type=password]')).toHaveCount(0);
	await expect(page.getByText(/read access token|TMDB key/i)).toHaveCount(0);
	await expect(page.getByText('This product uses the TMDB API but is not endorsed or certified by TMDB.')).toBeVisible();
	await page.goto('/settings/backups');
	for (const p of ['Balanced', 'Light', 'Careful']) await expect(page.getByText(p, { exact: true })).toBeVisible();
	await page.goto('/settings/sign-in');
	await expect(page.getByText('Skip sign-in on my local network')).toBeVisible();
	await page.goto('/settings/this-app');
	await expect(page.getByText('0.0.0-skeleton', { exact: true })).toBeVisible();
	// Stages: every supervised loop and worker pool with its state.
	const stages = page.getByTestId('stages');
	await expect(stages.getByRole('heading', { name: 'Stages' })).toBeVisible();
	for (const name of ['Backups', 'Download sync', 'Schedules', 'Session clean-up', 'Core workers']) {
		await expect(stages.getByTestId('stage').filter({ hasText: name })).toHaveAttribute('data-state', 'running');
	}
	await expect(stages.getByTestId('stage').filter({ hasText: 'Backups' }).locator('.chip')).toHaveText('Running');
	await expect(page.getByTestId('links').getByRole('heading', { name: 'Linked apps' })).toBeVisible();
});

test('the first-run Library folder keeps its health', async () => {
	await page.goto('/settings/library-folders');
	await expect(page.locator('#main').getByText('Movies', { exact: true })).toBeVisible();
	await expect(page.locator('#main .chip').first()).not.toHaveText('');
});

test('make and revoke an API key', async () => {
	await page.goto('/settings/sign-in');
	await page.getByLabel('Key name').fill('e2e script');
	await page.getByRole('button', { name: 'Make a key' }).click();
	await expect(page.getByText('It is shown only once')).toBeVisible();
	await expect(page.locator('code.key')).toContainText('hak_');
	page.once('dialog', (d) => d.accept());
	await page.getByRole('button', { name: 'Revoke' }).click();
	await expect(page.getByText('No API keys yet.')).toBeVisible();
});

test('make a backup and see it listed', async () => {
	await page.goto('/settings/backups');
	await page.getByRole('button', { name: 'Make a backup now' }).click();
	await expect(page.getByRole('link', { name: 'Download' }).first()).toBeVisible();
	await page.getByRole('radio', { name: /Careful/ }).check();
	await expect(page.getByText('Backups: Careful.')).toBeVisible();
});

test('search, add a movie, and the Library updates live', async ({ browser }) => {
	// A second tab watches the Library while the first adds a movie.
	const watcher = await browser.newPage();
	await signIn(watcher, owner());
	await watcher.goto('/library');
	await expect(watcher.getByRole('heading', { name: 'Your Library is empty' })).toBeVisible();

	const dialog = await addFromSearch(page, 'Driftwood Moon', 'Driftwood Moon');
	await dialog.getByLabel('Search when added').uncheck();
	await dialog.getByRole('button', { name: 'Add to Library' }).click();
	await expect(page).toHaveURL(/\/library\/\d+/);

	await expect(watcher.locator('.grid .item').first()).toBeVisible();
	await watcher.close();

	await expect(page.getByTestId('search-now')).toBeVisible();
	await page.getByRole('button', { name: 'More actions', exact: true }).click();
	await expect(page.getByRole('menuitemradio', { name: 'Keep up to date' })).toBeVisible();
	await page.keyboard.press('Escape');
});

test('Home reads a bounded Library summary', async () => {
	const fullLibrary: string[] = [];
	const record = (request: import('@playwright/test').Request) => {
		if (new URL(request.url()).pathname === '/api/v1/titles') fullLibrary.push(request.url());
	};
	// Let the movie page the last test left finish its own reads first.
	await page.waitForLoadState('networkidle');
	page.on('request', record);
	try {
		const response = page.waitForResponse((answer) => new URL(answer.url()).pathname === '/api/v1/library-summary' && answer.status() === 200);
		await page.goto('/');
		const summary = await (await response).json();
		expect(summary.missing.length).toBeLessThanOrEqual(8);
		await expect(page.locator('header.ui-head')).toContainText(`${summary.total} ${summary.total === 1 ? 'title' : 'titles'} in your Library`);
		expect(fullLibrary).toEqual([]);
	} finally {
		page.off('request', record);
	}
});

test('Library search debounces the indexed query and ignores stale results', async () => {
	await page.goto('/library');
	const canonical = await page.evaluate(async () => {
		const response = await fetch('/api/v1/titles');
		return (await response.json()).titles;
	});
	const driftwood = canonical.find((title: { name: string }) => title.name === 'Driftwood Moon');
	expect(driftwood).toBeTruthy();

	const calls: string[] = [];
	let answerSlow: (() => void) | undefined;
	const slow = new Promise<void>((resolve) => (answerSlow = resolve));
	await page.route('**/api/v1/titles?*', async (route) => {
		const query = new URL(route.request().url()).searchParams.get('q') ?? '';
		calls.push(query);
		if (query === 'slow alias') await slow;
		if (query === 'broken') return route.fulfill({ status: 500, json: { error: 'Library search failed.' } });
		const titles = query === 'alias café' || query === 'slow alias' ? [driftwood] : [];
		return route.fulfill({ json: { titles } });
	});

	const find = page.getByLabel('Find in the Library');
	const cards = page.getByTestId('library-card');
	await find.fill('alias café');
	await page.waitForTimeout(100);
	expect(calls).not.toContain('alias café');
	await expect.poll(() => calls).toContain('alias café');
	await expect(cards.filter({ hasText: 'Driftwood Moon' })).toBeVisible();
	await libraryStatus(page, 'missing');
	await expect(cards.filter({ hasText: 'Driftwood Moon' })).toBeVisible();
	await libraryStatus(page, 'have');
	await expect(cards.filter({ hasText: 'Driftwood Moon' })).toHaveCount(0);
	await libraryStatus(page, 'all');

	await find.fill('drift');
	await expect(cards.filter({ hasText: 'Driftwood Moon' })).toBeVisible();
	await expect.poll(() => calls).toContain('drift');
	await expect(cards.filter({ hasText: 'Driftwood Moon' })).toBeVisible();
	await find.fill(String(driftwood.year));
	await expect(cards.filter({ hasText: 'Driftwood Moon' })).toBeVisible();

	await find.fill('slow alias');
	await expect.poll(() => calls).toContain('slow alias');
	await find.fill('nothing');
	await expect.poll(() => calls).toContain('nothing');
	await expect(cards).toHaveCount(0);
	answerSlow?.();
	await page.waitForTimeout(50);
	await expect(cards).toHaveCount(0);

	const beforeClear = calls.length;
	await find.fill('clear before debounce');
	await find.clear();
	await page.waitForTimeout(250);
	expect(calls).toHaveLength(beforeClear);
	await expect(cards).toHaveCount(canonical.length);

	await find.fill('broken');
	await expect(page.getByRole('alert')).toContainText('Library search failed.');
	// The Kind chips and the layout switch stay usable after a failed search.
	await expect(page.getByRole('group', { name: 'Kind' })).toBeVisible();
	await expect(page.getByRole('radiogroup', { name: 'View' })).toBeVisible();
	await page.unroute('**/api/v1/titles?*');
});

test('desktop pages never scroll sideways', async () => {
	for (const url of ['/', '/library', '/search', '/downloads', '/calendar', '/settings']) {
		await page.goto(url);
		await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
		await noSidewaysScroll(page);
	}
});
