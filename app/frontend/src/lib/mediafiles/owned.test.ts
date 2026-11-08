import { describe, expect, it } from 'vitest';
import { createClient } from '../core/api.ts';
import { ownedIndex, ownedMark, ownedTitle, type ExploreCard } from '#lib/ui/title-cards.ts';
import { libraryBrief, libraryTitles, ownedLibrary, stubTitle } from './owned.ts';

const reply = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
const card = (ref: string, kind: 'movie' | 'show' | 'artist' = 'movie') => ({ key: ref, kind, ref, name: 'x' }) as unknown as ExploreCard;
const markFor = (state: string, percent?: number) => {
	const idx = ownedIndex([stubTitle({ kind: 'movie', id: 7, tmdb: 99, state, percent })]);
	const c = card('tmdb:99');
	return ownedMark(c, ownedTitle(c, idx), 'staff');
};

describe('stubTitle', () => {
	it('gives the same marks the full title would', () => {
		expect(markFor('available')?.state).toBe('available');
		expect(markFor('partial')?.state).toBe('partial');
		expect(markFor('downloading', 42)).toEqual({ state: 'downloading', label: 'Downloading 42%' });
		expect(markFor('requested')?.state).toBe('requested');
		expect(markFor('approved')?.label).toBe('Approved, waiting for a copy');
		expect(markFor('declined')?.state).toBe('declined');
		expect(markFor('missing')?.label).toBe('Waiting for a copy');
	});
	it('finds artists by MusicBrainz id and keeps the Library id for the link', () => {
		const idx = ownedIndex([stubTitle({ kind: 'artist', id: 12, mbid: 'abc', state: 'available' })]);
		expect(ownedTitle(card('mbid:abc', 'artist'), idx)?.id).toBe(12);
	});
});

describe('ownedLibrary', () => {
	it('uses the light answer when the server has it', async () => {
		const asked: string[] = [];
		const c = createClient(async (u) => (asked.push(u), reply(200, { items: [{ kind: 'show', id: 3, tmdb: '5', state: 'available' }] })), () => undefined);
		const r = await ownedLibrary(c);
		expect(r.light).toBe(true);
		expect(r.titles[0]).toMatchObject({ id: 3, kind: 'show', ids: { tmdb: '5' } });
		expect(asked).toEqual(['/api/v1/titles/owned']);
	});
	it('falls back to the full list on a 404 (an older server)', async () => {
		const asked: string[] = [];
		const c = createClient(async (u) => (asked.push(u), u.endsWith('/titles/owned') ? reply(404, { error: 'no' }) : reply(200, { titles: [{ id: 1, kind: 'movie' }] })), () => undefined);
		const r = await ownedLibrary(c);
		expect(r).toEqual({ titles: [{ id: 1, kind: 'movie' }], light: false });
		expect(asked).toEqual(['/api/v1/titles/owned', '/api/v1/titles']);
	});
	it('shares one full-list read between pages, and forgets it after a change', async () => {
		let n = 0;
		const c = createClient(async (u) => (u.endsWith('/titles') && n++, reply(200, { titles: [] })), () => undefined);
		await libraryTitles(c);
		await libraryTitles(c);
		expect(n).toBe(1);
		await c.post('/x');
		await libraryTitles(c);
		expect(n).toBe(2);
	});
	it('asks for the brief list once for Downloads, History and the Calendar', async () => {
		const asked: string[] = [];
		const c = createClient(async (u) => (asked.push(u), reply(200, { titles: [{ id: 1, kind: 'movie', name: 'A', artwork: [], slots: [] }] })), () => undefined);
		expect(await libraryBrief(c)).toEqual([{ id: 1, kind: 'movie', name: 'A', artwork: [], slots: [] }]);
		await libraryBrief(c);
		expect(asked).toEqual(['/api/v1/titles?brief=1']);
	});
});
