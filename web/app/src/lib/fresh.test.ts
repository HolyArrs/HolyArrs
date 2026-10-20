import { describe, expect, it } from 'vitest';
import { cardBadges, freshLabel, pickFresh, shownFresh, type Fresh } from './fresh.ts';

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
		expect(shownFresh(f('top10', 'Top 10', 2), { top10Row: true })).toBeNull();
		expect(shownFresh(f('recently_added'), { top10Row: true })).not.toBeNull();
	});
	it('labels the rank', () => {
		expect(freshLabel(f('top10', 'Top 10', 3))).toBe("Number 3 in this week's Top 10");
		expect(freshLabel(f('coming', 'Coming Friday'))).toBe('Coming Friday');
	});
	it('priority order', () => {
		expect(pickFresh([f('new_release'), f('top10'), f('new_season')])?.kind).toBe('new_season');
		expect(pickFresh([f('season_coming'), f('coming')])?.kind).toBe('coming');
		expect(pickFresh([null, undefined])).toBeNull();
	});
	it('at most one strip and one status circle', () => {
		const b = cardBadges({ fresh: f('top10'), owned: { state: 'available' }, typePill: 'movie' });
		expect(Object.values(b).filter(Boolean)).toHaveLength(3);
		expect(cardBadges({ fresh: f('coming'), music: true }).fresh).toBeNull();
	});
});
