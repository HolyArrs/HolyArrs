import { describe, expect, it, vi } from 'vitest';
import { ApiError, createClient, message } from './api.ts';

function reply(status: number, body?: unknown, headers: Record<string, string> = {}) {
	return new Response(body === undefined ? null : typeof body === 'string' ? body : JSON.stringify(body), { status, headers });
}

describe('api client', () => {
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
});
