import { describe, expect, it } from 'vitest';
import { ApiError } from './api.ts';
import {
	enoughCards,
	genreCardHref,
	genreRowAfter,
	discoverTitles,
	becauseYouHave,
	cardAction,
	cardTmdb,
	comingSoon,
	orderRecent,
	discoverCard,
	discoverPath,
	fetchProviderRow,
	fromPerson,
	genreRows,
	genresOf,
	keepFull,
	kindShelf,
	liveCard,
	readyToWatch,
	stillLooking,
	markTransitions,
	tmdbArtName,
	missingEpisodes,
	overlap,
	popularInHousehold,
	quality,
	recentlyAdded,
	score,
	cardPill,
	titleCard,
	type Facets,
	pickerServices,
	toggleService,
	readServices,
	writeServices,
	serviceRegion,
} from './explore.ts';
import type { Ask, CalendarEntry, SlotView, TitleView } from './types.ts';

const slot = (state: string, resolution?: number): SlotView =>
	({ id: 1, version: 'Main', version_id: 1, state, state_label: state, file: resolution ? { RelativePath: 'x', Size: 1, AddedAt: '', Quality: { resolution } } : null }) as SlotView;

const t = (id: number, extra: Partial<TitleView> = {}): TitleView =>
	({ id, kind: 'movie', name: `Title ${id}`, year: 2000 + id, ids: null, keep_up_to_date: true, why_added: 'by_hand', release_gate: 'released', gate_delay_days: 0, stage: 'released', gate_open: true, gate_reason: '', artwork: [`/a/${id}.jpg`], versions: [], slots: [], asks: [], ...extra }) as TitleView;

const ask = (id: number, extra: Partial<Ask> = {}): Ask =>
	({ id, kind: 'movie', tmdb_id: String(900 + id), ref: `tmdb:${900 + id}`, name: `Wish ${id}`, state: 'asked', state_label: 'Pending', asked_by: { id: 1, name: 'A' }, asked_at: `2026-10-0${id}T00:00:00Z`, people: [{ id: 1, name: 'A', asked_at: '' }], auto_approved: false, mine: false, ...extra }) as Ask;

const f = (genres: string[], people: string[] = []): Facets => ({ genres, people });

describe('explore cards', () => {
	it('reads the best file quality', () => {
		expect(quality(t(1, { slots: [slot('in_library', 1080), slot('in_library', 2160)] }))).toBe('4K');
		expect(quality(t(1, { slots: [slot('in_library', 720)] }))).toBe('720p');
		expect(quality(t(1))).toBeUndefined();
	});

	it('picks one overlay with the shared status words: downloading, then in the Library, then asked', () => {
		const pill = (x: TitleView, who: 'member' | 'staff') => cardPill(titleCard(x), who);
		expect(pill(t(1, { slots: [slot('downloading'), slot('in_library')] }), 'staff')?.kind).toBe('downloading');
		expect(pill(t(1, { asks: [ask(1)] }), 'member')).toEqual({ kind: 'requested', label: 'Requested' });
		expect(pill(t(1, { asks: [ask(1)] }), 'staff')).toEqual({ kind: 'requested', label: 'Waiting' });
		expect(pill(t(1, { asks: [ask(1)], slots: [slot('in_library')] }), 'member')).toEqual({ kind: 'in_library', label: 'Ready to watch' });
		expect(pill(t(1, { slots: [slot('in_library')] }), 'staff')).toEqual({ kind: 'in_library', label: 'In your Library' });
	});

	it('keeps a card label set on purpose, and shows nothing without a status', () => {
		expect(cardPill({ key: 'k', kind: 'movie', name: 'K', badges: [], status: 'missing', statusLabel: 'Not out yet' }, 'member')).toEqual({ kind: 'missing', label: 'Not out yet' });
		expect(cardPill({ key: 'k', kind: 'movie', name: 'K', badges: [], status: 'requested' }, 'member')).toEqual({ kind: 'requested', label: 'Requested' });
		expect(cardPill({ key: 'k', kind: 'movie', name: 'K', badges: [] }, 'member')).toBeNull();
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
		expect(cards[0].subtitle).toBe('S2 · E1');
		expect(cards[0].badges[1]).toBe('New');
		expect(cards[1].subtitle).toBe('On digital');
		expect(cards.every((c) => c.status === 'coming_soon')).toBe(true);
	});
	it('still shows titles it has no Library record for', () => {
		const [c] = comingSoon([e(7, '2026-10-21T00:00:00Z', { kind: 'album', album: 'Blue' })], [], now);
		expect(c).toMatchObject({ kind: 'artist', name: 'T7', subtitle: 'Blue' });
	});
});

