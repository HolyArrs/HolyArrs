import { expect, test, type BrowserContext, type Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import { finishFirstRun, moviesDir, newOwner, owner, plainForm, signIn, signOut, noSidewaysScroll, libraryStatus, libraryView, addFromSearch } from '../helpers/helpers.ts';

test.describe.configure({ mode: 'serial' });

let page: Page;
let context: BrowserContext;

async function scanFirstRun() {
	await expect(page.locator('main')).toHaveCount(1);
	await expect(page.locator('#main')).toHaveCount(1);
	// The first page fades in as the loader leaves; scan it once it is solid.
	await page.waitForFunction(() => !document.documentElement.classList.contains('hl-arrive') && document.getAnimations().every((a) => a.playState !== 'running'));
	const result = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	expect(result.violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
}

test.beforeAll(async ({ browser }, info) => {
	context = await browser.newContext();
	page = await context.newPage();
	// The desktop project runs after setup, which ran the first test (the owner
	// is made there); its later tests start from a signed-in page.
	if (info.project.name === 'desktop') await signIn(page, owner());
});
test.afterAll(async () => {
	await context.close();
});

test('first run makes the owner, adds the key and folder, then returns Home', async () => {
	await page.goto('/');
	await expect(page).toHaveURL(/\/login/);
	await expect(page.getByRole('heading', { name: 'Set up HolyArrs' })).toBeVisible();

	// No setup code: whoever opens a fresh HolyArrs first makes the owner.
	const o = newOwner();
	await expect(page.getByLabel('Setup code')).toHaveCount(0);
	await page.getByTestId('setup-ways').getByRole('button', { name: 'Password' }).click();
	await page.getByLabel('Name').fill(o.name);
	await page.getByLabel('Password', { exact: true }).fill(o.password);
	await page.getByLabel('Password again').fill(o.password);
	await page.getByRole('button', { name: 'Create account' }).click();

	// First run forks: bring the old apps, or start fresh.
	await expect(page).toHaveURL(/\/welcome\?first=1$/);
	await expect(page.getByRole('heading', { name: 'Coming from other apps?', level: 1 })).toBeVisible();
	await expect(page.getByRole('button', { name: /^Bring my apps/ })).toBeVisible();
	await scanFirstRun();
	// The media step: Movies and TV shows on, Music off, one folder row each.
	// It stays outside the app frame, and a reload stays on it. Metadata needs no key.
	await page.getByRole('button', { name: /^Start fresh/ }).click();
	await expect(page).toHaveURL(/\/welcome\?first=1&step=media$/);
	await expect(page.getByLabel('Movies', { exact: true })).toBeChecked();
	await expect(page.getByLabel('TV shows', { exact: true })).toBeChecked();
	await expect(page.getByLabel('Music', { exact: true })).not.toBeChecked();
	await expect(page.getByLabel('TV shows folder')).toBeVisible();
	await expect(page.getByLabel('Music folder')).toHaveCount(0);
	await page.reload();
	await expect(page.getByRole('heading', { name: /^Where.s your media\?$/, level: 1 })).toBeVisible();
	await expect(page.getByText(/TMDB|read access token/)).toHaveCount(0);
	await expect(page.getByRole('navigation', { name: 'Main' })).toHaveCount(0);
	await scanFirstRun();
	// The browser tests use every part: Music here, Requests through Invite family.
	await page.getByRole('button', { name: 'Back' }).click();
	await finishFirstRun(page, moviesDir, { tv: true, music: true, invite: true });

	await page.goto('/');
	await expect(page).toHaveURL(/\/$/);
	await expect(page.getByRole('heading', { level: 1, name: 'Home' })).toBeVisible();
	await expect(page.getByTestId('home-greeting')).toContainText(/luis/i);
	const gettingStarted = page.getByRole('region', { name: 'Welcome to HolyArrs' });
	await expect(gettingStarted.getByText('1 of 4 done', { exact: true })).toBeVisible();
	await expect(gettingStarted.getByText(/TMDB/)).toHaveCount(0);
	await expect(page.getByText('Metadata server is down')).toHaveCount(0);
	await expect(gettingStarted.getByRole('link', { name: /Choose a Library folder.*\(done\)/ })).toBeVisible();
});

test('sign out, wrong password, then sign in', async () => {
	await signOut(page);
	await expect(page).toHaveURL(/\/login/);
	await plainForm(page);
	await page.getByLabel('Name').fill('luis');
	await page.getByLabel('Password', { exact: true }).fill('wrong password');
	await page.getByRole('button', { name: 'Sign in', exact: true }).click();
	await expect(page.getByRole('alert')).toContainText(/wrong name or password/i);
	await signIn(page, owner());
});

test('the desktop menu reaches every destination and Back works', async () => {
	const nav = page.getByRole('navigation', { name: 'Main' }).first();
	const places: [string, RegExp, string][] = [
		['Movies', /\/library\?kind=movie$/, 'Movies'],
		['Downloads', /\/downloads$/, 'Downloads'],
		['Calendar', /\/calendar(\?view=[a-z]+)?$/, 'Calendar'],
		['History', /\/downloads\/history$/, 'History'],
		['Settings', /\/settings$/, 'Settings'],
		['Home', /\/$/, 'Home']
	];
	// A menu link may carry a status ("Settings, needs attention": a fresh
	// install has no key kit yet), so match the label up to the comma.
	const link = (label: string) => nav.getByRole('link', { name: new RegExp(`^${label}(,|$)`) });
	for (const [label, url, heading] of places) {
		await link(label).click();
		await expect(page).toHaveURL(url);
		await expect(page.getByRole('heading', { level: 1 })).toContainText(heading);
		await expect(link(label)).toHaveAttribute('aria-current', 'page');
		await expect(page.locator('.ui-sidebar a[aria-current="page"]')).toHaveCount(1);
	}
	// Search lives in the top bar; the rest are one level down.
	await page.getByTestId('search-box').click();
	await expect(page).toHaveURL(/\/search/);
	await page.getByTestId('search-close').click();
	await expect(page).not.toHaveURL(/\/search/);
	// Needs a decision lives on the Downloads rows now: its old address opens Downloads on Problems, then Review.
	await page.goto('/downloads/decide');
	await expect(page).toHaveURL(/\/downloads\?show=problems\.decide$/);
	await expect(page.getByRole('heading', { level: 1 })).toContainText('Downloads');
	await link('Settings').click();
	// Open the Linked apps card first.
	await page.getByTestId('settings-group').filter({ hasText: 'Linked apps' }).click();
	await page.getByRole('navigation', { name: 'Settings sections' }).getByRole('link', { name: 'Alerts' }).click();
	await expect(page).toHaveURL(/\/settings\/alerts$/);
	await expect(page.getByRole('heading', { level: 1 })).toContainText('Alerts');
	await page.goBack();
	await expect(page).toHaveURL(/\/settings\/media-servers$/);
	await page.goBack();
	await expect(page).toHaveURL(/\/settings$/);
	await page.goBack();
	await expect(page).toHaveURL(/\/downloads\?show=problems\.decide$/);
});

test('empty pages say what to do next', async () => {
	await page.goto('/library');
	await expect(page.getByRole('heading', { name: 'Nothing here yet' })).toBeVisible();
	await page.goto('/downloads');
	await expect(page.getByRole('heading', { name: 'No downloads yet' })).toBeVisible();
	await page.goto('/calendar');
	await expect(page.getByRole('heading', { name: 'Nothing coming up' })).toBeVisible();
	await page.goto('/search');
	await expect(page.getByRole('heading', { name: 'Search', level: 1 })).toBeVisible();
	await expect(page.getByRole('combobox', { name: 'Search for a title, an artist or a person' })).toBeVisible();
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
	// No credit lines in the app: they live on GitHub.
	await page.goto('/settings/backups');
	for (const p of ['Balanced', 'Light', 'Careful']) await expect(page.getByText(p, { exact: true })).toBeVisible();
	await page.goto('/settings/sign-in');
	await expect(page.getByText('Skip sign-in on my local network')).toBeVisible();
	await page.goto('/health?tab=about');
	await expect(page.getByText('0.0.0-skeleton', { exact: true })).toBeVisible();
	await page.goto('/settings/parts');
	const stages = page.getByTestId('control-stages');
	await expect(stages.getByRole('heading', { name: 'Stages' })).toBeVisible();
	for (const name of ['Backups', 'Download sync', 'Schedules', 'Session clean-up', 'Core workers']) {
		await expect(stages.getByTestId('control-stage').filter({ hasText: name })).toHaveAttribute('data-state', 'running');
	}
	await expect(stages.getByTestId('control-stage').filter({ hasText: 'Backups' }).locator('.chip')).toHaveText('Running');
	await expect(page.getByTestId('control-links').getByRole('heading', { name: 'Linked apps' })).toBeVisible();
});

test('the first-run Library folder keeps its health', async () => {
	await page.goto('/settings/library-folders');
	await expect(page.locator('#main').getByText('Movies', { exact: true })).toBeVisible();
	await expect(page.locator('#main .folder .st span').first()).not.toHaveText(''); // the health word next to the status dot
});

test('make and revoke an API key', async () => {
	await page.goto('/settings/api');
	await page.getByLabel('Key name').fill('e2e script');
	await page.getByRole('button', { name: 'Make a key' }).click();
	await expect(page.getByText('It is shown only once')).toBeVisible();
	await expect(page.locator('code.key')).toContainText('hak_');
	page.once('dialog', (d) => d.accept());
	await page.getByRole('button', { name: 'Revoke' }).click();
	await expect(page.getByText('No API keys')).toBeVisible();
});

test('make a backup and see it listed', async () => {
	await page.goto('/settings/backups');
	await page.getByRole('button', { name: 'Back up now' }).click();
	await expect(page.getByRole('link', { name: 'Download' }).first()).toBeVisible();
	await page.getByRole('radio', { name: /Careful/ }).check();
	await expect(page.getByText('Backup plan saved')).toBeVisible();
});

test('download the key kit from Backups', async () => {
	await page.goto('/settings/backups');
	const got = page.waitForEvent('download');
	await page.getByRole('button', { name: 'Download key kit' }).click();
	expect((await got).suggestedFilename()).toMatch(/\.key$/);
	await expect(page.getByText('Key kit downloaded')).toBeVisible();
});

test('search, add a movie, and the Library updates live', async ({ browser }) => {
	// A second tab watches the Library while the first adds a movie.
	const watcher = await browser.newPage();
	await signIn(watcher, owner());
	await watcher.goto('/library');
	await expect(watcher.getByRole('heading', { name: 'Nothing here yet' })).toBeVisible();

	const dialog = await addFromSearch(page, 'Driftwood Moon', 'Driftwood Moon');
	await dialog.getByLabel('Search when added').uncheck();
	await dialog.getByRole('button', { name: 'Add to Library' }).click();
	await expect(page).toHaveURL(/\/library\/\d+/);

	await expect(watcher.locator('[data-testid="library-card"], [data-testid="library-row"]').first()).toBeVisible();
	await watcher.close();

	await expect(page.getByTestId('hero-library')).toBeVisible();
	await page.getByRole('button', { name: 'More actions', exact: true }).click();
	// A movie is added kept up to date, so the menu offers to stop that.
	await expect(page.getByRole('menuitem', { name: 'Stop keeping up to date' })).toBeVisible();
	await page.keyboard.press('Escape');
});

test('Home reads its rows from one bounded answer', async () => {
	const fullLibrary: string[] = [];
	const record = (request: import('@playwright/test').Request) => {
		if (new URL(request.url()).pathname === '/api/v1/titles') fullLibrary.push(request.url());
	};
	// Let the movie page the last test left finish its own reads first.
	await page.waitForLoadState('networkidle');
	page.on('request', record);
	try {
		const response = page.waitForResponse((answer) => new URL(answer.url()).pathname === '/api/v1/home' && answer.status() === 200);
		await page.goto('/');
		const home = await (await response).json();
		expect(Array.isArray(home.rows)).toBe(true);
		await expect(page.getByRole('heading', { level: 1 })).toContainText('Home');
		// Missing is not on Home: it has its own page and a sidebar link.
		expect(home.rows).not.toContain('missing');
		await expect(page.getByTestId('home-missing')).toHaveCount(0);
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

	const find = page.getByTestId('library-find-input');
	await libraryView(page, 'Posters');
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
	// The Filter menu and the layout switch stay usable after a failed search.
	await expect(page.getByRole('button', { name: 'Sort & filter' })).toBeVisible();
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
