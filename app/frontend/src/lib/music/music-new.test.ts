import { cardBadges, eyebrow, heroNote, rowNote, type NewMusicCard } from './music-new.ts';
import { describe, expect, it } from 'vitest';

const now = new Date(2026, 9, 21, 12);
const card = (x: Partial<NewMusicCard>): NewMusicCard => ({ key: 'album:a', kind: 'album', mbid: 'a', name: 'A', artist: 'Ann', genres: [], ...x });

describe('music new', () => {
	it('writes the hero note from data only', () => {
		expect(heroNote(card({ date: '2026-10-21' }), now)).toBe('New album, out today');
		expect(heroNote(card({ date: '2026-10-18', badge: 'New EP' }), now)).toBe('New EP, out this week');
		expect(heroNote(card({ date: '2026-10-20', title_id: 3 }), now)).toBe('From an artist in your Library, out yesterday');
		expect(heroNote(card({ rank: 4 }), now)).toBe("#4 on this week's album chart");
	});
	it('names the eyebrow', () => {
		expect(eyebrow(card({ badge: 'New EP', date: '2026-10-01' }), now)).toBe('New EP');
		expect(eyebrow(card({ date: '2026-10-23' }), now)).toMatch(/^Out /);
	});
	it('never says New for an old release (a 5,000-day-old album charting now)', () => {
		const old = '2013-02-23';
		expect(eyebrow(card({ date: old }), now)).toBe('Popular now');
		expect(eyebrow(card({ date: old, badge: 'New album' }), now)).toBe('Popular now');
		expect(eyebrow(card({ date: old, kicker: '#12 worldwide' }), now)).toBe('#12 worldwide');
		expect(eyebrow(card({ date: old, kicker: 'New album' }), now)).toBe('Popular now');
		expect(eyebrow(card({ date: '2026-08-21' }), now)).toBe('Popular now');
		expect(eyebrow(card({ date: '2026-08-23' }), now)).toBe('New album');
		expect(eyebrow(card({ badge: 'New album' }), now)).toBe('Popular now');
		expect(heroNote(card({ date: old }), now)).toBe('2013 album');
		expect(heroNote(card({ date: '2026-06-01' }), now)).toMatch(/^Released /);
		expect(heroNote(card({ date: old, rank: 7 }), now)).toBe("#7 on this week's album chart · 2013 album");
		expect(rowNote({ type: 'np_music_tracks' }, card({ date: old }), now)).toBe('Ann · 2013');
	});
	it('says when on row lines', () => {
		expect(rowNote({ type: 'np_music_tracks' }, card({ date: '2026-10-19' }), now)).toBe('Ann · 2 days ago');
		expect(rowNote({ type: 'np_music_climbing' }, card({ date: '2026-10-19' }), now)).toBe('Ann');
		expect(rowNote({ type: 'np_music_climbing' }, card({ date: '2026-10-19', note: 'Up 4 · #9' }), now)).toBe('Ann · Up 4 · #9');
		expect(rowNote({ type: 'np_music_coming' }, card({ date: '2026-11-07' }), now)).toMatch(/^Ann · Out /);
	});
	it('marks explicit albums with E', () => {
		expect(cardBadges(card({ explicit: true }))).toEqual(['E']);
		expect(cardBadges(card({}))).toEqual([]);
	});
});
