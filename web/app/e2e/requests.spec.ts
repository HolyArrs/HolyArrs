import AxeBuilder from '@axe-core/playwright';
import { expect, test, type APIResponse, type Browser, type Page } from '@playwright/test';
import { mkdirSync, rmSync, truncateSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { discordPort, indexerKey, indexerPort, movieReleaseHash, qbitPort } from './fakes.mjs';
import { ensureInLibrary, noSidewaysScroll, openFromSearch, owner, signIn } from './helpers.ts';

// Requests at phone width (Pixel 7): a member asks for movies, the owner
// approves and turns down from a phone, a Discord Alert (the fake one in
// fakes.mjs) names the member, and the weekly limit says when to try again.
// Then, each with members of its own and titles only these tests use: two
// members ask for different seasons of a show and share one ask, an album
// ask, 4K asks (not in the Library and in it), a failed ask says why (also
// when its download failed), the owner reads their own messages, and Follow
// on title pages.
// Runs after the other projects: the owner, the Movies folder and the TMDB
// key exist. There is no show or music folder here, so show and music asks
// are checked up to the approver's card, not approved.
test.describe.configure({ mode: 'serial' });

let own: Page;
let kim: Page;
const kimPw = `e2e-${Math.random().toString(36).slice(2)}-kim`;
const kimDiscord = '123456789012345678';

async function fresh(browser: Browser): Promise<Page> {
	return (await browser.newContext()).newPage();
}

async function posts(): Promise<string[]> {
	const r = await fetch(`http://127.0.0.1:${discordPort}/posts`);
	return ((await r.json()) as { content: string }[]).map((p) => p.content);
}

// Search opens a movie's title page; Ask lives there. The main Ask only: a
// separate "Ask for … in 4K" can sit beside it.
async function askButton(p: Page, words: string, name: string) {
	await openFromSearch(p, words, name);
	return p.getByTestId('title-detail').getByRole('button', { name: new RegExp(`^Ask for ${name}( too)?$`) });
}

test.beforeAll(async ({ browser }) => {
	// Run on setup alone, Quiet Orchard (added by movies) is not there yet.
	await ensureInLibrary(browser, 'imdb:tt9000030', 'Quiet Orchard');
	own = await fresh(browser);
	await signIn(own, owner());
});

test('the owner adds a Discord Alert and sends a test', async () => {
	await own.goto('/settings/alerts');
	await expect(own.getByRole('heading', { level: 1, name: 'Alerts' })).toBeVisible();
	await expect(own.getByText('No Alerts yet')).toBeVisible();
	await own.getByRole('button', { name: 'Add an Alert' }).first().click();
	await own.getByLabel('Name', { exact: true }).fill('Family Discord');
	await own.getByLabel('Discord link').fill(`http://127.0.0.1:${discordPort}/api/webhooks/1/stand-in`);
	await own.getByLabel('May reach the local network').check();
	await own.locator('form.sc-form').getByRole('button', { name: 'Save', exact: true }).click();
	const row = own.getByTestId('alert');
	await expect(row).toContainText('Family Discord');
	await expect(row).toContainText('Someone asks · Approved · Declined · Ready');
	await expect(own.locator('body')).not.toContainText('stand-in');
	await row.getByRole('button', { name: 'Send a test' }).click();
	await expect(row.getByTestId('alert-result')).toContainText('Last sent');
	expect((await posts()).some((c) => c.includes('HolyArrs test'))).toBe(true);
	await noSidewaysScroll(own);
});

test('a member asks for a movie from a phone and sees it on the Requests page', async ({ browser }) => {
	await own.goto('/settings/users');
	await own.getByLabel('Note for yourself').fill('Kim');
	await own.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await own.locator('.fresh code').textContent())!.trim();
	kim = await fresh(browser);
	await kim.goto(new URL(link).pathname);
	await kim.getByLabel('Name', { exact: true }).fill('kim');
	await kim.getByLabel('Password', { exact: true }).fill(kimPw);
	await kim.getByLabel('Password again').fill(kimPw);
	await kim.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(kim).toHaveURL(/\/$/);

	await kim.goto('/account');
	await kim.getByLabel('Discord user ID').fill(kimDiscord);
	await kim.getByTestId('savebar').getByRole('button', { name: 'Save' }).click();
	await expect(kim.getByText('Saved where Alerts reach you.')).toBeVisible();

	await kim.goto('/requests');
	await expect(kim.getByTestId('ask-limit')).toContainText('You have 10 movie asks left this week');
	const askCopper = await askButton(kim, 'copper', 'Copper Lighthouse');
	await askCopper.click();
	// Ask never fires in one click: a confirm sheet opens with focus inside.
	const confirmAsk = kim.getByRole('dialog', { name: /^Ask for Copper Lighthouse/ });
	await expect(confirmAsk).toBeVisible();
	await expect(confirmAsk.locator(':focus')).toHaveCount(1);
	await kim.keyboard.press('Escape');
	await expect(confirmAsk).toBeHidden();
	await expect(askCopper).toBeFocused();
	await askCopper.click();
	await confirmAsk.getByRole('button', { name: 'Ask for Copper Lighthouse' }).click();
	await expect(askCopper).toBeDisabled();
	await noSidewaysScroll(kim);
	// A movie already in the Library opens its Library page; there is nothing to ask
	// (only a 4K copy, covered below).
	await openFromSearch(kim, 'orchard', 'Quiet Orchard');
	await expect(kim).toHaveURL(/\/library\/\d+/);
	await expect(kim.getByRole('button', { name: /^Ask for Quiet Orchard( too)?$/ })).toHaveCount(0);

	await kim.locator('nav.tabbar').getByRole('link', { name: 'Requests' }).click();
	await expect(kim.getByTestId('ask-limit')).toContainText('You have 9 movie asks left this week');
	const ask = kim.getByTestId('ask').filter({ hasText: 'Copper Lighthouse' });
	await expect(ask.getByTestId('ask-state')).toHaveText('Requested');
	await expect(ask.getByTestId('asked-by')).toContainText('Asked by kim');
	// Take back lives only in the row's ⋯ menu, as a destructive item with a confirm sheet.
	await expect(ask.getByRole('button', { name: 'Take back' })).toHaveCount(0);
	await ask.getByRole('button', { name: 'More for Copper Lighthouse' }).click();
	await expect(ask.getByRole('menuitem', { name: 'Take back' })).toBeVisible();
	await ask.getByRole('menuitem', { name: 'Take back' }).click();
	const confirmTakeBack = kim.getByRole('dialog', { name: /^Take back Copper Lighthouse/ });
	await expect(confirmTakeBack.getByRole('button', { name: 'Take back' })).toBeVisible();
	await confirmTakeBack.getByRole('button', { name: 'Keep it' }).click();
	await expect(confirmTakeBack).toBeHidden();
	await expect(ask.getByTestId('ask-state')).toHaveText('Requested');
	await expect(ask.getByRole('button', { name: 'Approve' })).toHaveCount(0);
	// Take back through to removal: the ask leaves Requests and, with no
	// row left, focus lands on the page heading.
	await ask.getByRole('button', { name: 'More for Copper Lighthouse' }).click();
	await ask.getByRole('menuitem', { name: 'Take back' }).click();
	await confirmTakeBack.getByRole('button', { name: 'Take back' }).click();
	await expect(confirmTakeBack).toBeHidden();
	await expect(confirmTakeBack.getByTestId('take-back-error')).toHaveCount(0);
	await expect(ask).toHaveCount(0);
	await expect(kim.getByText('Took back Copper Lighthouse.')).toBeVisible();
	await expect(kim.getByRole('heading', { level: 1 })).toBeFocused();
	// Asked again, it is back on the Requests page for the owner to approve.
	const again = await askButton(kim, 'copper', 'Copper Lighthouse');
	await again.click();
	await kim.getByRole('dialog', { name: /^Ask for Copper Lighthouse/ }).getByRole('button', { name: 'Ask for Copper Lighthouse' }).click();
	await expect(again).toBeDisabled();
	await kim.locator('nav.tabbar').getByRole('link', { name: 'Requests' }).click();
	await expect(ask.getByTestId('ask-state')).toHaveText('Requested');
	await noSidewaysScroll(kim);
	expect((await posts()).some((c) => c.includes('kim asked for Copper Lighthouse'))).toBe(true);
});

