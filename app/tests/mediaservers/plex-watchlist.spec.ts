import { expect, test, type Page } from '@playwright/test';
import { plexPort } from '../fakes/index.mjs';
import { noSidewaysScroll, owner, signIn } from '../helpers/helpers.ts';

// Plex watchlist sync, with the fake Plex only (fakes/): the
// owner links their Plex account, allows watchlists in Request rules and
// turns on Movies and Shows in My account. The fake watchlist holds a
// movie, a show, a movie already in the Library and a song: the movie and
// the show become asks marked as from the Plex watchlist (on Requests too),
// the Library movie and the song are not asked for, a second read asks for
// nothing new, and with Movies off a movie added later is not asked for.
// Then a member joins from an invite, links their own Plex account in My
// account (a different fake Plex user, with a watchlist only their sign-in
// sees) and turns on Movies: the first movie is asked as the member, the
// second is held back by their weekly limit of one, a Read now asks for
// nothing new, and the approver sees the member's ask from the watchlist.
// The card is checked at 390 wide and in dark mode. Cleans up: the asks are
// taken back or their titles deleted, and both Plex accounts unlinked.
// Titles only this spec uses (fakes/): Rowan Semaphore, Flint Gazebo,
// Nettle Observatory, Larch Ferrymen, Juniper Lantern, Quill Aqueduct.
test.describe.configure({ mode: 'serial' });

const plex = `http://127.0.0.1:${plexPort}`;
const movie = { ratingKey: 'wl-rowan', type: 'movie', title: 'Rowan Semaphore', year: 2012, Guid: [{ id: 'imdb://tt9001020' }, { id: 'tmdb://900102' }] };
const show = { ratingKey: 'wl-larch', type: 'show', title: 'Larch Ferrymen', year: 2019, Guid: [{ id: 'tvdb://810102' }, { id: 'tmdb://800102' }] };
const kept = { ratingKey: 'wl-flint', type: 'movie', title: 'Flint Gazebo', year: 2011, Guid: [{ id: 'tmdb://900103' }] };
const song = { ratingKey: 'wl-song', type: 'track', title: 'Not a movie' };
const later = { ratingKey: 'wl-nettle', type: 'movie', title: 'Nettle Observatory', year: 2010, Guid: [{ id: 'tmdb://900104' }] };
const juniper = { ratingKey: 'wl-juniper', type: 'movie', title: 'Juniper Lantern', year: 2013, Guid: [{ id: 'tmdb://900105' }] };
const quill = { ratingKey: 'wl-quill', type: 'movie', title: 'Quill Aqueduct', year: 2014, Guid: [{ id: 'tmdb://900106' }] };

let own: Page;
type Call = (method: string, path: string, data?: unknown) => ReturnType<Page['request']['fetch']>;
let call: Call;
// The member, once they joined (the third test).
let kidCall: Call | undefined;

type Ask = { id: number; kind: string; name: string; state: string; tmdb_id?: string; title_id?: number; people: { name: string; from_watchlist?: boolean }[] };
type Watch = { movies: boolean; shows: boolean; checked_at?: string; error?: string; items: { name: string; state: string }[] };

async function lab(path: string, body: unknown) {
	const r = await fetch(`${plex}${path}`, { method: 'PUT', body: JSON.stringify(body) });
	expect(r.ok).toBe(true);
}

const ours = new Set(['Rowan Semaphore', 'Larch Ferrymen', 'Flint Gazebo', 'Nettle Observatory', 'Juniper Lantern', 'Quill Aqueduct']);
async function asks(): Promise<Ask[]> {
	return ((await (await call('GET', '/asks')).json()) as { asks: Ask[] }).asks.filter((a) => ours.has(a.name));
}
async function watch(as: Call = call): Promise<Watch> {
	return (await (await as('GET', '/my/plex-watchlist')).json()) as Watch;
}

/** api signs requests with the page's CSRF token, as the app does; id is who signed in. */
async function api(p: Page): Promise<{ id: number; call: Call }> {
	const me = (await (await p.request.get('/api/v1/auth/me')).json()) as { id: number; csrf: string };
	return { id: me.id, call: (method, path, data) => p.request.fetch(`/api/v1${path}`, { method, data, headers: { 'X-CSRF-Token': me.csrf } }) };
}

test.beforeAll(async ({ browser }) => {
	own = await (await browser.newContext({ viewport: { width: 1280, height: 800 } })).newPage();
	await signIn(own, owner());
	call = (await api(own)).call;
});

