import { describe, expect, it } from 'vitest';
import { cardBadges, freshLabel, metaLine, pickFresh, ROW_SUPPRESS, shownFresh, typeWord, type Fresh } from './fresh.ts';

const f = (kind: Fresh['kind'], text = 'x', rank?: number): Fresh => ({ kind, text, rank });

describe('freshness strip', () => {
	it('shows what the server picked', () => {
		expect(shownFresh(f('new_episode', 'New Episode'))?.text).toBe('New Episode');
		expect(shownFresh(null)).toBeNull();
		expect(shownFresh({ kind: 'nope' as Fresh['kind'], text: 'x' })).toBeNull();
	});
	it('music keeps Recently Added and New Release only', () => {
		expect(shownFresh(f('recently_added'), { music: true })).not.toBeNull();
		expect(shownFresh(f('new_release'), { music: true })).not.toBeNull();
		for (const k of ['new_episode', 'new_season', 'top10', 'coming', 'season_coming'] as const) expect(shownFresh(f(k), { music: true })).toBeNull();
	});
	it('the Top 10 row drops its own Top 10 tag', () => {
		expect(shownFresh(f('top10', 'Trending', 2), { top10Row: true })).toBeNull();
		expect(shownFresh(f('recently_added'), { top10Row: true })).not.toBeNull();
	});
	it('labels the rank', () => {
		expect(freshLabel(f('top10', 'Trending', 3))).toBe('Number 3 in Trending now');
		expect(freshLabel(f('coming', 'Coming Friday'))).toBe('Coming Friday');
	});
	it('priority order', () => {
		expect(pickFresh([f('new_release'), f('top10'), f('new_season')])?.kind).toBe('new_season');
		expect(pickFresh([f('season_coming'), f('coming')])?.kind).toBe('coming');
		expect(pickFresh([null, undefined])).toBeNull();
	});
	it('at most one freshness badge and one status circle, no type badge', () => {
		const b = cardBadges({ fresh: f('new_season'), owned: { state: 'available' } });
		expect(Object.keys(b)).toEqual(['fresh', 'owned']);
		expect(Object.values(b).filter(Boolean)).toHaveLength(2);
		expect(cardBadges({ fresh: f('coming'), music: true }).fresh).toBeNull();
	});
	it('a row never repeats its own meaning', () => {
		expect(shownFresh(f('top10'), { row: 'top10' })).toBeNull();
		expect(shownFresh(f('new_release'), { row: 'best' })).toBeNull();
		expect(shownFresh(f('new_release'), { row: 'fresh' })).toBeNull();
		expect(shownFresh(f('recently_added'), { row: 'just_arrived' })).toBeNull();
		expect(shownFresh(f('coming'), { row: 'coming_soon' })).toBeNull();
		expect(shownFresh(f('season_coming'), { row: 'coming_soon' })).toBeNull();
		expect(shownFresh(f('new_episode'), { row: 'new_episodes' })).toBeNull();
		// The Top 10 rank is shown only on the Netflix Top 10 rows.
		expect(shownFresh(f('top10'), { row: 'best' })).toBeNull();
		expect(shownFresh(f('new_release'), { row: 'trending' })?.kind).toBe('new_release');
		for (const [row, kinds] of Object.entries(ROW_SUPPRESS)) for (const k of kinds) expect(shownFresh(f(k), { row })).toBeNull();
	});
	it('puts the type first on the metadata line', () => {
		expect(metaLine('show', ['4 Seasons', 2024])).toEqual(['Show', '4 Seasons', '2024']);
		expect(metaLine('movie', ['2h 10m', 2024, null])).toEqual(['Movie', '2h 10m', '2024']);
		expect(metaLine('album', ['2024'])).toEqual(['2024']);
		expect(metaLine('show', ['Show', '2024'])).toEqual(['Show', '2024']);
		expect(typeWord('artist')).toBe('');
	});
});