// An approver's card on the Requests page (the cards list, every width).
function card(p: Page, name: string) {
	return p.getByRole('list', { name: 'Requests' }).getByRole('listitem').filter({ hasText: name });
}

test('the owner approves from a phone: the movie is added and kim is told', async () => {
	await own.goto('/library');
	// Approvers find Requests in the Manage sheet, under Your stuff; the tab shows a dot.
	const tab = own.locator('nav.tabbar').getByRole('button', { name: /^Manage/ });
	await expect(tab).toHaveAccessibleName(/items need attention/);
	await tab.click();
	await own.getByRole('dialog', { name: 'Manage' }).getByRole('link', { name: 'Requests, 1 waiting' }).click();
	// Approvers open on Pending, with the count on the tab.
	await expect(own.getByRole('button', { name: /^Pending 1/ })).toHaveAttribute('aria-pressed', 'true');
	const ask = card(own, 'Copper Lighthouse');
	await expect(ask).toContainText('kim');
	await expect(ask.getByRole('button', { name: 'Open Copper Lighthouse, asked by kim' })).toBeVisible();
	await noSidewaysScroll(own);
	// The card opens its window: the banner says who asked, and the window opens
	// straight into What to add, which holds the Approve.
	await ask.getByRole('button', { name: 'Open Copper Lighthouse, asked by kim' }).click();
	const win = own.getByRole('dialog', { name: 'Copper Lighthouse' });
	await expect(win).toBeVisible();
	await expect(win.getByTestId('ask-banner')).toContainText('kim asked');
	const sheet = win.getByRole('region', { name: 'What to add' });
	await expect(sheet).toBeVisible();
	await expect(sheet).toContainText('Approving adds the movie');
	await noSidewaysScroll(own);
	await sheet.getByRole('button', { name: 'Approve', exact: true }).click();
	await expect(win).toBeHidden();
	// Undo waits at the bottom for a few seconds, then the answer is sent.
	await expect(own.getByRole('status').filter({ hasText: 'Approved Copper Lighthouse.' }).getByRole('button', { name: 'Undo' })).toBeVisible();
	await expect(own.getByRole('button', { name: /^Pending 0/ })).toBeVisible();
	await expect.poll(async () => (await posts()).some((c) => c.startsWith(`<@${kimDiscord}> **Approved: Copper Lighthouse`)), { timeout: 20_000 }).toBe(true);

	// Processing holds it; its window says it is being got for kim.
	await own.getByRole('button', { name: /^Processing/ }).click();
	await ask.getByRole('button', { name: /^Open Copper Lighthouse/ }).click();
	await expect(win.getByTestId('ask-status')).toContainText('Getting it for kim');
	// The Movie page names who asked (the one title page has no Requests section, so
	// who approved is checked in the Events list below).
	await win.getByTestId('ask-title-page').click();
	await expect(own).toHaveURL(/\/library\/\d+/);
	await expect(own.getByTestId('asked-by-line')).toContainText('Asked by kim');
	// The Events list names them (Home no longer has a Latest Events row).
	// kim asked, took it back and asked again: two ask events, the take-back between them.
	const events = ((await (await own.request.get('/api/v1/events?limit=200')).json()) as { events: { Summary: string }[] }).events.map((e) => e.Summary);
	expect(events.filter((e) => /asked for Copper Lighthouse/.test(e))).toHaveLength(2);
	expect(events.filter((e) => /took back the ask for Copper Lighthouse/.test(e))).toHaveLength(1);
	expect(events.filter((e) => /Copper Lighthouse.*: approved by luis/.test(e))).toHaveLength(1);

	await kim.goto('/requests');
	await kim.getByText(/^Show the latest/).click();
	await expect(kim.getByTestId('message').first()).toContainText('Copper Lighthouse (2017) was approved by an admin');
	await expect(kim.getByTestId('ask').filter({ hasText: 'Copper Lighthouse' }).getByTestId('ask-state')).toHaveText('Approved');
	await kim.getByRole('button', { name: 'Mark all read' }).click();
	await expect(kim.getByRole('button', { name: 'Mark all read' })).toHaveCount(0);
});

test('turned down with a note; over the weekly limit kim is told when to try again', async () => {
	const velvet = await askButton(kim, 'velvet', 'Velvet Tide');
	await velvet.click();
	await kim.getByRole('dialog', { name: /^Ask for Velvet Tide/ }).getByRole('button', { name: 'Ask for Velvet Tide' }).click();
	await expect(velvet).toBeDisabled();

	await own.goto('/requests');
	await card(own, 'Velvet Tide').getByRole('button', { name: 'Decline Velvet Tide' }).click();
	const why = own.getByRole('dialog', { name: /^Decline Velvet Tide/ });
	await expect(why.getByLabel(/^Reason/)).toBeFocused();
	await why.getByLabel(/^Reason/).fill('We have it on disc');
	await why.getByRole('button', { name: 'Decline', exact: true }).click();
	await expect(why).toBeHidden();
	await own.getByRole('button', { name: /^Done/ }).click();
	const ask = card(own, 'Velvet Tide');
	// A turn down is sent after its few seconds of Undo (the card says Sending… till then).
	await expect(ask.getByText('Sending…')).toHaveCount(0, { timeout: 15_000 });
	await expect(ask).toContainText('Declined');

	await kim.goto('/requests');
	await kim.getByRole('button', { name: /^Declined/ }).click();
	const mine = kim.getByTestId('ask').filter({ hasText: 'Velvet Tide' });
	await expect(mine.getByTestId('ask-state')).toHaveText('Declined');
	await expect(mine).toContainText('We have it on disc');

	// One movie a week for kim, set on the Users page: Copper Lighthouse used
	// it (the turned down one does not count). Shows and music keep the default.
	await own.goto('/settings/users');
	const row = own.locator('li.person', { hasText: 'kim' });
	await expect(row.getByTestId('user-limits')).toContainText('1 movie ask of 10');
	await row.getByRole('button', { name: 'Change kim' }).click();
	const dlg = own.getByRole('dialog');
	await expect(dlg.getByLabel('Show asks')).toHaveAttribute('placeholder', 'Default (10)');
	await dlg.getByLabel('Movie asks').fill('1');
	await dlg.getByRole('button', { name: 'Save' }).click();
	await expect(row.getByTestId('user-limits')).toContainText('1 movie ask of 1 (own)');
	await expect(row.getByTestId('user-limits')).toContainText('0 show asks of 10');
	await noSidewaysScroll(own);

	await kim.goto('/requests');
	await expect(kim.getByTestId('ask-limit')).toContainText('Try again in 7 days');
	await (await askButton(kim, 'salt choir', 'Salt Choir')).click();
	await kim.getByRole('dialog', { name: /^Ask for Salt Choir/ }).getByRole('button', { name: 'Ask for Salt Choir' }).click();
	await expect(kim.getByRole('alert')).toContainText('try again in 7 days');
	await noSidewaysScroll(kim);
});

