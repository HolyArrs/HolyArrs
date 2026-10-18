import { describe, expect, it } from 'vitest';
import { ApiError } from './api.ts';
import {
	becauseYouHave,
	cardAction,
	comingSoon,
	discoverCard,
	discoverPath,
	fetchProviderRow,
	fromPerson,
	genreRows,
	genresOf,
	keepFull,
	markTransitions,
	missingEpisodes,
	overlap,
	popularInHousehold,
	quality,
	recentlyAdded,
	score,
	titleStatus,
	type Facets
} from './explore.ts';
import type { Ask, CalendarEntry, SlotView, TitleView } from './types.ts';

const slot = (state: string, resolution?: number): SlotView =>
	({ id: 1, version: 'Main', version_id: 1, state, state_label: state, file: resolution ? { RelativePath: 'x', Size: 1, AddedAt: '', Quality: { resolution } } : null }) as SlotView;

const t = (id: number, extra: Partial<TitleView> = {}): TitleView =>
	({ id, kind: 'movie', name: `Title ${id}`, year: 2000 + id, ids: null, keep_up_to_date: true, why_added: 'by_hand', release_gate: 'released', gate_delay_days: 0, stage: 'released', gate_open: true, gate_reason: '', artwork: [`/a/${id}.jpg`], versions: [], slots: [], asks: [], ...extra }) as TitleView;

const ask = (id: number, extra: Partial<Ask> = {}): Ask =>
	({ id, kind: 'movie', tmdb_id: String(900 + id), ref: `tmdb:${900 + id}`, name: `Wish ${id}`, state: 'asked', state_label: 'Waiting for approval', asked_by: { id: 1, name: 'A' }, asked_at: `2026-10-0${id}T00:00:00Z`, people: [{ id: 1, name: 'A', asked_at: '' }], auto_approved: false, mine: false, ...extra }) as Ask;

const f = (genres: string[], people: string[] = []): Facets => ({ genres, people });

describe('explore cards', () => {
	it('reads the best file quality', () => {
		expect(quality(t(1, { slots: [slot('in_library', 1080), slot('in_library', 2160)] }))).toBe('4K');
		expect(quality(t(1, { slots: [slot('in_library', 720)] }))).toBe('720p');
		expect(quality(t(1))).toBeUndefined();
	});

	it('picks one overlay: downloading, then requested, then in the Library', () => {
		expect(titleStatus(t(1, { slots: [slot('downloading'), slot('in_library')] })).status).toBe('downloading');
		expect(titleStatus(t(1, { asks: [ask(1)] })).status).toBe('requested');
		expect(titleStatus(t(1, { asks: [ask(1)], slots: [slot('in_library')] })).status).toBe('in_library');
		expect(titleStatus(t(1)).label).toBe('In Library ✓');
	});

	it('sorts recently added by id, newest first', () => {
		expect(recentlyAdded([t(3), t(9), t(5)], 2).map((x) => x.id)).toEqual([9, 5]);
	});
});

describe('coming soon', () => {
	const now = new Date(2026, 9, 18, 12);
	const e = (title_id: number, date: string, extra: Partial<CalendarEntry> = {}): CalendarEntry => ({ title_id, name: `T${title_id}`, kind: 'digital', date, ...extra });
	it('keeps each title once, soonest first, and drops the past', () => {
		const cards = comingSoon(
			[e(1, '2026-10-25T00:00:00Z'), e(2, '2026-10-19T00:00:00Z', { kind: 'episode', season: 2, episode: 1, timed: true }), e(1, '2026-10-20T00:00:00Z'), e(3, '2026-09-01T00:00:00Z')],
			[t(1), t(2, { kind: 'show' })],
			now
		);
		expect(cards.map((c) => c.titleId)).toEqual([2, 1]);
		expect(cards[0].subtitle).toMatch(/^S02E01 · /);
		expect(cards[1].subtitle).toMatch(/^On digital · /);
		expect(cards.every((c) => c.status === 'coming_soon')).toBe(true);
	});
	it('still shows titles it has no Library record for', () => {
		const [c] = comingSoon([e(7, '2026-10-21T00:00:00Z', { kind: 'album', album: 'Blue' })], [], now);
		expect(c).toMatchObject({ kind: 'artist', name: 'T7', subtitle: expect.stringMatching(/^Blue · /) });
	});
});

