import { pageLabel } from './came-from.svelte.ts';
import { describe, expect, it } from 'vitest';

describe('came-from', () => {
	it('names a page from its title', () => expect(pageLabel('Search · HolyArrs')).toBe('Search'));
});

describe('came-from history index', () => {
	it('reads the index SvelteKit keeps under sveltekit:metadata', async () => {
		const store = new Map<string, string>();
		const g = globalThis as Record<string, unknown>;
		g.sessionStorage = { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => store.set(k, v) };
		g.document = { title: 'FROM · HolyArrs' };
		g.location = { pathname: '/title/show/124364', search: '' };
		g.history = { state: { 'sveltekit:metadata': { historyIndex: 7 } } };
		const { arrived, cameFrom, leaving } = await import('./came-from.svelte.ts');
		leaving();
		arrived('link');
		expect(cameFrom()).toEqual({ path: '/title/show/124364', label: 'FROM', y: 0 });
		for (const k of ['sessionStorage', 'document', 'location', 'history']) delete g[k];
	});
	it('gives Back to Search the same index under either key', async () => {
		const g = globalThis as Record<string, unknown>;
		const { historyIndex } = await import('./came-from.svelte.ts');
		g.history = { state: { 'sveltekit:metadata': { historyIndex: 4 } } };
		expect(historyIndex()).toBe('4');
		g.history = { state: { 'sveltekit:history': 2 } };
		expect(historyIndex()).toBe('2');
		g.history = { state: null };
		expect(historyIndex()).toBe(null);
		delete g.history;
	});
});

import { isAuthPath as _isAuth } from './came-from.svelte.ts';
describe('auth routes', () => {
	it('are never a came-from page', () => {
		for (const p of ['/login', '/welcome', '/login?next=/', '/welcome/x']) expect(_isAuth(p)).toBe(true);
		for (const p of ['/explore/music', '/search', '/']) expect(_isAuth(p)).toBe(false);
	});
});