// The roadmap items below (#56 seasons, joining and albums, #59 4K, #134 why
// an ask failed, #58 the owner's own messages, #19 Follow). Each test makes
// its own members, so none leans on kim's weekly limit or on another test.
// They sit after the tests above, which count the waiting asks.

const slow = { timeout: 45_000 };
// Wicker Owls in the fake MusicBrainz (fakes.mjs): two albums out and an EP in 2099.
const owls = 'a4444444-0000-4000-8000-000000000004';

/** member makes an invite link as the owner and joins with it on a phone. */
async function member(browser: Browser, name: string): Promise<Page> {
	await own.goto('/settings/users');
	await own.getByLabel('Note for yourself').fill(name);
	await own.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await own.locator('.fresh code').textContent())!.trim();
	const p = await fresh(browser);
	const pw = `e2e-${Math.random().toString(36).slice(2)}-${name}`;
	await p.goto(new URL(link).pathname);
	await p.getByLabel('Name', { exact: true }).fill(name);
	await p.getByLabel('Password', { exact: true }).fill(pw);
	await p.getByLabel('Password again').fill(pw);
	await p.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(p).toHaveURL(/\/$/);
	return p;
}

type Call = (method: 'GET' | 'POST' | 'PUT' | 'DELETE', path: string, data?: unknown) => Promise<APIResponse>;