describe('missing episodes', () => {
	it('lists shows by how many episodes are missing', () => {
		const show = (id: number, missing: number) => t(id, { kind: 'show', show: { missing } as TitleView['show'] });
		const cards = missingEpisodes([show(1, 2), show(2, 0), show(3, 8), show(4, 1)]);
		expect(cards.map((c) => c.titleId)).toEqual([3, 1, 4]);
		expect(cards[2].subtitle).toBe('1 episode missing');
		expect(cards[0].status).toBe('missing');
	});
});

describe('popular in the household', () => {
	it('ranks by people who asked, joins asks for the same thing, skips turned down', () => {
		const cards = popularInHousehold(
			[
				ask(1, { people: [{ id: 1, name: 'A', asked_at: '' }] }),
				ask(2, { people: [{ id: 1, name: 'A', asked_at: '' }, { id: 2, name: 'B', asked_at: '' }, { id: 3, name: 'C', asked_at: '' }] }),
				ask(3, { ref: 'tmdb:901', tmdb_id: '901', people: [{ id: 4, name: 'D', asked_at: '' }] }),
				ask(4, { state: 'turned_down', people: [{ id: 9, name: 'Z', asked_at: '' }, { id: 8, name: 'Y', asked_at: '' }, { id: 7, name: 'X', asked_at: '' }, { id: 6, name: 'W', asked_at: '' }] })
			],
			[]
		);
		expect(cards.map((c) => c.ref)).toEqual(['tmdb:902', 'tmdb:901']);
		expect(cards[0].subtitle).toBe('3 people asked');
		expect(cards[1].subtitle).toBe('2 people asked');
		expect(cards[0].status).toBe('requested');
	});
	it('uses the Library title when the ask led to one', () => {
		const [c] = popularInHousehold([ask(1, { title_id: 5, mine: true })], [t(5, { slots: [slot('in_library')] })]);
		expect(c).toMatchObject({ titleId: 5, status: 'in_library', mine: true, subtitle: '1 person asked' });
	});
});

describe('ranking', () => {
	it('counts a shared person double a shared genre', () => {
		expect(overlap(f(['Drama', 'Crime'], ['Ann']), f(['Crime'], ['Ann', 'Bo']))).toBe(3);
		expect(overlap(f(['Drama']), undefined)).toBe(0);
	});
	it('scores overlap × recency × asks', () => {
		expect(score({ overlap: 2, ageRank: 0, total: 3, asks: 0 })).toBe(2);
		expect(score({ overlap: 2, ageRank: 2, total: 3, asks: 0 })).toBe(1);
		expect(score({ overlap: 2, ageRank: 0, total: 3, asks: 4 })).toBe(4);
		expect(score({ overlap: 0, ageRank: 0, total: 3, asks: 9 })).toBe(0);
	});
	it('builds Because you have from the newest title with enough related titles', () => {
		const titles = [t(1), t(2), t(3), t(4), t(5)];
		const facets = new Map([
			[5, f(['Horror'])],
			[4, f(['Drama'], ['Ann'])],
			[3, f(['Drama'])],
			[2, f(['Drama'], ['Ann'])],
			[1, f(['Drama'])]
		]);
		const r = becauseYouHave(titles, facets, 3)!;
		expect(r.seed.id).toBe(4);
		expect(r.cards.map((c) => c.titleId)).toEqual([2, 3, 1]);
		expect(becauseYouHave(titles, new Map())).toBeNull();
	});
	it('does not mix kinds', () => {
		const titles = [t(1, { kind: 'show' }), t(2), t(3)];
		const facets = new Map([[3, f(['A'])], [2, f(['A'])], [1, f(['A'])]]);
		expect(becauseYouHave(titles, facets, 2)).toBeNull();
	});
	it('makes genre rows for the biggest genres only', () => {
		const titles = [1, 2, 3, 4, 5].map((i) => t(i));
		const facets = new Map(titles.map((x) => [x.id, f(x.id <= 4 ? ['Comedy', 'Family'] : ['Comedy'])]));
		const rows = genreRows(titles, facets, 1, 4);
		expect(rows).toHaveLength(1);
		expect(rows[0]).toMatchObject({ id: 'genre-comedy', title: 'Comedy in your library' });
		expect(rows[0].cards).toHaveLength(5);
		expect(genreRows(titles, facets, 3, 6)).toEqual([]);
	});
	it('picks the person in the most titles', () => {
		const titles = [t(1), t(2), t(3)];
		const facets = new Map([[1, f([], ['Ann', 'Bo'])], [2, f([], ['Bo'])], [3, f([], ['Bo', 'Ann', 'Ann'])]]);
		expect(fromPerson(titles, facets)).toMatchObject({ person: 'Bo' });
		expect(fromPerson(titles, facets)!.cards).toHaveLength(3);
		expect(fromPerson([t(1)], new Map([[1, f([], ['Ann'])]]))).toBeNull();
	});
	it('reads genres only when the API sends them', () => {
		expect(genresOf(t(1))).toEqual([]);
		expect(genresOf({ ...t(1), genres: ['Drama', 3] } as unknown as TitleView)).toEqual(['Drama']);
	});
});