test.afterAll(async () => {
	await lab('/lab/watchlist', { token: '*', items: [] });
	if (kidCall) {
		await kidCall('PUT', '/my/plex-watchlist', { movies: false, shows: false });
		await kidCall('DELETE', '/auth/plex/my-link');
	}
	await call('PUT', '/my/plex-watchlist', { movies: false, shows: false });
	for (const a of await asks()) {
		if (a.title_id) await call('POST', `/titles/${a.title_id}/delete`, {});
		else await call('DELETE', `/asks/${a.id}`);
	}
	const { titles } = (await (await call('GET', '/titles')).json()) as { titles: { id: number; name: string }[] };
	for (const t of titles.filter((x) => ours.has(x.name))) await call('POST', `/titles/${t.id}/delete`, {});
	await call('DELETE', '/auth/plex/link');
});

test('a movie and a show on the Plex watchlist become asks, once; a Library movie is not asked for', async () => {
	test.setTimeout(180_000);
	// A movie already in the Library (no search, not kept up to date).
	const added = await call('POST', '/titles', { id: 'tmdb:900103', search_on_add: false, keep_up_to_date: false, follow: false });
	expect(added.status(), await added.text()).toBe(201);

	// Link the owner's Plex account as its own fake Plex user.
	await lab('/lab/plex-user', { id: 9100, username: 'ownerplex' });
	const start = (await (await call('POST', '/auth/plex/link/start', {})).json()) as { id: number; auth_url: string };
	await fetch(`${plex}/app/auth`);
	await expect
		.poll(async () => (await (await call('POST', '/auth/plex/link/finish', { id: start.id })).text()), { timeout: 15_000, intervals: [1200] })
		.toContain('ownerplex');

	// The card stays hidden until the owner allows watchlists.
	await own.goto('/settings/account');
	await expect(own.getByRole('heading', { name: 'Devices' })).toBeVisible();
	await expect(own.getByRole('heading', { name: 'Plex watchlist' })).toHaveCount(0);
	await own.goto('/settings/requests');
	await own.getByLabel('Request from Plex watchlists').check();
	await own.getByTestId('savebar').getByRole('button', { name: 'Save' }).click();
	await expect(own.getByText('Request settings saved')).toBeVisible();

	await lab('/lab/watchlist', { token: '*', items: [movie, show, kept, song] });
	await own.goto('/settings/account');
	const card = own.locator('#watchlist-me');
	await expect(card.getByRole('heading', { name: 'Plex watchlist' })).toBeVisible();
	// Turning a switch on queues a read straight away (the I/O workers may be busy with other Jobs first).
	await card.getByLabel('Request movies on my Plex watchlist').check();
	await expect(own.getByText('Plex watchlist on')).toBeVisible();
	await expect(card.getByLabel('Request movies on my Plex watchlist')).toBeChecked();
	await card.getByLabel('Request shows on my Plex watchlist').check();
	await expect(card.getByLabel('Request shows on my Plex watchlist')).toBeChecked();
	await expect(card.getByText('Plex: ownerplex')).toBeVisible();

	const states = async () => Object.fromEntries((await watch()).items.map((i) => [i.name, i.state]));
	await expect
		.poll(states, { timeout: 90_000, intervals: [2000] })
		.toEqual({ 'Rowan Semaphore': 'asked', 'Larch Ferrymen': 'asked', 'Flint Gazebo': 'in_library' });
	expect((await watch()).error ?? '').toBe('');

	await own.reload();
	const items = card.getByTestId('watchlist-items');
	await expect(items).toContainText('Rowan Semaphore');
	await expect(items).toContainText('Larch Ferrymen');
	await expect(items).toContainText('In the Library');
	await expect(items).not.toContainText('Not a movie');

	// One ask each, from the owner, marked as from the watchlist; none for the Library movie.
	const first = await asks();
	expect(first.map((a) => a.name).sort()).toEqual(['Larch Ferrymen', 'Rowan Semaphore']);
	for (const a of first) {
		expect(a.people).toHaveLength(1);
		expect(a.people[0].name).toBe(owner().name);
		expect(a.people[0].from_watchlist).toBe(true);
	}

	// Read now again: nothing new is asked for.
	const before = (await watch()).checked_at;
	await card.getByRole('button', { name: 'Read now' }).click();
	await expect(own.getByText('Reading Plex watchlist')).toBeVisible();
	await expect.poll(async () => (await watch()).checked_at, { timeout: 60_000, intervals: [1500] }).not.toBe(before);
	expect((await asks()).map((a) => a.id).sort()).toEqual(first.map((a) => a.id).sort());

	// The approver sees both on Requests, marked as from the Plex watchlist.
	await own.goto('/requests');
	const desk = own.getByTestId('requests-desktop');
	await expect(desk).toBeVisible();
	for (const a of first) {
		const tab = a.state === 'asked' ? 'Waiting' : a.state === 'approved' ? 'Processing' : 'Done';
		await desk.getByRole('button', { name: new RegExp(`^${tab} \\d+`) }).click();
		const row = desk.locator(`li.bc[data-ask="${a.id}"]`);
		await expect(row).toContainText(a.name);
		await expect(row).toContainText(owner().name);
		await expect(row.getByTestId('ask-from-watchlist')).toHaveText('· from Plex watchlist');
	}
	await expect(desk.locator('li.bc', { hasText: 'Flint Gazebo' })).toHaveCount(0);

	// Phone width and dark mode: the card fits.
	await own.goto('/settings/account');
	await own.setViewportSize({ width: 390, height: 844 });
	await own.emulateMedia({ colorScheme: 'dark' });
	await own.reload();
	await expect(items).toContainText('Rowan Semaphore');
	await noSidewaysScroll(own);
	await own.emulateMedia({ colorScheme: 'light' });
	await own.setViewportSize({ width: 1280, height: 800 });
});