/** api signs requests with the page's CSRF token, as the app does. */
async function api(p: Page): Promise<Call> {
	const me = (await (await p.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	return (method, path, data) => p.request.fetch(`/api/v1${path}`, { method, data, headers: { 'X-CSRF-Token': me.csrf } });
}

/** inLibrary adds a fake-TMDB movie (no search, not kept up to date) unless it is there; it returns its id. */
async function inLibrary(call: Call, tmdb: number, name: string): Promise<number> {
	const { titles } = (await (await call('GET', '/titles')).json()) as { titles: { id: number; name: string; kind: string; state?: string }[] };
	const have = titles.find((t) => t.kind === 'movie' && t.name === name && t.state !== 'removed');
	if (have) return have.id;
	const added = await call('POST', '/titles', { id: `tmdb:${tmdb}`, search_on_add: false, keep_up_to_date: false, follow: false });
	expect(added.status(), await added.text()).toBe(201);
	return ((await added.json()) as { title: { id: number } }).title.id;
}

/** giveFile imports a stand-in video into a Library movie by hand (as library-tools.spec does), so its file is in. */
async function giveFile(call: Call, titleId: number, name: string, year: number) {
	// Import by hand reads only from known folders: .tmp/by-hand is an extra import folder.
	const byHand = join(dirname(fileURLToPath(import.meta.url)), '.tmp', 'by-hand');
	const now = (await (await call('GET', '/settings/import-folders')).json()) as { extra_folders?: string[] };
	const extra = now.extra_folders ?? [];
	if (!extra.includes(byHand)) {
		const put = await call('PUT', '/settings/import-folders', { extra_folders: [...extra, byHand] });
		expect(put.ok(), await put.text()).toBe(true);
	}
	const dir = join(byHand, name.replace(/[^A-Za-z0-9]+/g, '-').toLowerCase());
	rmSync(dir, { recursive: true, force: true });
	mkdirSync(dir, { recursive: true });
	const file = join(dir, `${name.replace(/[^A-Za-z0-9]+/g, '.')}.${year}.1080p.WEB-DL.H264-LAB.mkv`);
	writeFileSync(file, 'stand-in video '.repeat(64));
	const r = await call('POST', '/manual-import', { items: [{ path: file, title_id: titleId, mode: 'move' }] });
	expect(r.status(), await r.text()).toBe(200);
	const [res] = ((await r.json()) as { results: { outcome: string; error?: string }[] }).results;
	expect(res.outcome, res.error).toBe('placed');
}

/** askOf finds a plain (not 4K) ask by name and state, as the owner reads it. */
async function askOf(call: Call, name: string, state: 'asked' | 'approved') {
	const { asks } = (await (await call('GET', `/asks?state=${state}`)).json()) as { asks: { id: number; name: string; title_id?: number; four_k?: boolean }[] };
	const a = asks.find((x) => x.name === name && !x.four_k);
	expect(a, `${name} is ${state}`).toBeTruthy();
	return a!;
}

test('two members ask for different seasons of a show: one ask, both askers on it', async ({ browser }) => {
	test.setTimeout(120_000);
	const rosa = await member(browser, 'rosa');
	const teo = await member(browser, 'teo');
	const show = 'Kestrel Shoals';
	// Opens the show's Ask sheet and ticks one season.
	const pick = async (p: Page, season: number) => {
		await openFromSearch(p, 'kestrel', show, 'show');
		await p.getByTestId('title-detail').getByRole('button', { name: new RegExp(`^Ask for ${show}( too)?$`) }).click();
		const sheet = p.getByRole('dialog', { name: `Ask for ${show}` });
		await sheet.getByRole('radio', { name: /Choose seasons/ }).check();
		await sheet.getByRole('checkbox', { name: `Season ${season}`, exact: true }).check();
		await noSidewaysScroll(p);
		return sheet;
	};

	const first = await pick(rosa, 1);
	await first.getByRole('button', { name: 'Ask for season 1' }).click();
	await expect(first).toBeHidden();
	await expect(rosa.getByRole('status').filter({ hasText: `Asked for ${show} (season 1)` })).toBeVisible();

	// teo is told someone asked already; season 2 joins rosa's ask and widens it.
	const second = await pick(teo, 2);
	await expect(second).toContainText('Someone asked for season 1 already');
	await second.getByRole('button', { name: 'Ask for season 2' }).click();
	await expect(second).toBeHidden();
	await expect(teo.getByRole('status').filter({ hasText: 'you are on the ask too' })).toBeVisible();

	for (const p of [rosa, teo]) {
		await p.goto('/requests');
		const row = p.getByTestId('ask').filter({ hasText: show });
		await expect(row).toHaveCount(1);
		await expect(row.getByTestId('ask-state')).toHaveText('Requested');
		await expect(row.getByTestId('ask-kind')).toHaveText('Show');
		await expect(row.getByTestId('ask-seasons')).toHaveText('Asked for seasons 1 and 2');
		await expect(row.getByTestId('asked-by')).toContainText('Asked by rosa and teo');
		await noSidewaysScroll(p);
	}

	// The approver gets one card, under the first asker; the Events name both.
	await own.goto('/requests');
	await own.getByRole('button', { name: /^Pending/ }).click();
	await expect(card(own, show)).toHaveCount(1);
	await expect(card(own, show).getByRole('button', { name: `Open ${show}, asked by rosa` })).toBeVisible();
	await noSidewaysScroll(own);
	const events = ((await (await own.request.get('/api/v1/events?limit=200')).json()) as { events: { Summary: string }[] }).events.map((e) => e.Summary);
	expect(events.filter((e) => e.startsWith(`rosa asked for ${show}`))).toHaveLength(1);
	expect(events.filter((e) => e.startsWith(`teo also asked for ${show}`))).toHaveLength(1);
});

test('a member asks for one album of an artist from its page', async ({ browser }) => {
	test.setTimeout(120_000);
	const nell = await member(browser, 'nell');
	// The fake metadata server has no music, so a member's artist lookup goes
	// to the fake MusicBrainz behind every other caller and can take long.
	// The owner looks Wicker Owls up first (ahead of the queue), so nell's
	// page is answered from the kept copy.
	const seen = await own.request.get(`/api/v1/artists/lookup?term=${encodeURIComponent(`mbid:${owls}`)}`, { timeout: 60_000 });
	expect(await seen.text()).toContain('Wicker Owls');
	await nell.goto(`/music/artist/${owls}`);
	await expect(nell.getByRole('heading', { level: 1, name: 'Wicker Owls' })).toBeVisible(slow);
	// The hero's Ask and the slim bar's copy share a test id; the slim bar is
	// hidden from the accessibility tree until it slides in.
	const ask = nell.getByRole('button').and(nell.getByTestId('artist-ask'));
	await expect(ask).toBeEnabled(slow);
	await ask.click();
	const sheet = nell.getByRole('dialog', { name: 'Ask for Wicker Owls' });
	await sheet.getByRole('radio', { name: /Only some albums/ }).check(slow);
	await sheet.getByRole('checkbox', { name: /^Barn Light/ }).check();
	await noSidewaysScroll(nell);
	await sheet.getByRole('button', { name: 'Ask for Barn Light' }).click();
	await expect(sheet).toBeHidden();
	await expect(nell.getByRole('status').filter({ hasText: 'Asked for Wicker Owls' })).toBeVisible();
	await expect(ask).toBeDisabled();

	await nell.goto('/requests');
	const row = nell.getByTestId('ask').filter({ hasText: 'Wicker Owls' });
	await expect(row.getByTestId('ask-state')).toHaveText('Requested');
	await expect(row.getByTestId('ask-kind')).toHaveText('Music');
	await expect(row.getByTestId('ask-albums')).toHaveText('Asked for Barn Light');
	await noSidewaysScroll(nell);

	await own.goto('/requests');
	await own.getByRole('button', { name: /^Pending/ }).click();
	await expect(card(own, 'Wicker Owls').getByRole('button', { name: 'Open Wicker Owls, asked by nell' })).toBeVisible();
});

test('4K: a member asks for a 4K copy of a movie and of one in the Library; the approver sees 4K', async ({ browser }) => {
	test.setTimeout(120_000);
	const gus = await member(browser, 'gus');
	// Not in the Library: the Ask sheet's 4K box makes it a 4K ask of its own.
	const ask = await askButton(gus, 'saffron', 'Saffron Tramline');
	await ask.click();
	const sheet = gus.getByRole('dialog', { name: /^Ask for Saffron Tramline/ });
	await sheet.getByTestId('four-k-box').getByRole('checkbox').check();
	await noSidewaysScroll(gus);
	await sheet.getByRole('button', { name: 'Ask for Saffron Tramline in 4K' }).click();
	await expect(sheet).toBeHidden();
	await expect(gus.getByTestId('hero-ask-4k')).toHaveAccessibleName('Asked for Saffron Tramline in 4K');
	await expect(gus.getByTestId('hero-ask-4k')).toBeDisabled();

	// In the Library: only a 4K copy can be asked, from its own button.
	await inLibrary(await api(own), 900032, 'Juniper Telegraph');
	await openFromSearch(gus, 'juniper', 'Juniper Telegraph');
	await expect(gus).toHaveURL(/\/library\/\d+/);
	await expect(gus.getByRole('button', { name: /^Ask for Juniper Telegraph( too)?$/ })).toHaveCount(0);
	const fourK = gus.getByTestId('hero-ask-4k');
	await expect(fourK).toHaveAccessibleName('Ask for Juniper Telegraph in 4K');
	await fourK.click();
	const only = gus.getByRole('dialog', { name: /^Ask for Juniper Telegraph/ });
	await expect(only).toContainText('A 4K copy is its own ask');
	await expect(only.getByTestId('four-k-box')).toHaveCount(0);
	await only.getByRole('button', { name: 'Ask for Juniper Telegraph in 4K' }).click();
	await expect(only).toBeHidden();
	await expect(fourK).toHaveAccessibleName('Asked for Juniper Telegraph in 4K');
	await expect(fourK).toBeDisabled();
	// Still asked after a reload: the lookup carries the open 4K ask.
	await gus.reload();
	await expect(gus.getByTestId('hero-ask-4k')).toHaveAccessibleName('Asked for Juniper Telegraph in 4K');
	await noSidewaysScroll(gus);

	await gus.goto('/requests');
	for (const name of ['Saffron Tramline', 'Juniper Telegraph']) {
		await expect(gus.getByTestId('ask').filter({ hasText: name }).getByTestId('ask-state')).toHaveText('Requested');
	}

	// The approver's cards carry the 4K tag, and the window says 4K copy.
	await own.goto('/requests');
	await own.getByRole('button', { name: /^Pending/ }).click();
	for (const name of ['Saffron Tramline', 'Juniper Telegraph']) {
		await expect(card(own, name).getByTitle('Asked for a 4K copy, approved on its own')).toBeVisible();
	}
	await noSidewaysScroll(own);
	await card(own, 'Juniper Telegraph').getByRole('button', { name: 'Open Juniper Telegraph, asked by gus' }).click();
	const win = own.getByRole('dialog', { name: 'Juniper Telegraph' });
	await expect(win.getByTestId('ask-banner')).toContainText('4K copy');
	await own.keyboard.press('Escape');
	await expect(win).toBeHidden();
	// Leave no 4K ask waiting: later projects (music) count and step through the Pending ones.
	const call = await api(own);
	const { asks } = (await (await call('GET', '/asks?state=asked')).json()) as { asks: { id: number; name: string; four_k?: boolean }[] };
	for (const a of asks.filter((x) => x.four_k && ['Saffron Tramline', 'Juniper Telegraph'].includes(x.name))) {
		const r = await call('POST', `/asks/${a.id}/turn-down`, { note: '' });
		expect(r.ok(), await r.text()).toBe(true);
	}
});

test('a failed ask says why: the member and the approver read the reason', async ({ browser }) => {
	test.setTimeout(90_000);
	const pip = await member(browser, 'pip');
	const ask = await askButton(pip, 'pewter', 'Pewter Kite');
	await ask.click();
	await pip.getByRole('dialog', { name: /^Ask for Pewter Kite/ }).getByRole('button', { name: 'Ask for Pewter Kite', exact: true }).click();
	await expect(ask).toBeDisabled();

	// Approved, then deleted from the Library before it arrived: the ask cannot be met.
	const call = await api(own);
	const asked = await askOf(call, 'Pewter Kite', 'asked');
	const yes = await call('POST', `/asks/${asked.id}/approve`, {});
	expect(yes.ok(), await yes.text()).toBe(true);
	const titleId = (await askOf(call, 'Pewter Kite', 'approved')).title_id;
	expect(titleId).toBeTruthy();
	const gone = await call('POST', `/titles/${titleId}/delete`, { delete_files: false, never_add: false, apply: true });
	expect(gone.ok(), await gone.text()).toBe(true);

	const why = 'It was deleted from the Library after it was approved.';
	await pip.goto('/requests');
	await pip.getByRole('button', { name: /^Approved/ }).click();
	await expect(pip.getByTestId('ask').filter({ hasText: 'Pewter Kite' }).getByTestId('ask-failed')).toHaveText(why);
	await noSidewaysScroll(pip);

	await own.goto('/requests');
	await own.getByRole('button', { name: /^Processing/ }).click();
	await card(own, 'Pewter Kite').getByRole('button', { name: 'Open Pewter Kite, asked by pip' }).click();
	const win = own.getByRole('dialog', { name: 'Pewter Kite' });
	await expect(win.getByTestId('ask-status')).toContainText(why);
	await own.keyboard.press('Escape');
	await expect(win).toBeHidden();
});

test('an approved ask whose download failed tells the member why', async ({ browser }) => {
	test.setTimeout(180_000);
	const call = await api(own);
	// A Source and a qBittorrent, the fakes in fakes.mjs, unless earlier projects added them; this
	// test takes its own away at the end, so the Sources and Downloaders tests start as before.
	const mine: string[] = [];
	const { sources } = (await (await call('GET', '/sources')).json()) as { sources: { id: number; url: string }[] };
	if (!sources.some((x) => x.url.includes(`:${indexerPort}/`))) {
		const r = await call('POST', '/sources', { name: 'Requests Torznab', format: 'torznab', url: `http://127.0.0.1:${indexerPort}/api`, api_key: indexerKey, allow_local_network: true });
		expect(r.status(), await r.text()).toBe(201);
		const { id } = (await r.json()) as { id: number };
		await call('POST', `/sources/${id}/test`);
		mine.push(`/sources/${id}`);
	}
	const { downloaders } = (await (await call('GET', '/downloaders')).json()) as { downloaders: { id: number; kind: string }[] };
	if (!downloaders.some((d) => d.kind === 'qbittorrent')) {
		// The fake qBittorrent takes any key (a made-up one, like the fakes' stand-in keys).
		const r = await call('POST', '/downloaders', { name: 'Requests qBittorrent', kind: 'qbittorrent', url: `http://127.0.0.1:${qbitPort}`, secret: ['e2e', 'any', 'key'].join('-'), allow_local_network: true });
		expect(r.ok(), await r.text()).toBe(true);
		mine.push(`/downloaders/${((await r.json()) as { id: number }).id}`);
	}

	const wren = await member(browser, 'wren');
	const ask = await askButton(wren, 'garnet', 'Garnet Sextant');
	await ask.click();
	await wren.getByRole('dialog', { name: /^Ask for Garnet Sextant/ }).getByRole('button', { name: 'Ask for Garnet Sextant', exact: true }).click();
	await expect(ask).toBeDisabled();
	const yes = await call('POST', `/asks/${(await askOf(call, 'Garnet Sextant', 'asked')).id}/approve`, {});
	expect(yes.ok(), await yes.text()).toBe(true);
	const titleId = (await askOf(call, 'Garnet Sextant', 'approved')).title_id!;
	expect(titleId).toBeTruthy();

	// Approving searches and sends the 1080p copy; the owner marks it failed, then the 720p copy
	// sent next. The cam copy is all that is left and the Pick rules refuse it.
	type Row = { id: number; title_id?: number; phase: string };
	const sending = async () => {
		const { downloads } = (await (await call('GET', '/downloads/view?asked=1')).json()) as { downloads: Row[] };
		return downloads.find((d) => d.title_id === titleId && ['waiting', 'downloading', 'stalled'].includes(d.phase))?.id ?? 0;
	};
	for (let i = 0; i < 2; i++) {
		await expect.poll(sending, { timeout: 45_000 }).toBeGreaterThan(0);
		const failed = await call('POST', `/downloads/${await sending()}/mark-failed`, {});
		expect(failed.ok(), await failed.text()).toBe(true);
	}

	// wren reads that the download failed, and the reason.
	const why = `The download failed. Marked failed by ${owner().name}.`;
	const row = wren.getByTestId('ask').filter({ hasText: 'Garnet Sextant' });
	await expect(async () => {
		await wren.goto('/requests');
		await wren.getByRole('button', { name: /^Approved/ }).click();
		await expect(row.getByTestId('ask-failed')).toHaveText(why, { timeout: 3000 });
	}).toPass({ timeout: 45_000 });
	await noSidewaysScroll(wren);

	// Leave nothing for later projects: the movie, its turned-down Releases, and this test's Source and qBittorrent.
	const gone = await call('POST', `/titles/${titleId}/delete`, { delete_files: true, never_add: false, apply: true });
	expect(gone.ok(), await gone.text()).toBe(true);
	const { releases } = (await (await call('GET', '/turned-down?limit=500')).json()) as { releases: { id: number; name: string }[] };
	for (const r of releases.filter((x) => x.name.startsWith('Garnet.Sextant.'))) {
		const off = await call('DELETE', `/turned-down/${r.id}`);
		expect(off.ok(), await off.text()).toBe(true);
	}
	for (const path of mine) {
		const off = await call('DELETE', path);
		expect(off.ok(), await off.text()).toBe(true);
	}
});

test('the owner reads the messages about their own asks on Requests', async () => {
	// Owners ask through the API (the title page offers them Add); the owner may
	// auto-approve, else they approve it themselves. Either way a message is left.
	const call = await api(own);
	const made = await call('POST', '/asks', { id: 'tmdb:900034', follow: false });
	expect(made.ok(), await made.text()).toBe(true);
	const { ask } = (await made.json()) as { ask: { id: number; state: string } };
	if (ask.state === 'asked') {
		const yes = await call('POST', `/asks/${ask.id}/approve`, {});
		expect(yes.ok(), await yes.text()).toBe(true);
	}
	await own.goto('/requests');
	const mine = own.getByRole('region', { name: /^Your messages/ });
	await expect(mine).toBeVisible();
	await mine.getByText(/^Show the latest/).click();
	await expect(mine.getByTestId('message').filter({ hasText: 'Thistle Almanac' })).toContainText('was approved');
	await noSidewaysScroll(own);
});

test('Follow on title pages: a show, a movie that lands later, and an album not out yet', async ({ browser }) => {
	test.setTimeout(120_000);
	const dot = await member(browser, 'dot');
	// A show not in the Library: Follow asks for nothing and stays on after a reload.
	await openFromSearch(dot, 'kestrel', 'Kestrel Shoals', 'show');
	const bell = dot.getByTestId('title-detail').getByTestId('follow');
	await expect(bell).toHaveAttribute('aria-pressed', 'false');
	await expect(bell).toHaveText('Follow');
	await expect(bell).toHaveAttribute('title', /Nothing is asked for/);
	await bell.click();
	await expect(bell).toHaveAttribute('aria-pressed', 'true');
	await expect(bell).toHaveText('Following');
	await expect(bell).toHaveAttribute('title', /Tap to stop/);
	await dot.reload();
	await expect(bell).toHaveAttribute('aria-pressed', 'true');
	await noSidewaysScroll(dot);
	await dot.goto('/requests');
	await expect(dot.getByText('Nothing on the Requests page yet')).toBeVisible();

	// A movie not in the Library: Follow waits for it, and dot is told once its file is in.
	await openFromSearch(dot, 'cobalt', 'Cobalt Orrery');
	const movieBell = dot.getByTestId('title-detail').getByTestId('follow');
	await expect(movieBell).toHaveAttribute('aria-pressed', 'false');
	await movieBell.click();
	await expect(movieBell).toHaveAttribute('aria-pressed', 'true');
	await expect(movieBell).toHaveAttribute('title', /you hear when it is in the Library/);
	const told = async () => ((await (await dot.request.get('/api/v1/messages')).json()) as { messages: { text: string }[] }).messages.some((m) => m.text.includes('Cobalt Orrery is in the Library'));
	const ownCall = await api(own);
	const cobalt = await inLibrary(ownCall, 900033, 'Cobalt Orrery');
	// Added with no file is not ready to watch: no word yet.
	await own.waitForTimeout(3000);
	expect(await told()).toBe(false);
	// Its file comes in: the next ready check (every 30 s, and after each import) tells dot.
	await giveFile(ownCall, cobalt, 'Cobalt Orrery', 2016);
	await expect.poll(told, slow).toBe(true);
	await dot.goto('/requests');
	await dot.getByText(/^Show the latest/).click();
	await expect(dot.getByTestId('message').filter({ hasText: 'Cobalt Orrery is in the Library' })).toBeVisible();
	// The follow moved onto the Library title.
	await openFromSearch(dot, 'cobalt', 'Cobalt Orrery');
	await expect(dot).toHaveURL(/\/library\/\d+/);
	await expect(dot.getByTestId('title-detail').getByTestId('follow')).toHaveAttribute('aria-pressed', 'true');

	// A movie already in the Library with no file: following its Library title, dot is told once its file is in.
	const heron = await inLibrary(ownCall, 900037, 'Heron Clockwork');
	await openFromSearch(dot, 'heron', 'Heron Clockwork');
	await expect(dot).toHaveURL(/\/library\/\d+/);
	const heronBell = dot.getByTestId('title-detail').getByTestId('follow');
	await expect(heronBell).toHaveAttribute('aria-pressed', 'false');
	await heronBell.click();
	await expect(heronBell).toHaveAttribute('aria-pressed', 'true');
	const heronTold = async () => ((await (await dot.request.get('/api/v1/messages')).json()) as { messages: { text: string }[] }).messages.filter((m) => m.text.includes('Heron Clockwork is in the Library')).length;
	expect(await heronTold()).toBe(0);
	await giveFile(ownCall, heron, 'Heron Clockwork', 2015);
	await expect.poll(heronTold, slow).toBe(1);
	await dot.goto('/requests');
	await dot.getByText(/^Show the latest/).click();
	await expect(dot.getByTestId('message').filter({ hasText: 'Heron Clockwork is in the Library and ready to watch' })).toBeVisible();

	// An album not out yet, on its artist's page: the bell says you will hear when it is out.
	// Following a Library artist is its own test below (it needs a music folder).
	await dot.goto(`/music/artist/${owls}`);
	const soon = dot.getByRole('button', { name: 'Tell me when Night Fields is out' });
	await expect(soon).toBeVisible(slow);
	await soon.click();
	await expect(dot.getByRole('button', { name: 'Stop following Night Fields' })).toHaveAttribute('aria-pressed', 'true');
	await expect(dot.getByRole('status').filter({ hasText: 'You will hear when Night Fields is out.' })).toBeVisible();
	await dot.reload();
	await expect(dot.getByRole('button', { name: 'Stop following Night Fields' })).toBeVisible(slow);
	await noSidewaysScroll(dot);
});

// Quartz Gulls in the fake MusicBrainz (fakes.mjs): only the artist follow test uses it.
const gulls = 'a6666666-0000-4000-8000-000000000006';

/** musicFolder is a music Library folder's id: music.spec's when it ran first, else one of its own under .tmp. */
async function musicFolder(call: Call): Promise<number> {
	const { library_folders } = (await (await call('GET', '/library-folders')).json()) as { library_folders: { id: number; kind: string }[] };
	const have = library_folders.find((f) => f.kind === 'music');
	if (have) return have.id;
	const path = join(dirname(fileURLToPath(import.meta.url)), '.tmp', 'requests-music');
	mkdirSync(path, { recursive: true });
	const made = await call('POST', '/library-folders', { path, kind: 'music', label: 'Requests music' });
	expect(made.status(), await made.text()).toBe(201);
	return ((await made.json()) as { id: number }).id;
}

// In the full run this test is its own project after the music ones (playwright.config.ts), so
// it uses music.spec's folder and never adds a second one before music.spec counts its folders.
test('a member follows a Library artist: Following stays after a reload and nothing is asked for', async ({ browser }) => {
	test.setTimeout(120_000);
	const ownCall = await api(own);
	const { titles } = (await (await ownCall('GET', '/titles')).json()) as { titles: { id: number; name: string; kind: string; state?: string }[] };
	if (!titles.some((t) => t.kind === 'artist' && t.name === 'Quartz Gulls' && t.state !== 'removed')) {
		// No albums kept up to date: adding it searches for nothing.
		const added = await ownCall('POST', '/artists', { id: `mbid:${gulls}`, library_folder_id: await musicFolder(ownCall), albums: 'none', keep_up_to_date: false, follow: false });
		expect(added.status(), await added.text()).toBe(201);
	}
	const asked = async () => {
		const out: string[] = [];
		for (const state of ['asked', 'approved']) out.push(...((await (await ownCall('GET', `/asks?state=${state}`)).json()) as { asks: { name: string }[] }).asks.map((a) => a.name));
		return out.filter((n) => n === 'Quartz Gulls').length;
	};
	const ada = await member(browser, 'ada');
	await ada.goto(`/music/artist/${gulls}`);
	await expect(ada.getByRole('heading', { level: 1, name: 'Quartz Gulls' })).toBeVisible(slow);
	const bell = ada.getByRole('button', { name: /^Follow(ing)?$/ });
	await expect(bell).toHaveText('Follow', slow);
	await expect(bell).toHaveAttribute('aria-pressed', 'false');
	await bell.click();
	await expect(bell).toHaveText('Following');
	await expect(bell).toHaveAttribute('aria-pressed', 'true');
	await ada.reload();
	await expect(ada.getByRole('heading', { level: 1, name: 'Quartz Gulls' })).toBeVisible(slow);
	await expect(bell).toHaveText('Following', slow);
	await expect(bell).toHaveAttribute('aria-pressed', 'true');
	await noSidewaysScroll(ada);
	// Following asks for nothing.
	expect(await asked()).toBe(0);
	await ada.goto('/requests');
	await expect(ada.getByText('Nothing on the Requests page yet')).toBeVisible();
});

test('Approve on a card still lands when the approver leaves the page at once', async ({ browser }) => {
	const lou = await member(browser, 'lou');
	const ask = await askButton(lou, 'marigold', 'Marigold Bellows');
	await ask.click();
	await lou.getByRole('dialog', { name: /^Ask for Marigold Bellows/ }).getByRole('button', { name: 'Ask for Marigold Bellows', exact: true }).click();
	await expect(ask).toBeDisabled();

	await own.goto('/requests');
	await card(own, 'Marigold Bellows').getByRole('button', { name: 'Approve Marigold Bellows' }).click();
	// Away (in the app) before the card's beat and the Undo time are over: the answer is still sent.
	await own.locator('nav.tabbar').getByRole('link', { name: 'Home', exact: true }).click();
	await expect(own).toHaveURL(/\/$/);
	await expect.poll(async () => {
		const { asks } = (await (await own.request.get('/api/v1/asks?state=approved')).json()) as { asks: { name: string }[] };
		return asks.some((a) => a.name === 'Marigold Bellows');
	}, { timeout: 15_000 }).toBe(true);
});


// #136: three admins (made here, in this throwaway HolyArrs only) decline
// the same movie in turn. Two of them answer one request at the same moment:
// one decision wins and the other is told who answered first. The newest
// request's window then names the others who declined it before, on a phone
// and at desktop width, in light and dark.
const harrow = 'Harrow Teaspoon';

/** admin joins with an Admin invite and sets the display name everyone sees. */
async function admin(browser: Browser, name: string, shown: string): Promise<Page> {
	await own.goto('/settings/users');
	await own.getByLabel('Note for yourself').fill(name);
	await own.locator('form.invite').getByRole('group', { name: 'Role' }).getByLabel('Admin').check();
	await own.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await own.locator('.fresh code').textContent())!.trim();
	const p = await fresh(browser);
	const pw = `e2e-${Math.random().toString(36).slice(2)}-${name}`;
	await p.goto(new URL(link).pathname);
	await p.getByLabel('Name', { exact: true }).fill(name);
	await p.getByLabel('Password', { exact: true }).fill(pw);
	await p.getByLabel('Password again').fill(pw);
	await p.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(p).toHaveURL(/\/$/);
	await p.goto('/account');
	const box = p.getByTestId('box-display-name');
	await box.getByTestId('edit-display-name').click();
	await box.getByRole('textbox').fill(shown);
	await box.getByRole('button', { name: 'Save' }).click();
	await expect(box).toContainText(shown);
	return p;
}

type AskRow = { id: number; name: string; state: string; decided_by?: { name: string; display_name?: string }; also_declined: { name: string; display_name?: string }[] };

/** asksFor reads every request for name, newest first (as an approver sees them). */
async function asksFor(p: Page, name: string): Promise<AskRow[]> {
	const { asks } = (await (await p.request.get('/api/v1/asks')).json()) as { asks: AskRow[] };
	return asks.filter((a) => a.name === name).sort((a, b) => b.id - a.id);
}

/** askHarrow asks for Harrow Teaspoon as p (a member), through the API the Ask button uses. */
async function askHarrow(p: Page): Promise<number> {
	if (!p.url().startsWith('http')) await p.goto('/');
	const r = await p.evaluate(async () => {
		const me = (await (await fetch('/api/v1/auth/me')).json()) as { csrf: string };
		const res = await fetch('/api/v1/asks', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': me.csrf }, body: JSON.stringify({ id: 'tmdb:900601' }) });
		return { status: res.status, body: await res.json() };
	});
	expect(r.status, JSON.stringify(r.body)).toBeLessThan(300);
	return (r.body as { ask: { id: number } }).ask.id;
}

/** declineOnCard declines the waiting Harrow Teaspoon from p's Requests cards (no reason). */
async function declineOnCard(p: Page) {
	await card(p, harrow).getByRole('button', { name: `Decline ${harrow}` }).click();
	const why = p.getByRole('dialog', { name: new RegExp(`^Decline ${harrow}`) });
	await why.getByRole('button', { name: 'Decline', exact: true }).click();
	await expect(why).toBeHidden();
}

test('#136 several admins decline the same movie: the window names who else declined', async ({ browser }) => {
	test.setTimeout(150_000);
	const hal = await member(browser, 'hal');
	const ada = await admin(browser, 't-req-ada', 'Ada Quill');
	const bea = await admin(browser, 't-req-bea', 'Bea Thorn');
	const cy = await admin(browser, 't-req-cy', 'Cy Marsh');
	const stateOf = async (id: number) => (await asksFor(ada, harrow)).find((a) => a.id === id)?.state;

	// 1. Ada declines hal's first request.
	const first = await askHarrow(hal);
	await ada.goto('/requests');
	await declineOnCard(ada);
	await expect.poll(() => stateOf(first), { timeout: 20_000 }).toBe('turned_down');

	// 2. hal asks again; Bea and Cy both press Decline on it at once. One wins,
	// the other is told who answered first (not "Try again").
	const second = await askHarrow(hal);
	await bea.goto('/requests');
	await cy.goto('/requests');
	await expect(card(bea, harrow)).toBeVisible();
	await expect(card(cy, harrow)).toBeVisible();
	await Promise.all([declineOnCard(bea), declineOnCard(cy)]);
	await expect.poll(() => stateOf(second), { timeout: 20_000 }).toBe('turned_down');
	const winner = (await asksFor(ada, harrow)).find((a) => a.id === second)!.decided_by!.display_name!;
	expect(['Bea Thorn', 'Cy Marsh']).toContain(winner);
	const [loserPage, loser] = winner === 'Bea Thorn' ? [cy, 'Cy Marsh'] : [bea, 'Bea Thorn'];
	await expect(loserPage.getByRole('status').filter({ hasText: `${winner} already declined ${harrow}.` })).toBeVisible({ timeout: 20_000 });
	await expect(loserPage.getByText(/Try again/)).toHaveCount(0);

	// 3. hal asks a third time; the admin who lost the race declines it.
	const third = await askHarrow(hal);
	await loserPage.goto('/requests');
	await declineOnCard(loserPage);
	await expect.poll(() => stateOf(third), { timeout: 20_000 }).toBe('turned_down');

	// The server lists the others, newest first, each once, without the request's own decider.
	const rows = await asksFor(ada, harrow);
	const others = (id: number) => rows.find((a) => a.id === id)!.also_declined.map((w) => w.display_name);
	expect(others(third)).toEqual([winner, 'Ada Quill']);
	expect(others(second)).toEqual([loser, 'Ada Quill']);
	expect(others(first)).toEqual([loser, winner]);

	// 4. The newest request's window, as Ada: phone and desktop, light and dark.
	const sentence = `By ${loser}. ${winner} and Ada Quill declined it before.`;
	for (const colorScheme of ['light', 'dark'] as const) {
		for (const viewport of [{ width: 390, height: 844 }, { width: 1280, height: 800 }]) {
			await ada.emulateMedia({ colorScheme });
			await ada.setViewportSize(viewport);
			await ada.goto(`/requests?ask=${third}`);
			const win = ada.getByRole('dialog', { name: harrow });
			await expect(win).toBeVisible();
			const said = win.getByTestId('ask-status');
			await expect(said).toContainText('Declined');
			await expect(said).toContainText(sentence);
			await noSidewaysScroll(ada);
			// Contrast is read once the window's open fade is over (looping art drift aside).
			await ada.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running' || a.effect?.getTiming().iterations === Infinity));
			const axe = await new AxeBuilder({ page: ada }).include('[data-testid="ask-status"]').withRules(['color-contrast']).analyze();
			expect(axe.violations, `${colorScheme} ${viewport.width}: ${JSON.stringify(axe.violations.map((v) => v.nodes.map((n) => n.failureSummary)))}`).toEqual([]);
			await ada.screenshot({ path: test.info().outputPath(`also-declined-${colorScheme}-${viewport.width}.png`) });
		}
	}

	// Who else declined is for approvers only: hal (a member) sees who declined his
	// own request and nothing about the earlier ones, on the page or in the API.
	await hal.goto('/requests');
	await hal.getByRole('button', { name: /^Declined/ }).click();
	const mine = hal.locator(`[data-ask-id="${third}"]`);
	await expect(mine.getByTestId('decided-by')).toContainText(`Declined by ${loser}`);
	await expect(hal.getByText(/declined it before/)).toHaveCount(0);
	const halSees = JSON.stringify(await (await hal.request.get('/api/v1/asks')).json());
	expect(halSees).not.toContain('also_declined');
});

// #139: a member's open Requests page follows their own request live: the
// approver's answer moves it without a reload, and who decided reads as a
// display name or "an admin", never the sign-in name.
test('a member sees the answer to their request live, without a reload', async ({ browser }) => {
	const zed = await member(browser, 'zed');
	await zed.goto('/');
	const made = await zed.evaluate(async () => {
		const me = (await (await fetch('/api/v1/auth/me')).json()) as { csrf: string };
		const r = await fetch('/api/v1/asks', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': me.csrf }, body: JSON.stringify({ id: 'tmdb:900602' }) });
		return { status: r.status, body: await r.json() };
	});
	expect(made.status, JSON.stringify(made.body)).toBeLessThan(300);
	const id = (made.body as { ask: { id: number } }).ask.id;
	await zed.goto('/requests');
	const card = zed.getByTestId('ask').filter({ hasText: 'Quince Lamplight' });
	await expect(card.getByTestId('ask-state')).toHaveText('Requested');
	// The owner answers from somewhere else; zed's page is left alone.
	const me = (await (await own.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	const res = await own.request.post(`/api/v1/asks/${id}/turn-down`, { headers: { 'X-CSRF-Token': me.csrf }, data: { note: '' } });
	expect(res.ok()).toBe(true);
	await expect(card).toHaveCount(0, { timeout: 15_000 });
	await zed.getByRole('button', { name: /^Declined/ }).click();
	await expect(card.getByTestId('ask-state')).toHaveText('Declined');
	await expect(card.getByTestId('decided-by')).toContainText('Declined by an admin');
	await expect(card).not.toContainText('luis');
});

// #139: the person who asked sees their approved request's steps live
// (searching, then nothing found yet) with no percent and no download
// details; the approver's window says what it waits for, not a bare
// "Processing".
test('a member follows their approved request step by step', async ({ browser }) => {
	test.setTimeout(90_000);
	const tamsin = await member(browser, 'tamsin');
	await tamsin.goto('/');
	const made = await tamsin.evaluate(async () => {
		const me = (await (await fetch('/api/v1/auth/me')).json()) as { csrf: string };
		const r = await fetch('/api/v1/asks', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': me.csrf }, body: JSON.stringify({ id: 'tmdb:900603' }) });
		return { status: r.status, body: await r.json() };
	});
	expect(made.status, JSON.stringify(made.body)).toBeLessThan(300);
	const id = (made.body as { ask: { id: number } }).ask.id;
	await tamsin.goto('/requests');
	const card = tamsin.getByTestId('ask').filter({ hasText: 'Rook Lantern' });
	await expect(card.getByTestId('ask-state')).toHaveText('Requested');
	const me = (await (await own.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	expect((await own.request.post(`/api/v1/asks/${id}/approve`, { headers: { 'X-CSRF-Token': me.csrf }, data: {} })).ok()).toBe(true);
	// The card leaves Requested on its own and shows where it is.
	await expect(card).toHaveCount(0, { timeout: 15_000 });
	await tamsin.getByRole('button', { name: /^Approved/ }).click();
	await expect(card.getByTestId('ask-step')).toHaveText(/^(Searching for it now|Nothing found yet.*)$/);
	await expect(card.getByTestId('ask-step')).toContainText('Nothing found yet', { timeout: 45_000 });
	await expect(card).not.toContainText('%');
	await noSidewaysScroll(tamsin);
	// The approver's window: what it waits for, never a bare Processing.
	await own.goto(`/requests?ask=${id}`);
	const said = own.getByRole('dialog', { name: 'Rook Lantern' }).getByTestId('ask-status');
	await expect(said).toContainText('Looking for a release');
	await expect(said).not.toContainText('Processing');
});

// #139: the member's own card follows the download through to the Library:
// "Downloading now", then "Adding it to the Library" (or straight to Ready
// when the import is quicker than the page's next read), never a percent.
test('a member sees their request downloading, then added to the Library', async ({ browser }) => {
	test.setTimeout(120_000);
	const call = await api(own);
	// The fake Source and qBittorrent, unless earlier projects added them; taken away at the end.
	const mine: string[] = [];
	const { sources } = (await (await call('GET', '/sources')).json()) as { sources: { id: number; url: string }[] };
	if (!sources.some((x) => x.url.includes(`:${indexerPort}/`))) {
		const r = await call('POST', '/sources', { name: 'Requests Torznab', format: 'torznab', url: `http://127.0.0.1:${indexerPort}/api`, api_key: indexerKey, allow_local_network: true });
		expect(r.status(), await r.text()).toBe(201);
		const { id } = (await r.json()) as { id: number };
		await call('POST', `/sources/${id}/test`);
		mine.push(`/sources/${id}`);
	}
	const { downloaders } = (await (await call('GET', '/downloaders')).json()) as { downloaders: { id: number; kind: string }[] };
	if (!downloaders.some((d) => d.kind === 'qbittorrent')) {
		const r = await call('POST', '/downloaders', { name: 'Requests qBittorrent', kind: 'qbittorrent', url: `http://127.0.0.1:${qbitPort}`, secret: ['e2e', 'any', 'key'].join('-'), allow_local_network: true });
		expect(r.ok(), await r.text()).toBe(true);
		mine.push(`/downloaders/${((await r.json()) as { id: number }).id}`);
	}

	const odile = await member(browser, 'odile');
	const made = await (await api(odile))('POST', '/asks', { id: 'tmdb:900604' });
	expect(made.status(), await made.text()).toBeLessThan(300);
	const id = ((await made.json()) as { ask: { id: number } }).ask.id;
	await odile.goto('/requests');
	const card = odile.getByTestId('ask').filter({ hasText: 'Cobalt Weathervane' });
	await expect(card.getByTestId('ask-state')).toHaveText('Requested');
	expect((await call('POST', `/asks/${id}/approve`, {})).ok()).toBe(true);
	await expect(card).toHaveCount(0, { timeout: 15_000 });
	await odile.getByRole('button', { name: /^Approved/ }).click();

	// Every card text odile's page shows on the way: none may hold a percent.
	const seen: string[] = [];
	const look = async () => {
		const text = (await card.count()) ? await card.innerText().catch(() => '') : '';
		if (text) seen.push(text);
		return (await card.count()) ? ((await card.getByTestId('ask-step').textContent().catch(() => '')) ?? '').trim() : 'gone';
	};
	await expect.poll(look, { timeout: 45_000, intervals: [250] }).toBe('Downloading now');

	// The download finishes with the movie file in its folder; HolyArrs imports it.
	const qbit = `http://127.0.0.1:${qbitPort}`;
	const prefix = movieReleaseHash(900604, 1).slice(0, 8);
	const hash = ((await (await fetch(`${qbit}/state`)).json()) as { added: string[] }).added.find((h) => h.startsWith(prefix));
	expect(hash, 'a Cobalt Weathervane release went to qBittorrent').toBeTruthy();
	const name = 'Cobalt.Weathervane.2018.1080p.WEB-DL.H264-HOLYARRS';
	const dl = join(dirname(fileURLToPath(import.meta.url)), '.tmp', 'downloads');
	rmSync(join(dl, name), { recursive: true, force: true });
	mkdirSync(join(dl, name), { recursive: true });
	// Big enough for the import's fake checks: 1 MB a minute of a 101-minute movie on disk
	// (sparse, so quick), and a listed size near the Release's own.
	writeFileSync(join(dl, name, `${name}.mkv`), '');
	truncateSync(join(dl, name, `${name}.mkv`), 120 << 20);
	const done = await fetch(`${qbit}/set?hash=${hash}&progress=1&state=stalledUP&save_path=${encodeURIComponent(dl)}&files=${encodeURIComponent(`${name}/${name}.mkv`)}&filesize=${3_900_000_000}`);
	expect(done.ok).toBe(true);
	await expect.poll(look, { timeout: 45_000, intervals: [250] }).toMatch(/^(Adding it to the Library|gone)$/);
	await odile.getByRole('button', { name: /^Ready/ }).click();
	await expect(card).toHaveCount(1, { timeout: 15_000 });
	seen.push(await card.innerText());
	expect(seen.some((t) => t.includes('Downloading now'))).toBe(true);
	expect(seen.filter((t) => t.includes('%'))).toEqual([]);
	await noSidewaysScroll(odile);

	// Leave nothing for later projects: the movie, its torrent, and this test's Source and qBittorrent.
	const titleId = ((await (await call('GET', `/asks?state=available`)).json()) as { asks: { id: number; title_id?: number }[] }).asks.find((a) => a.id === id)?.title_id;
	expect(titleId, 'the approved ask has its title').toBeTruthy();
	expect((await call('POST', `/titles/${titleId}/delete`, { delete_files: true, never_add: false, apply: true })).ok()).toBe(true);
	await fetch(`${qbit}/api/v2/torrents/delete?hashes=${hash}`);
	rmSync(join(dl, name), { recursive: true, force: true });
	for (const path of mine) {
		const off = await call('DELETE', path);
		expect(off.ok(), await off.text()).toBe(true);
	}
});
