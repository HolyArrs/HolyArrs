import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createClient } from './api.ts';
import { createViews, VIEW_SEED_WAIT } from './views.svelte.ts';

function server() {
	const puts: Record<string, unknown>[] = [];
	const inits: RequestInit[] = [];
	let failing = false;
	let hold: Promise<void> | null = null;
	const client = createClient(async (url: string, init?: RequestInit) => {
		if (init?.method === 'PUT' && url.endsWith('/my/views')) {
			inits.push(init);
			if (hold) await hold;
			if (failing) return new Response(JSON.stringify({ error: 'down' }), { status: 500 });
			puts.push(JSON.parse(String(init.body)));
			return new Response('{}');
		}
		return new Response(JSON.stringify({ error: 'no route' }), { status: 404 });
	});
	return {
		client,
		puts,
		inits,
		fail: (on: boolean) => (failing = on),
		// Holds the next answers until the returned function is called.
		slow() {
			let go = () => {};
			hold = new Promise((r) => (go = r));
			return () => ((hold = null), go());
		}
	};
}

function memory(start: Record<string, string> = {}) {
	const m = new Map(Object.entries(start));
	return { m, store: { getItem: (k: string) => m.get(k) ?? null, removeItem: (k: string) => void m.delete(k) } };
}

const settle = () => vi.advanceTimersByTimeAsync(0);

describe('account views', () => {
	beforeEach(() => vi.useFakeTimers());
	afterEach(() => vi.useRealTimers());

	it('opens on the picks that came with the session', () => {
		const v = createViews(server().client, () => memory().store);
		v.take({ 'library.view.movie': 'posters', 'calendar.follows': 'everyone' });
		expect(v.get('library.view.movie')).toBe('posters');
		expect(v.get('calendar.follows')).toBe('everyone');
		expect(v.get('library.view.show')).toBeUndefined();
	});

	it('saves a pick at once, in a write that outlives a reload', async () => {
		const s = server();
		const v = createViews(s.client, () => memory().store);
		v.take({});
		v.set('library.view.movie', 'posters');
		await settle();
		expect(s.puts).toEqual([{ 'library.view.movie': 'posters' }]);
		expect(s.inits[0].keepalive).toBe(true);
		expect(v.get('library.view.movie')).toBe('posters');
	});

	it('sends picks made while a save is on its way as one more save, the last one winning', async () => {
		const s = server();
		const v = createViews(s.client, () => memory().store);
		v.take({});
		const go = s.slow();
		v.set('library.view.movie', 'posters');
		v.set('library.view.movie', 'table');
		v.set('calendar.view', 'list');
		go();
		await settle();
		expect(s.puts).toEqual([{ 'library.view.movie': 'posters' }, { 'library.view.movie': 'table', 'calendar.view': 'list' }]);
	});

	it('sends nothing for a pick that did not change', async () => {
		const s = server();
		const v = createViews(s.client, () => memory().store);
		v.take({ 'calendar.view': 'list' });
		v.set('calendar.view', 'list');
		await settle();
		expect(s.puts).toEqual([]);
	});

	it('keeps a pick that failed to save and sends it with the next one', async () => {
		const s = server();
		const v = createViews(s.client, () => memory().store);
		v.take({});
		s.fail(true);
		v.set('library.view.show', 'overview');
		await settle();
		s.fail(false);
		v.set('calendar.follows', 'mine');
		await settle();
		expect(s.puts).toEqual([{ 'library.view.show': 'overview', 'calendar.follows': 'mine' }]);
	});

	it('moves a view this browser kept before onto the account once, after the first page has loaded', async () => {
		const s = server();
		const old = memory({ 'holyarrs.library-view.movie': 'posters', 'holyarrs.library-view.show': 'table', 'holyarrs.library-view.artist': 'grid' });
		const v = createViews(s.client, () => old.store);
		// The account already has a TV shows pick: it wins, and the old one goes.
		v.take({ 'library.view.show': 'rows' });
		expect(v.get('library.view.movie')).toBe('posters');
		expect(v.get('library.view.show')).toBe('rows');
		expect(old.m.has('holyarrs.library-view.show')).toBe(false);
		// A value the old page read as Rows is not moved.
		expect(v.get('library.view.artist')).toBeUndefined();
		expect(old.m.has('holyarrs.library-view.artist')).toBe(false);
		await settle();
		expect(s.puts).toEqual([]);
		await vi.advanceTimersByTimeAsync(VIEW_SEED_WAIT);
		expect(s.puts).toEqual([{ 'library.view.movie': 'posters' }]);
		expect(old.m.has('holyarrs.library-view.movie')).toBe(false);
	});

	it('keeps the old browser pick when moving it failed, to try again next time', async () => {
		const s = server();
		s.fail(true);
		const old = memory({ 'holyarrs.library-view.artist': 'overview' });
		const v = createViews(s.client, () => old.store);
		v.take({});
		await vi.advanceTimersByTimeAsync(VIEW_SEED_WAIT);
		expect(old.m.get('holyarrs.library-view.artist')).toBe('overview');
	});

	it('survives storage that throws', () => {
		const broken = { getItem: () => { throw new Error('no'); }, removeItem: () => { throw new Error('no'); } };
		const v = createViews(server().client, () => broken);
		expect(() => v.take({})).not.toThrow();
	});

	it('forgets the picks and sends nothing once signed out', async () => {
		const s = server();
		const v = createViews(s.client, () => memory().store);
		v.take({ 'library.view.movie': 'posters' });
		v.forget();
		expect(v.get('library.view.movie')).toBeUndefined();
		v.set('library.view.movie', 'overview');
		await settle();
		expect(s.puts).toEqual([]);
	});

	it('never hands a failed pick to the next person who signs in', async () => {
		const s = server();
		const v = createViews(s.client, () => memory().store);
		v.take({});
		s.fail(true);
		const go = s.slow();
		v.set('library.view.movie', 'posters');
		v.forget();
		go();
		await settle();
		s.fail(false);
		v.take({});
		expect(v.get('library.view.movie')).toBeUndefined();
		v.set('calendar.view', 'list');
		await settle();
		expect(s.puts).toEqual([{ 'calendar.view': 'list' }]);
	});
});
