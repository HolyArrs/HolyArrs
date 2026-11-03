import { expect, test, type Page } from '@playwright/test';
import { plexPort } from './fakes.mjs';
import { noSidewaysScroll, owner, signIn } from './helpers.ts';

// Plex watchlist sync (#102), with the fake Plex only (fakes.mjs): the
// owner links their Plex account, allows watchlists in Request rules and
// turns on Movies in My account; a movie added to the fake watchlist is
// asked for as them, marked as from the Plex watchlist, and is not asked
// for again. The card is checked at 390 wide and in dark mode. Cleans up:
// the ask's title is deleted and the Plex account unlinked.
test.describe.configure({ mode: 'serial' });

const plex = `http://127.0.0.1:${plexPort}`;
const movie = { ratingKey: 'wl-cinder', type: 'movie', title: 'Cinder Waltz', year: 2012, Guid: [{ id: 'imdb://tt9000140' }, { id: 'tmdb://900014' }] };

let own: Page;
type Call = (method: string, path: string, data?: unknown) => ReturnType<Page['request']['fetch']>;
let call: Call;

async function lab(path: string, body: unknown) {
	const r = await fetch(`${plex}${path}`, { method: 'PUT', body: JSON.stringify(body) });
	expect(r.ok).toBe(true);
}

test.beforeAll(async ({ browser }) => {
	own = await (await browser.newContext({ viewport: { width: 1280, height: 800 } })).newPage();
	await signIn(own, owner());
	const me = (await (await own.request.get('/api/v1/auth/me')).json()) as { csrf: string };
	call = (method, path, data) => own.request.fetch(`/api/v1${path}`, { method, data, headers: { 'X-CSRF-Token': me.csrf } });
});

test.afterAll(async () => {
	await lab('/lab/watchlist', { token: '*', items: [] });
	const asks = (await (await call('GET', '/asks')).json()) as { asks: { tmdb_id: string; title_id?: number }[] };
	for (const a of asks.asks.filter((x) => x.tmdb_id === '900014' && x.title_id)) await call('POST', `/titles/${a.title_id}/delete`, {});
	await call('PUT', '/my/plex-watchlist', { movies: false, shows: false });
	await call('DELETE', '/auth/plex/link');
});

test('a movie on the Plex watchlist becomes an ask, once', async () => {
	test.setTimeout(150_000);
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
	await own.goto('/settings/wishlist');
	await own.getByLabel('Ask from Plex watchlists').check();
	await own.getByTestId('savebar').getByRole('button', { name: 'Save' }).click();
	await expect(own.getByText('Request settings saved.')).toBeVisible();

	await lab('/lab/watchlist', { token: '*', items: [movie, { ratingKey: 'wl-song', type: 'track', title: 'Not a movie' }] });
	await own.goto('/settings/account');
	const card = own.locator('#watchlist-me');
	await expect(card.getByRole('heading', { name: 'Plex watchlist' })).toBeVisible();
	await card.getByLabel('Ask for movies on my Plex watchlist').check();
	await expect(own.getByText('HolyArrs reads your Plex watchlist now')).toBeVisible();
	// Turning it on queues a read straight away (the I/O workers may be busy with other Jobs first).
	await expect
		.poll(
			async () => {
				await own.reload();
				return (await own.locator('#watchlist-me [data-testid="watchlist-items"]').textContent().catch(() => '')) ?? '';
			},
			{ timeout: 60_000, intervals: [2000] }
		)
		.toContain('Cinder Waltz');
	await expect(card.getByTestId('watchlist-items')).toContainText('Asked for');
	await expect(card.getByTestId('watchlist-items')).not.toContainText('Not a movie');
	await expect(card.getByText('Plex: ownerplex')).toBeVisible();

	type Ask = { id: number; tmdb_id: string; people: { from_watchlist?: boolean }[] };
	const mine = async () => ((await (await call('GET', '/asks')).json()) as { asks: Ask[] }).asks.filter((a) => a.tmdb_id === '900014');
	const first = await mine();
	expect(first).toHaveLength(1);
	expect(first[0].people[0].from_watchlist).toBe(true);

	// Read now again: nothing new is asked for.
	await card.getByRole('button', { name: 'Read now' }).click();
	await expect(own.getByText('Reading your Plex watchlist now.')).toBeVisible();
	await own.waitForTimeout(3000);
	expect(await mine()).toHaveLength(1);

	// Phone width and dark mode: the card fits.
	await own.setViewportSize({ width: 390, height: 844 });
	await own.emulateMedia({ colorScheme: 'dark' });
	await own.reload();
	await expect(card.getByTestId('watchlist-items')).toContainText('Cinder Waltz');
	await noSidewaysScroll(own);
	await own.emulateMedia({ colorScheme: 'light' });
});
