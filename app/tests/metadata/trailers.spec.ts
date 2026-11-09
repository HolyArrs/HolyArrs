import { expect, test, type BrowserContext, type Page } from '@playwright/test';
import { fakeYouTube, tmdbPort, videoArtist } from '../fakes/index.mjs';
import { owner, signIn } from '../helpers/helpers.ts';

// Trailers from HolyArrs's own reader, against the fake YouTube (fakes/):
// a page only notes a title, the trailer pass (every few seconds here)
// looks it up and keeps the address, and the players use what was kept.
// Lumen Cistern plays; Aster Funicular's first trailer is behind YouTube's
// age sign-in, so its second one plays; both of Gossamer Weir's trailers
// were removed; Pumice Belfry is not looked up yet when its page opens.
// The title page's Trailer button on Gossamer Weir says why at once. An
// artist's music video (an artist only this spec opens) is only noted when its page asks: the
// pass makes it ready, and the page and the in-view batch then answer it.
// Nothing leaves this machine: the browser's calls to YouTube's video
// servers and player are answered here (page routes), everything else
// outside 127.0.0.1 is refused.

test.describe.configure({ mode: 'serial' });

const lumen = '900501';
const aster = '900502';
const weir = '900503';
const belfry = '900504';

interface Answer {
	status: number;
	body: { type?: string; url?: string; key?: string; audio?: boolean; error?: string; reason?: string };
}

let ctx: BrowserContext;
let page: Page;
/** The video addresses the browser asked YouTube's video servers for. */
const videoAsks: string[] = [];
/** The YouTube player embeds the browser opened (their video keys). */
const embedAsks: string[] = [];
// Video requests wait until the test has looked at the player (a stand-in
// file would not play anyway); then they are answered as missing.
let release: () => void = () => {};
let held = new Promise<void>((r) => (release = r));

/** The fake YouTube's player calls so far (video keys, in order). */
async function readerCalls(): Promise<string[]> {
	const r = await fetch(`http://127.0.0.1:${tmdbPort}/youtube/lab/calls`);
	return ((await r.json()) as { player: string[] }).player;
}

async function trailer(id: string): Promise<Answer> {
	const r = await page.request.get(`/api/v1/trailer/movie/${id}?language=en`);
	return { status: r.status(), body: await r.json() };
}

/** waitForPass asks for the title until the trailer pass has kept an answer that fits. */
async function waitForPass(id: string, done: (a: Answer) => boolean): Promise<Answer> {
	let last: Answer = { status: 0, body: {} };
	await expect
		.poll(async () => done((last = await trailer(id))), { message: `the trailer pass answered movie ${id}`, timeout: 45_000, intervals: [500] })
		.toBe(true);
	return last;
}

test.beforeAll(async ({ browser }) => {
	ctx = await browser.newContext();
	await ctx.route((u) => u.hostname !== '127.0.0.1', (r) => r.abort());
	await ctx.route(
		(u) => u.hostname.endsWith('.googlevideo.com'),
		async (r) => {
			videoAsks.push(r.request().url());
			await held;
			await r.fulfill({ status: 404, body: '' }).catch(() => {});
		}
	);
	// The YouTube player: a removed video says so the way YouTube's player
	// does (an onError message); any other is an empty page.
	await ctx.route(
		(u) => u.hostname === 'www.youtube-nocookie.com',
		(r) => {
			const key = decodeURIComponent(new URL(r.request().url()).pathname.split('/')[2] ?? '');
			embedAsks.push(key);
			const gone = fakeYouTube[key] === 'gone' ? `<script>parent.postMessage(JSON.stringify({ event: 'onError', info: 100 }), '*');</script>` : '';
			return r.fulfill({ status: 200, contentType: 'text/html', body: `<!doctype html><title>player</title>${gone}` });
		}
	);
	page = await ctx.newPage();
	await signIn(page, owner());
	// Pages note titles; the pass looks them up. Three are noted now.
	for (const id of [lumen, aster, weir]) {
		const a = await trailer(id);
		expect(a.status, `movie ${id} before the pass`).toBe(404);
		expect(a.body.reason, `movie ${id} before the pass`).toBeUndefined();
	}
});