describe('rows on a page', () => {
	it('names only the first card of each title for the view transition', () => {
		const rows = markTransitions([
			{ id: 'a', title: 'A', cards: [{ key: 'x', kind: 'movie', name: 'X', titleId: 1, badges: [] }] },
			{ id: 'b', title: 'B', cards: [{ key: 'y', kind: 'movie', name: 'X', titleId: 1, badges: [] }, { key: 'z', kind: 'movie', name: 'Z', badges: [] }] }
		]);
		expect(rows[0].cards[0].vtName).toBe('title-art-1');
		expect(rows[1].cards.map((c) => c.vtName)).toEqual([undefined, undefined]);
	});
	it('drops empty rows', () => {
		expect(keepFull([null, { id: 'a', title: 'A', cards: [] }, { id: 'b', title: 'B', cards: [{ key: 'k', kind: 'movie', name: 'K', badges: [] }] }]).map((r) => r.id)).toEqual(['b']);
	});
	it('offers Add to managers and Ask to members, never on Library titles or own asks', () => {
		const out = { key: 'k', kind: 'movie' as const, name: 'K', badges: [], ref: 'tmdb:1' };
		expect(cardAction(out, { manage: true, ask: true })).toBe('add');
		expect(cardAction(out, { manage: false, ask: true })).toBe('ask');
		expect(cardAction({ ...out, mine: true }, { manage: false, ask: true })).toBeNull();
		expect(cardAction({ ...out, titleId: 3 }, { manage: true, ask: true })).toBeNull();
		expect(cardAction(out, { manage: false, ask: false })).toBeNull();
	});
});

describe('discover', () => {
	it('writes the query', () => {
		expect(discoverPath({ row: 'trending' })).toBe('/discover?row=trending');
		expect(discoverPath({ row: 'similar', id: 12 })).toBe('/discover?row=similar&id=12');
		expect(discoverPath({ row: 'genre', g: 'Sci-Fi & Fantasy', kind: 'show' })).toBe('/discover?row=genre&g=Sci-Fi+%26+Fantasy&kind=show');
	});
	it('returns null when the server has no discover endpoint', async () => {
		for (const status of [404, 501]) {
			const client = { get: async () => { throw new ApiError('nope', status); } };
			expect(await fetchProviderRow({ row: 'popular' }, client as never)).toBeNull();
		}
	});
	it('passes other errors on and answers through', async () => {
		const bad = { get: async () => { throw new ApiError('down', 500); } };
		await expect(fetchProviderRow({ row: 'popular' }, bad as never)).rejects.toThrow('down');
		const good = { get: async (p: string) => ({ row: 'popular', items: [], p }) };
		expect(await fetchProviderRow({ row: 'popular' }, good as never)).toMatchObject({ p: '/discover?row=popular' });
	});
	it('turns items into cards', () => {
		expect(discoverCard({ tmdb_id: '5', kind: 'movie', name: 'M', in_library: true, title_id: 9 }, 'trending')).toMatchObject({ titleId: 9, status: 'in_library', ref: 'tmdb:5' });
		expect(discoverCard({ tmdb_id: '5', kind: 'show', name: 'S', in_library: false, ask: { id: 1, state: 'asked', state_label: '', mine: true, people: 1 } }, 'p')).toMatchObject({ titleId: undefined, status: 'requested', mine: true });
	});
});
