import { describe, expect, it, vi } from 'vitest';
import { ApiError, createClient, isIncomplete, keepFor, message } from './api.ts';

function reply(status: number, body?: unknown, headers: Record<string, string> = {}) {
	return new Response(body === undefined ? null : typeof body === 'string' ? body : JSON.stringify(body), { status, headers });
}

describe('api client', () => {
	it('shares one request between identical reads and forgets it after a change', async () => {
		const f = vi.fn(async () => reply(200, { n: [1] }));
		const c = createClient(f);
		const [a, b] = await Promise.all([c.get<{ n: number[] }>('/x'), c.get<{ n: number[] }>('/x')]);
		expect(f).toHaveBeenCalledTimes(1);
		a.n.push(2);
		expect(b.n).toEqual([1]);
		await c.get('/x');
		expect(f).toHaveBeenCalledTimes(1);
		await c.post('/y', {});
		await c.get('/x');
		expect(f).toHaveBeenCalledTimes(3);
	});

	it('does not keep a failed read', async () => {
		const f = vi.fn(async () => reply(500, { error: 'no' }));
		const c = createClient(f);
		await expect(c.get('/x')).rejects.toBeInstanceOf(ApiError);
		await expect(c.get('/x')).rejects.toBeInstanceOf(ApiError);
		expect(f).toHaveBeenCalledTimes(2);
	});

	it('prefixes /api/v1 and parses JSON', async () => {
		const f = vi.fn(async () => reply(200, { titles: [] }));
		const c = createClient(f);
		expect(await c.get('/titles')).toEqual({ titles: [] });
		expect(f).toHaveBeenCalledWith('/api/v1/titles', expect.objectContaining({ method: 'GET', credentials: 'same-origin' }));
	});

	it('sends the CSRF token on changes but not on reads', async () => {
		const f = vi.fn(async (_u: string, _o?: RequestInit) => reply(200, {}));
		const c = createClient(f);
		c.setCsrf('tok');
		await c.get('/titles');
		await c.post('/titles', { id: 'tmdb:1' });
		const getHeaders = f.mock.calls[0][1]?.headers as Record<string, string>;
		const postInit = f.mock.calls[1][1] as RequestInit;
		const postHeaders = postInit.headers as Record<string, string>;
		expect(getHeaders['X-CSRF-Token']).toBeUndefined();
		expect(postHeaders['X-CSRF-Token']).toBe('tok');
		expect(postHeaders['Content-Type']).toBe('application/json');
		expect(postInit.body).toBe('{"id":"tmdb:1"}');
	});

	it('returns undefined on 204', async () => {
		const c = createClient(async () => reply(204));
		expect(await c.del('/auth/keys/1')).toBeUndefined();
	});

	it('turns error bodies into ApiError with the server message and Retry-After', async () => {
		const c = createClient(async () => reply(429, { error: 'too many failed sign-ins' }, { 'Retry-After': '31' }));
		const err = (await c.post('/auth/login', {}).catch((e: unknown) => e)) as ApiError;
		expect(err).toBeInstanceOf(ApiError);
		expect(err.status).toBe(429);
		expect(err.message).toBe('too many failed sign-ins');
		expect(err.retryAfter).toBe(31);
		expect(message(err)).toBe('too many failed sign-ins');
	});

	it('handles plain-text errors and unreachable servers', async () => {
		const plain = createClient(async () => reply(500, 'boom\n'));
		await expect(plain.get('/x')).rejects.toMatchObject({ status: 500, message: 'boom' });
		const down = createClient(async () => {
			throw new TypeError('fetch failed');
		});
		await expect(down.get('/x')).rejects.toMatchObject({ status: 0 });
	});

	it('reports a lost sign-in on 401, except from the sign-in calls', async () => {
		const c = createClient(async () => reply(401, { error: 'sign in first' }));
		const lost = vi.fn();
		c.onSignedOut(lost);
		await c.get('/titles').catch(() => {});
		expect(lost).toHaveBeenCalledTimes(1);
		await c.post('/auth/login', {}).catch(() => {});
		await c.post('/auth/oidc/finish', {}).catch(() => {});
		await c.post('/auth/forward/login', {}).catch(() => {});
		await c.post('/auth/reauth', {}).catch(() => {});
		await c.get('/auth/me').catch(() => {});
		expect(lost).toHaveBeenCalledTimes(1);
	});

	it('sends raw bodies as octet-stream', async () => {
		const f = vi.fn(async (_u: string, _o?: RequestInit) => reply(202, { staged: true }));
		const c = createClient(f);
		await c.request('/backups/restore', { method: 'POST', raw: 'zipbytes' });
		const h = f.mock.calls[0][1]?.headers as Record<string, string>;
		expect(h['Content-Type']).toBe('application/octet-stream');
		expect(f.mock.calls[0][1]?.body).toBe('zipbytes');
	});

	describe('kept answers', () => {
		it('keeps a read as long as its max-age, at most 60 s', async () => {
			vi.useFakeTimers();
			try {
				const f = vi.fn(async () => reply(200, { a: 1 }, { 'Cache-Control': 'private, max-age=60' }));
				const c = createClient(f);
				await c.get('/music/artists/x?part=core');
				vi.advanceTimersByTime(59_000);
				expect(c.has('/music/artists/x?part=core')).toBe(true);
				await c.get('/music/artists/x?part=core');
				expect(f).toHaveBeenCalledTimes(1);
				vi.advanceTimersByTime(2_000);
				expect(c.has('/music/artists/x?part=core')).toBe(false);
				await c.get('/music/artists/x?part=core');
				expect(f).toHaveBeenCalledTimes(2);
			} finally {
				vi.useRealTimers();
			}
		});

		it('keeps an answer fetched ahead (warm) longer, but never a no-store or incomplete one', async () => {
			vi.useFakeTimers();
			try {
				const f = vi.fn(async (u: string) => (u.endsWith('/no') ? reply(200, { a: 1 }, { 'Cache-Control': 'no-store' }) : u.endsWith('/part') ? reply(200, { incomplete: true }) : reply(200, { a: 1 })));
				const c = createClient(f);
				await c.warm('/metadata/movie/1', 30_000);
				await c.warm('/no', 30_000);
				await c.warm('/part', 30_000);
				await Promise.resolve();
				vi.advanceTimersByTime(20_000);
				expect(c.has('/metadata/movie/1')).toBe(true);
				expect(c.has('/no')).toBe(false);
				expect(c.has('/part')).toBe(false);
				await c.get('/metadata/movie/1');
				expect(f).toHaveBeenCalledTimes(3);
				vi.advanceTimersByTime(11_000);
				expect(c.has('/metadata/movie/1')).toBe(false);
			} finally {
				vi.useRealTimers();
			}
		});

		it('reads the keep from Cache-Control', () => {
			expect(keepFor('private, max-age=60')).toBe(60_000);
			expect(keepFor('private, max-age=3600')).toBe(60_000);
			expect(keepFor(null)).toBe(1_500);
			expect(keepFor('no-store')).toBe(0);
			expect(keepFor('no-cache, max-age=60')).toBe(0);
		});

		it('never keeps an incomplete answer or a no-store one', async () => {
			const f = vi.fn(async () => reply(200, { incomplete: true }, { 'Cache-Control': 'private, max-age=60' }));
			const c = createClient(f);
			await c.get('/music/albums/x?artist=y');
			expect(c.has('/music/albums/x?artist=y')).toBe(false);
			await c.get('/music/albums/x?artist=y');
			expect(f).toHaveBeenCalledTimes(2);
			const g = vi.fn(async () => reply(200, { a: 1 }, { 'Cache-Control': 'no-store' }));
			const d = createClient(g);
			await d.get('/x');
			await d.get('/x');
			expect(g).toHaveBeenCalledTimes(2);
			expect(isIncomplete({ incomplete: false })).toBe(false);
			const h = vi.fn(async () => reply(200, { building: true }, { 'Cache-Control': 'private, max-age=60' }));
			const e = createClient(h);
			await e.get('/new');
			expect(e.has('/new')).toBe(false);
		});

		it('forgets kept answers on a new session token and on a lost sign-in', async () => {
			let status = 200;
			const f = vi.fn(async () => reply(status, status === 200 ? { who: 'ana' } : { error: 'sign in' }, { 'Cache-Control': 'private, max-age=60' }));
			const c = createClient(f);
			c.setCsrf('t1');
			await c.get('/x');
			c.setCsrf('t1');
			expect(c.has('/x')).toBe(true);
			c.setCsrf('t2');
			expect(c.has('/x')).toBe(false);
			await c.get('/x');
			status = 401;
			await expect(c.get('/y')).rejects.toBeInstanceOf(ApiError);
			expect(c.has('/x')).toBe(false);
		});
	});
});