describe('coming soon wide cards', () => {
	const now = new Date(2026, 9, 18, 12);
	it('picks the backdrop, then the title backdrop, then the still, else the name tile', () => {
		const cards = comingSoon(
			[
				{ title_id: 1, name: 'A', kind: 'episode', date: '2026-10-20T00:00:00Z', season: 1, episode: 2, still: 's.jpg', backdrop: 'b.jpg' },
				{ title_id: 2, name: 'B', kind: 'digital', date: '2026-10-21T00:00:00Z', poster: 'p2.jpg' },
				{ title_id: 3, name: 'C', kind: 'episode', date: '2026-10-22T00:00:00Z', season: 3, episode: 4, still: 's3.jpg' },
				{ title_id: 4, name: 'D', kind: 'physical', date: '2026-10-23T00:00:00Z' }
			],
			[t(1, { kind: 'show', artwork: ['own.jpg'] }), t(2, { backdrop: 'tb.jpg' })],
			now
		);
		expect(cards.map((c) => c.wide)).toEqual(['b.jpg', 'tb.jpg', 's3.jpg', null]);
		expect(cards.map((c) => c.image ?? null)).toEqual(['own.jpg', '/a/2.jpg', null, null]);
	});
	it('writes line 2 per kind', () => {
		const lines = comingSoon(
			[
				{ title_id: 1, name: 'A', kind: 'episode', date: '2026-10-20T00:00:00Z', season: 1, episode: 3, episode_name: 'Pilot two' },
				{ title_id: 2, name: 'B', kind: 'digital', date: '2026-10-21T00:00:00Z' },
				{ title_id: 3, name: 'C', kind: 'physical', date: '2026-10-21T00:00:00Z' },
				{ title_id: 4, name: 'D', kind: 'cinema', date: '2026-10-21T00:00:00Z' },
				{ title_id: 5, name: 'E', kind: 'album', date: '2026-10-21T00:00:00Z', album: 'Blue' }
			],
			[],
			now
		).map((c) => c.subtitle);
		expect(lines).toEqual(['S1 · E3 · Pilot two', 'On digital', 'On disc', 'In cinemas', 'Blue']);
	});
	it('badges the day and premieres', () => {
		const b = comingSoon(
			[
				{ title_id: 1, name: 'A', kind: 'episode', date: '2026-10-18T20:00:00', season: 1, episode: 1, timed: true },
				{ title_id: 2, name: 'B', kind: 'episode', date: '2026-10-19T20:00:00', season: 4, episode: 1, timed: true },
				{ title_id: 3, name: 'C', kind: 'episode', date: '2026-10-21T20:00:00', season: 2, episode: 5, timed: true },
				{ title_id: 4, name: 'D', kind: 'digital', date: '2026-11-20T12:00:00' }
			],
			[],
			now
		).map((c) => c.badges);
		expect(b[0]).toEqual(['Today', 'Premiere']);
		expect(b[1]).toEqual(['Tomorrow', 'New']);
		expect(b[2]).toEqual([new Date(2026, 9, 21).toLocaleDateString(undefined, { weekday: 'long' })]);
		expect(b[3]).toEqual([new Date(2026, 10, 20).toLocaleDateString(undefined, { day: 'numeric', month: 'short' })]);
	});
});