test.afterEach(() => {
	release();
	held = new Promise<void>((r) => (release = r));
});

test.afterAll(async () => {
	release();
	await ctx.close();
});

test('a title not looked up yet plays in the YouTube player', async () => {
	expect(await readerCalls()).not.toContain('pumiceBel01');
	const asked = page.waitForResponse((r) => r.url().includes(`/api/v1/trailer/movie/${belfry}?`));
	await page.goto(`/title/movie/${belfry}`);
	await expect(page.getByTestId('title-detail').getByRole('heading', { level: 1, name: 'Pumice Belfry' })).toBeVisible();
	// The page only asks for what was kept: nothing yet, so no reason and no address.
	const first = await asked;
	expect(first.status()).toBe(404);
	const miss = await first.json();
	expect(miss.reason).toBeUndefined();
	expect(miss.url).toBeUndefined();
	// The hero's muted trailer falls back to YouTube's player, on the title's own trailer.
	const frame = page.getByTestId('title-hero').locator('iframe[title="Pumice Belfry trailer"]');
	await expect(frame).toHaveAttribute('src', /^https:\/\/www\.youtube-nocookie\.com\/embed\/pumiceBel01\?/, { timeout: 15_000 });
	await expect.poll(() => embedAsks.includes('pumiceBel01')).toBe(true);
	expect(videoAsks.some((u) => u.includes('id=pumiceBel01'))).toBe(false);
	// The ask noted the title: the next pass looks it up.
	const kept = await waitForPass(belfry, (a) => a.status === 200);
	expect(kept.body.key).toBe('pumiceBel01');
	expect(await readerCalls()).toContain('pumiceBel01');
});

test('the pass keeps a stream and the title hero plays its direct address', async () => {
	const kept = await waitForPass(lumen, (a) => a.status === 200);
	expect(kept.body).toMatchObject({ type: 'mp4', key: 'lumenCist01', audio: true });
	const url = kept.body.url ?? '';
	expect(url).toMatch(/^https:\/\/rr1---sn-lab\.googlevideo\.com\/videoplayback\?expire=\d+&id=lumenCist01&itag=18$/);
	expect(await readerCalls()).toContain('lumenCist01');

	await page.goto(`/title/movie/${lumen}`);
	await expect(page.getByTestId('title-detail').getByRole('heading', { level: 1, name: 'Lumen Cistern' })).toBeVisible();
	// The hero's muted trailer is HolyArrs's own player on the kept address, asked straight from YouTube's video servers.
	const video = page.getByTestId('title-hero').locator('video[aria-label="Lumen Cistern trailer"]').first();
	await expect(video).toHaveAttribute('src', url, { timeout: 15_000 });
	await expect.poll(() => videoAsks.includes(url)).toBe(true);
});

test('an age-restricted trailer: the next one is looked up and plays', async () => {
	const kept = await waitForPass(aster, (a) => a.status === 200);
	expect(kept.body.key).toBe('asterPlay02');
	const url = kept.body.url ?? '';
	expect(url).toContain('&id=asterPlay02&');
	// The age-restricted one was asked first, then the next one.
	const calls = await readerCalls();
	expect(calls).toContain('asterAge001');
	expect(calls.indexOf('asterAge001')).toBeLessThan(calls.indexOf('asterPlay02'));

	await page.goto(`/title/movie/${aster}`);
	// The Trailers row lists the age-restricted one first; playing it opens the big player on the trailer that plays.
	await page.getByTestId('title-trailers').getByRole('button', { name: 'Play Aster Funicular red band trailer' }).click();
	const theater = page.getByTestId('theater');
	await expect(theater.locator('video')).toHaveAttribute('src', url);
	await expect.poll(() => videoAsks.includes(url)).toBe(true);
	expect(videoAsks.some((u) => u.includes('id=asterAge001'))).toBe(false);
	// No can't-play line (the dialog's other live region stays empty).
	await expect(theater.getByRole('status').filter({ hasText: /\S/ })).toHaveCount(0);
});

