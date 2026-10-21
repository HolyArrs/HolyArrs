import { describe, expect, it } from 'vitest';
import { cardBadges, exploreLinks, eyebrow, heroNote, rowNote, type NewMusicCard } from './music-new.ts';

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
		expect(eyebrow(card({ badge: 'New EP' }), now)).toBe('New EP');
		expect(eyebrow(card({ date: '2026-10-23' }), now)).toMatch(/^Out /);
	});
	it('says when on row lines', () => {
		expect(rowNote({ type: 'np_music_tracks' }, card({ date: '2026-10-19' }), now)).toBe('Ann · 2 days ago');
		expect(rowNote({ type: 'np_music_climbing' }, card({ date: '2026-10-19' }), now)).toBe('Ann');
		expect(rowNote({ type: 'np_music_coming' }, card({ date: '2026-11-07' }), now)).toMatch(/^Ann · Out /);
	});
	it('marks explicit albums with E', () => {
		expect(cardBadges(card({ explicit: true }))).toEqual(['E']);
		expect(cardBadges(card({}))).toEqual([]);
	});
	it('links Explore more into Search → Music', () => {
		const l = exploreLinks(['indie rock']);
		expect(l[0]).toMatchObject({ label: 'indie rock', group: 'genre' });
		expect(l[0].href).toContain('/search?kind=music&mgenre=indie%20rock');
		expect(l.some((x) => x.group === 'decade')).toBe(true);
		expect(l.some((x) => x.group === 'mood')).toBe(true);
	});
});