describe('orderRecent', () => {
	const c = (key: string, kind: 'movie' | 'show' | 'artist', image: string | null) => ({ key, kind, name: key, image, badges: [] });
	it('puts cards with artwork first and keeps the order otherwise', () => {
		expect(orderRecent([c('a', 'movie', null), c('b', 'movie', 'b.jpg'), c('c', 'movie', 'c.jpg')]).map((x) => x.key)).toEqual(['b', 'c', 'a']);
	});
	it('takes turns between kinds', () => {
		const out = orderRecent([c('m1', 'movie', 'x'), c('m2', 'movie', 'x'), c('m3', 'movie', 'x'), c('s1', 'show', 'x'), c('a1', 'artist', 'x')]);
		expect(out.map((x) => x.key)).toEqual(['m1', 's1', 'a1', 'm2', 'm3']);
	});
	it('interleaves within the artwork group, then the rest', () => {
		const out = orderRecent([c('m1', 'movie', null), c('s1', 'show', 'x'), c('m2', 'movie', 'x'), c('a1', 'artist', null)]);
		expect(out.map((x) => x.key)).toEqual(['s1', 'm2', 'm1', 'a1']);
	});
	it('keeps an empty list empty', () => {
		expect(orderRecent([])).toEqual([]);
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
		expect(cards[0].ask).toEqual({ id: 2, state: 'asked', state_label: 'Pending', mine: false, people: 3 });
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
	it('names a not-added movie or show card by its TMDB id, once', () => {
		const rows = markTransitions([
			{ id: 'a', title: 'A', cards: [{ key: 'm', kind: 'movie', name: 'M', ref: 'tmdb:12', badges: [] }, { key: 'm2', kind: 'movie', name: 'M', ref: 'tmdb:12', badges: [] }] }
		]);
		expect(rows[0].cards.map((c) => c.vtName)).toEqual([tmdbArtName('movie', '12'), undefined]);
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
	it('finds the TMDB id a card can open a confirm sheet with', () => {
		const out = { key: 'k', kind: 'movie' as const, name: 'K', badges: [], ref: 'tmdb:12' };
		expect(cardTmdb(out)).toBe('12');
		expect(cardTmdb({ ...out, kind: 'show' })).toBe('12');
		expect(cardTmdb({ ...out, kind: 'artist', ref: 'mbid:x' })).toBe('');
		expect(cardTmdb({ ...out, ref: 'tvdb:4' })).toBe('');
		expect(cardTmdb({ ...out, ref: undefined })).toBe('');
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
			expect(await fetchProviderRow({ row: 'best' }, client as never)).toBeNull();
		}
	});
	it('passes other errors on and answers through', async () => {
		const bad = { get: async () => { throw new ApiError('down', 500); } };
		await expect(fetchProviderRow({ row: 'best' }, bad as never)).rejects.toThrow('down');
		const good = { get: async (p: string) => ({ row: 'best', items: [], p }) };
		expect(await fetchProviderRow({ row: 'best' }, good as never)).toMatchObject({ p: '/discover?row=best' });
	});
	it('turns items into cards', () => {
		expect(discoverCard({ tmdb_id: '5', kind: 'movie', name: 'M', in_library: true, title_id: 9 }, 'trending')).toMatchObject({ titleId: 9, status: 'in_library', ref: 'tmdb:5' });
		expect(discoverCard({ tmdb_id: '5', kind: 'show', name: 'S', in_library: false, ask: { id: 1, state: 'asked', state_label: '', mine: true, people: 1 } }, 'p')).toMatchObject({ titleId: undefined, status: 'requested', mine: true });
	});
	it('keeps the open ask on the card so asking joins it', () => {
		const a = { id: 1, state: 'asked' as const, state_label: 'Pending', mine: false, people: 2 };
		expect(discoverCard({ tmdb_id: '5', kind: 'movie', name: 'M', in_library: false, ask: a }, 'p').ask).toEqual(a);
		expect(discoverCard({ tmdb_id: '5', kind: 'movie', name: 'M', in_library: false, ask: { ...a, state: 'turned_down' } }, 'p').ask).toBeUndefined();
	});
});

describe('cards asked or refused on the page', () => {
	const card = { key: 'k', kind: 'movie' as const, name: 'K', badges: [], ref: 'tmdb:1' };
	it('reads the returned ask state, so an auto-approved ask says Approved', () => {
		expect(liveCard(card, { ask: 'approved' })).toMatchObject({ mine: true, status: 'approved', statusLabel: undefined });
		expect(cardPill(liveCard(card, { ask: 'approved' }), 'member')).toEqual({ kind: 'approved', label: 'Approved' });
		expect(cardPill(liveCard(card, { ask: 'asked' }), 'member')).toEqual({ kind: 'requested', label: 'Requested' });
	});
	it('turns a card refused as in the Library into a Library card with no action', () => {
		const c = liveCard(card, { titleId: 7 });
		expect(c).toMatchObject({ titleId: 7, status: 'in_library' });
		expect(cardAction(c, { manage: false, ask: true })).toBeNull();
	});
	it('leaves other cards alone', () => {
		expect(liveCard(card, undefined)).toBe(card);
	});
});

describe('library shelves', () => {
	const list = [
		t(1, { slots: [slot('in_library')] }),
		t(2, { slots: [slot('wanted')] }),
		t(3, { kind: 'show', slots: [slot('in_library')], show: { missing: 2 } as TitleView['show'] }),
		t(4, { kind: 'show', slots: [slot('in_library')], show: { missing: 0 } as TitleView['show'] }),
		t(5, { kind: 'artist', slots: [slot('wanted')] })
	];
	it('splits the Library by kind, newest first', () => {
		expect(kindShelf(list, 'show').map((c) => c.titleId)).toEqual([4, 3]);
		expect(kindShelf(list, 'artist').map((c) => c.titleId)).toEqual([5]);
	});
	it('ready to watch needs every copy and no missing episode', () => {
		expect(readyToWatch(list).map((c) => c.titleId)).toEqual([4, 1]);
	});
	it('still looking lists movies and artists still looking, marked missing', () => {
		const w = stillLooking(list);
		expect(w.map((c) => c.titleId)).toEqual([5, 2]);
		expect(w[0].status).toBe('missing');
	});
});

describe('on your services', () => {
	const all = [
		{ id: 8, name: 'Netflix', priority: 1 },
		{ id: 9, name: 'Prime Video', priority: 2 },
		{ id: 337, name: 'Disney Plus', priority: 3 },
		{ id: 1899, name: 'Max', priority: 4 },
		{ id: 350, name: 'Apple TV+', priority: 5 },
		{ id: 15, name: 'Hulu', priority: 6 },
		{ id: 99, name: 'Small', priority: 40 }
	];
	it('offers the top six, plus a picked one that fell out', () => {
		expect(pickerServices(all, []).map((s) => s.id)).toEqual([8, 9, 337, 1899, 350, 15]);
		expect(pickerServices(all, [99]).map((s) => s.id)).toEqual([8, 9, 337, 1899, 350, 15, 99]);
	});
	it('toggles and remembers per region', () => {
		expect(toggleService([8], 9)).toEqual([8, 9]);
		expect(toggleService([8, 9], 8)).toEqual([9]);
		expect(readServices('US', null)).toBeNull();
		expect(readServices('US', 'nope')).toBeNull();
		const raw = writeServices('US', [8, 15], writeServices('GB', [9], null));
		expect(readServices('US', raw)).toEqual([8, 15]);
		expect(readServices('GB', raw)).toEqual([9]);
		expect(readServices('DE', raw)).toBeNull();
	});
	it('uses the country, else US', () => {
		expect(serviceRegion('GB')).toBe('GB');
		expect(serviceRegion('global')).toBe('US');
	});
	it('asks the server with services and region', () => {
		expect(discoverPath({ row: 'services', services: [8, 337], region: 'US', kind: 'show' })).toBe('/discover?row=services&kind=show&services=8%7C337&region=US');
		expect(discoverTitles.best).toBe('Best new releases');
		expect('popular' in discoverTitles).toBe(false);
	});
});

describe('row sizes and Browse by genre', () => {
	it('drops rows under 8 cards but keeps kept rows', () => {
		const row = (n: number, keep = false) => ({ cards: Array.from({ length: n }), keep });
		expect(enoughCards([row(7), row(8), row(0, true)]).map((r) => r.cards.length)).toEqual([8, 0]);
	});
	it('goes after row 6, never next to a genre row', () => {
		const rows = (types: string[]) => types.map((type) => ({ type }));
		expect(genreRowAfter(rows(['a', 'b', 'c', 'd', 'e', 'f', 'g']))).toBe(5);
		expect(genreRowAfter(rows(['a', 'b', 'c', 'd', 'e', 'f', 'genre', 'h', 'i']))).toBe(7);
		expect(genreRowAfter(rows(['a', 'b', 'c']))).toBe(-1);
	});
	it('opens the genre page of its kind', () => {
		expect(genreCardHref({ kind: 'show', id: 18 })).toBe('/explore?type=shows&genre=18');
		expect(genreCardHref({ kind: 'movie', id: 28, both: true })).toBe('/explore?genre=28');
	});
});