test('every trailer removed: the player says why in plain words', async () => {
	const kept = await waitForPass(weir, (a) => a.status === 404 && !!a.body.reason);
	expect(kept.body).toMatchObject({ reason: 'unavailable', key: '' });
	expect(kept.body.url).toBeUndefined();
	const calls = await readerCalls();
	expect(calls).toContain('weirGone001');
	expect(calls).toContain('weirGone002');

	await page.goto(`/title/movie/${weir}`);
	await page.getByTestId('title-trailers').getByRole('button', { name: 'Play Gossamer Weir official trailer' }).click();
	const theater = page.getByTestId('theater');
	// HolyArrs already knows both were removed: the line shows at once, with no YouTube player behind it.
	const line = theater.getByRole('status').filter({ hasText: /\S/ });
	await expect(line).toHaveText(/^This trailer was removed from YouTube\.\s*Close$/);
	await expect(theater.locator('iframe')).toHaveCount(0);
	expect(embedAsks.some((k) => k.startsWith('weirGone'))).toBe(false);
	expect(videoAsks.some((u) => u.includes('id=weirGone'))).toBe(false);
	// The line is plain words: no reason code reaches the screen.
	await expect(line).not.toContainText('unavailable');
});

test("every trailer removed: the title page's Trailer button says why at once", async () => {
	await waitForPass(weir, (a) => a.status === 404 && a.body.reason === 'unavailable');
	await page.goto(`/title/movie/${weir}`);
	await expect(page.getByTestId('title-detail').getByRole('heading', { level: 1, name: 'Gossamer Weir' })).toBeVisible();
	const embedsBefore = embedAsks.length;
	await page.getByTestId('hero-trailer').click();
	const theater = page.getByTestId('theater');
	// HolyArrs already knows none plays here: the line shows with no YouTube player behind it.
	const line = theater.getByRole('status').filter({ hasText: /\S/ });
	await expect(line).toHaveText(/^This trailer was removed from YouTube\.\s*Close$/);
	await expect(theater.locator('iframe')).toHaveCount(0);
	expect(embedAsks.slice(embedsBefore).some((k) => k.startsWith('weirGone'))).toBe(false);
	await line.getByRole('button', { name: 'Close' }).click();
	await expect(theater).toBeHidden();
});

test("an artist's music video: the page only notes it, the pass makes it ready", async () => {
	// An artist no other spec opens, so nothing made its video ready first.
	const artist = videoArtist;
	const video = `/api/v1/music/artists/${artist}?part=video`;
	const first = await page.request.get(video);
	expect(first.status(), await first.text()).toBe(404);
	expect((await first.json()).error).toBe('no video ready yet');
	// The page ask looked nothing up on YouTube.
	expect(await readerCalls()).not.toContain('tinLantVid1');
	let ready: { key?: string; proxy_url?: string } = {};
	await expect
		.poll(async () => {
			const r = await page.request.get(video);
			if (r.status() === 200) ready = await r.json();
			return r.status();
		}, { message: 'the trailer pass made the music video ready', timeout: 45_000, intervals: [500] })
		.toBe(200);
	expect(ready.key).toBe('tinLantVid1');
	expect(ready.proxy_url).toMatch(/\/api\/v1\/trailer\/stream\/[0-9a-f]+\/manifest\.m3u8$/);
	expect((await readerCalls()).filter((k) => k === 'tinLantVid1')).toHaveLength(1);
	// The in-view batch answers it too, with nothing pending.
	const b = await (await page.request.get(`/api/v1/trailer/batch?ids=artist:${artist}&language=en`)).json();
	expect(b.videos[`artist:${artist}`]?.key).toBe('tinLantVid1');
	expect(b.pending).toEqual([]);
});