test('with the switches off, new watchlist entries are not asked for', async () => {
	test.setTimeout(120_000);
	await own.goto('/settings/account');
	const card = own.locator('#watchlist-me');
	// Shows stay on, so saving queues a read; let it finish first.
	const saved = (await watch()).checked_at;
	await card.getByLabel('Request movies on my Plex watchlist').uncheck();
	await expect(card.getByLabel('Request movies on my Plex watchlist')).not.toBeChecked();
	await expect.poll(async () => (await watch()).movies).toBe(false);
	await expect.poll(async () => (await watch()).checked_at, { timeout: 60_000, intervals: [1500] }).not.toBe(saved);

	// A movie added to the watchlist now is left alone.
	await lab('/lab/watchlist', { token: '*', items: [later, movie, show, kept] });
	const before = (await watch()).checked_at;
	expect((await call('POST', '/my/plex-watchlist/check', {})).status()).toBe(202);
	await expect.poll(async () => (await watch()).checked_at, { timeout: 60_000, intervals: [1500] }).not.toBe(before);
	expect((await asks()).some((a) => a.name === 'Nettle Observatory')).toBe(false);
	expect((await watch()).items.some((i) => i.name === 'Nettle Observatory')).toBe(false);

	// Both off: there is nothing to read.
	await card.getByLabel('Request shows on my Plex watchlist').uncheck();
	await expect.poll(async () => (await watch()).shows).toBe(false);
	expect((await call('POST', '/my/plex-watchlist/check', {})).status()).toBe(409);
	expect((await asks()).some((a) => a.name === 'Nettle Observatory')).toBe(false);
});

