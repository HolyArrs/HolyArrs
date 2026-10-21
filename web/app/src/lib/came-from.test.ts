import { describe, expect, it } from 'vitest';
import { fromIcon, pageLabel } from './came-from.svelte.ts';

describe('came-from', () => {
	it('names a page from its title', () => expect(pageLabel('Search · HolyArrs')).toBe('Search'));
	it('picks the icon by page', () => {
		expect(fromIcon('/search?q=drake')).toBe('search');
		expect(fromIcon('/explore')).toBe('compass');
		expect(fromIcon('/people/64295')).toBe('people');
		expect(fromIcon('/')).toBe('home');
		expect(fromIcon('/library?kind=movie')).toBe(null);
	});
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
		expect(cameFrom()).toEqual({ path: '/title/show/124364', label: 'FROM' });
		for (const k of ['sessionStorage', 'document', 'location', 'history']) delete g[k];
	});
});
