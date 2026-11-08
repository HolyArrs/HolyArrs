import { discoverCard, matchOwned, ownedIndex, ownedMark, ownedTitle, type ExploreCard } from '#lib/ui/title-cards.ts';
import { describe, expect, it } from 'vitest';
import type { TitleView } from '../core/types.ts';

const title = (o: Partial<TitleView>): TitleView => ({ id: 1, kind: 'movie', name: 'M', ids: { tmdb: '10' }, slots: [], asks: [], ...o }) as unknown as TitleView;
const slot = (state: string, file = false) => ({ state, file: file ? { ID: 1 } : null });
const card = (o: Partial<ExploreCard> = {}): ExploreCard => ({ key: 'k', kind: 'movie', name: 'M', ref: 'tmdb:10', badges: [], ...o });

describe('owned matching', () => {
	const movie = title({ id: 7, slots: [slot('in_library', true)] } as Partial<TitleView>);
	const show = title({ id: 8, kind: 'show', ids: { tmdb: '10' } });
	const artist = title({ id: 9, kind: 'artist', ids: {}, artist: { mbid: 'ab-1' } } as Partial<TitleView>);
	const idx = ownedIndex([movie, show, artist]);

	it('finds a discover card by kind and TMDB id', () => {
		expect(ownedTitle(card(), idx)?.id).toBe(7);
		expect(ownedTitle(card({ kind: 'show' }), idx)?.id).toBe(8);
		expect(ownedTitle(card({ ref: 'tmdb:11' }), idx)).toBeUndefined();
	});
	it('finds an artist by MusicBrainz id, and a card by title id', () => {
		expect(ownedTitle(card({ kind: 'artist', ref: 'mbid:ab-1' }), idx)?.id).toBe(9);
		expect(ownedTitle(card({ ref: undefined, titleId: 8 }), idx)?.id).toBe(8);
	});
	it('matchOwned sets the title id so the card opens the title page and has no +', () => {
		const d = discoverCard({ tmdb_id: '10', kind: 'movie', name: 'M', in_library: false }, 'trending');
		expect(matchOwned(d, idx)).toMatchObject({ titleId: 7, status: 'in_library' });
		const other = card({ ref: 'tmdb:99' });
		expect(matchOwned(other, idx)).toBe(other);
	});
});

describe('ownedMark', () => {
	it('all files: available', () => {
		expect(ownedMark(card(), title({ slots: [slot('in_library', true)] } as Partial<TitleView>), 'member')).toEqual({ state: 'available', label: 'In your library' });
	});
	it('some files or missing episodes: partial', () => {
		expect(ownedMark(card(), title({ slots: [slot('in_library', true), slot('wanted')] } as Partial<TitleView>), 'member')?.state).toBe('partial');
		expect(ownedMark(card(), title({ kind: 'show', slots: [slot('in_library', true)], show: { missing: 2 } } as Partial<TitleView>), 'member')).toEqual({ state: 'partial', label: 'Partly available' });
	});
	it('downloading wins', () => {
		expect(ownedMark(card(), title({ slots: [slot('in_library', true), slot('downloading')] } as Partial<TitleView>), 'member')?.state).toBe('downloading');
	});
	it('tracked without files, or asked: requested', () => {
		expect(ownedMark(card(), title({ slots: [slot('wanted')] } as Partial<TitleView>), 'member')?.state).toBe('requested');
		expect(ownedMark(card({ status: 'requested' }), undefined, 'member')?.state).toBe('requested');
		expect(ownedMark(card({ status: 'approved' }), undefined, 'member')?.state).toBe('requested');
	});
	it('declined only for staff', () => {
		expect(ownedMark(card({ declined: true }), undefined, 'staff')?.state).toBe('declined');
		expect(ownedMark(card({ declined: true }), undefined, 'member')).toBeNull();
		const t = title({ asks: [{ state: 'turned_down' }] } as Partial<TitleView>);
		expect(ownedMark(card(), t, 'staff')?.state).toBe('declined');
		expect(ownedMark(card(), t, 'member')).toBeNull();
	});
	it('not owned and not asked: no icon', () => {
		expect(ownedMark(card(), undefined, 'staff')).toBeNull();
	});
});