test('a member links Plex in My account: their own watchlist is asked as them, within their weekly limit', async ({ browser }) => {
	test.setTimeout(180_000);
	// The owner makes an invite link; juno joins with it.
	await own.goto('/settings/users');
	await own.getByRole('button', { name: 'Invite someone' }).click();
	await own.getByLabel('Note for yourself').fill('juno');
	await own.getByRole('button', { name: 'Make an invite link' }).click();
	const link = (await own.locator('.fresh code').textContent())!.trim();
	const kid = await (await browser.newContext({ viewport: { width: 1280, height: 800 } })).newPage();
	const pw = `e2e-${Math.random().toString(36).slice(2)}-juno`;
	await kid.goto(new URL(link).pathname);
	await kid.getByLabel('Name', { exact: true }).fill('juno');
	await kid.getByLabel('Password', { exact: true }).fill(pw);
	await kid.getByLabel('Confirm password').fill(pw);
	await kid.getByRole('button', { name: 'Join and sign in' }).click();
	await expect(kid).toHaveURL(/\/$/);
	const juno = await api(kid);
	kidCall = juno.call;

	// One movie a week for juno, so the second watchlist movie is held back.
	const limit = await call('PUT', `/ask-limits/${juno.id}`, { movies_per_week: 1, shows_per_week: null, music_per_week: null });
	expect(limit.ok(), await limit.text()).toBe(true);

	// juno links their own Plex account, a fake Plex user of its own, from My account.
	await lab('/lab/plex-user', { id: 9102, username: 'junoplex' });
	await kid.goto('/settings/account');
	const linked = kid.locator('#linked-me');
	await linked.getByTestId('link-sign-in').click();
	const box = linked.getByTestId('confirm-sign-in');
	if (await box.isVisible().catch(() => false)) {
		await box.getByLabel('HolyArrs password').fill(pw);
		await box.getByRole('button', { name: 'Confirm' }).click();
	}
	const started = kid.waitForResponse((r) => r.url().endsWith('/api/v1/auth/plex/my-link/start'));
	const popup = kid.waitForEvent('popup');
	await linked.getByRole('button', { name: 'Plex', exact: true }).click();
	const pin = (await (await started).json()) as { id: number };
	await (await popup).waitForLoadState().catch(() => {});
	await expect(kid.getByText('Plex junoplex linked')).toBeVisible({ timeout: 20_000 });
	await expect(kid.getByTestId('my-plex-link')).toContainText('Plex, as junoplex');

	// Only juno's Plex sign-in has these two on its watchlist (the fake Plex
	// hands out one token per pin).
	await lab('/lab/watchlist', { token: `ptok${pin.id}`, items: [juniper, quill] });
	await kid.reload();
	const card = kid.locator('#watchlist-me');
	await expect(card.getByRole('heading', { name: 'Plex watchlist' })).toBeVisible();
	await card.getByLabel('Request movies on my Plex watchlist').check();
	await expect(card.getByLabel('Request movies on my Plex watchlist')).toBeChecked();
	await expect(card.getByText('Plex: junoplex')).toBeVisible();

	const states = async () => Object.fromEntries((await watch(juno.call)).items.map((i) => [i.name, i.state]));
	await expect
		.poll(async () => Object.values(await states()).sort(), { timeout: 90_000, intervals: [2000] })
		.toEqual(['asked', 'limit']);
	const first = await states();
	expect(Object.keys(first).sort()).toEqual(['Juniper Lantern', 'Quill Aqueduct']);
	const askedName = Object.keys(first).find((n) => first[n] === 'asked')!;
	const heldName = Object.keys(first).find((n) => first[n] === 'limit')!;
	expect((await watch(juno.call)).error ?? '').toBe('');
	const lim = (await (await juno.call('GET', '/asks/limit')).json()) as { movies: { per_week: number; left: number } };
	expect(lim.movies.per_week).toBe(1);
	expect(lim.movies.left).toBe(0);

	// One ask, from juno, marked as from the watchlist; none for the held-back movie.
	const mine = (await asks()).filter((a) => a.name === askedName || a.name === heldName);
	expect(mine.map((a) => a.name)).toEqual([askedName]);
	expect(mine[0].people).toHaveLength(1);
	expect(mine[0].people[0].name).toBe('juno');
	expect(mine[0].people[0].from_watchlist).toBe(true);

	// Read now: nothing new is asked for, and the limit still holds the second back.
	const before = (await watch(juno.call)).checked_at;
	await card.getByRole('button', { name: 'Read now' }).click();
	await expect(kid.getByText('Reading Plex watchlist')).toBeVisible();
	await expect.poll(async () => (await watch(juno.call)).checked_at, { timeout: 60_000, intervals: [1500] }).not.toBe(before);
	expect(await states()).toEqual(first);
	expect((await asks()).filter((a) => a.name === askedName || a.name === heldName).map((a) => a.id)).toEqual([mine[0].id]);

	// The approver sees it on Requests, asked by juno, from the Plex watchlist.
	await own.goto('/requests');
	const desk = own.getByTestId('requests-desktop');
	await expect(desk).toBeVisible();
	await desk.getByRole('button', { name: /^Waiting \d+/ }).click();
	const row = desk.locator(`li.bc[data-ask="${mine[0].id}"]`);
	await expect(row).toContainText(askedName);
	await expect(row).toContainText('juno');
	await expect(row.getByTestId('ask-from-watchlist')).toHaveText('· from Plex watchlist');
	await expect(desk.locator('li.bc', { hasText: heldName })).toHaveCount(0);
});
