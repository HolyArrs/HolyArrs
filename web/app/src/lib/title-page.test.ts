import { describe, expect, it } from 'vitest';
import { artName, crumbFor, heroStatus, pageSections, qualityBadges, similarTitles } from './title-page.ts';
import type { SlotView } from './types.ts';

const slot = (s: Partial<SlotView>): SlotView => ({ id: 1, version: 'Main', version_id: 1, state: 'wanted', state_label: '', ...s });

describe('title page helpers', () => {
	it('names the breadcrumb by kind', () => {
		expect(crumbFor('movie').label).toBe('Movies');
		expect(crumbFor('show').label).toBe('TV shows');
		expect(crumbFor('artist').label).toBe('Music');
	});

	it('derives badges only from file data', () => {
		expect(qualityBadges([])).toEqual([]);
		expect(qualityBadges([{ Quality: { resolution: 1080 } }])).toEqual(['HD']);
		expect(qualityBadges([{ Quality: { resolution: 2160 }, MediaInfo: { hdr: 'Dolby Vision', audio_codec: 'TrueHD Atmos', subtitle_languages: ['en'] } }])).toEqual([
			'4K',
			'Dolby Vision',
			'Atmos',
			'CC'
		]);
		expect(qualityBadges([{ Quality: { resolution: 1080 } }, { Quality: { resolution: 2160, hdr: 'HDR10' } }])).toEqual(['4K', 'HDR']);
	});

	it('picks the hero status', () => {
		expect(heroStatus([slot({ state: 'in_library' })]).kind).toBe('in_library');
		expect(heroStatus([slot({})], true).kind).toBe('requested');
		expect(heroStatus([slot({})]).kind).toBe('missing');
		const dl = heroStatus([slot({ download: { active: true, progress: 0.42 } as SlotView['download'] })]);
		expect(dl).toEqual({ kind: 'downloading', percent: 42 });
	});

	it('ranks similar titles by overlap and leaves out unrelated ones', () => {
		const cur = { id: 1, kind: 'movie', name: 'A', year: 2020, original_language: 'en', genres: ['Action'], people: ['p1'] };
		const all = [
			cur,
			{ id: 2, kind: 'movie', name: 'B', year: 1990, original_language: 'ja' },
			{ id: 3, kind: 'movie', name: 'C', year: 2021, original_language: 'en' },
			{ id: 4, kind: 'movie', name: 'D', year: 1980, genres: ['Action'], people: ['p1'] },
			{ id: 5, kind: 'show', name: 'E', year: 2020, original_language: 'en' }
		];
		expect(similarTitles(cur, all).map((t) => t.id)).toEqual([4, 3]);
		expect(similarTitles(cur, all, 1).map((t) => t.id)).toEqual([4]);
	});

	it('shares the poster transition name', () => {
		expect(artName(12)).toBe('title-art-12');
	});
});

describe('title page sections', () => {
	it('lists a movie page in page order, with the staff sections for staff', () => {
		expect(pageSections('movie', { manage: true, asks: true }).map((s) => s.label)).toEqual(['Cast', 'Versions', 'Releases', 'Subtitles', 'About', 'Wishlist', 'History', 'Similar']);
	});
	it('leaves out staff sections and an empty Wishlist for members', () => {
		expect(pageSections('movie', { manage: false, asks: false }).map((s) => s.id)).toEqual(['title-cast', 'h-versions', 'h-dates', 'title-similar']);
	});
	it('starts a show page with its seasons', () => {
		const ids = pageSections('show', { manage: false, asks: true }).map((s) => s.id);
		expect(ids).toEqual(['h-seasons', 'title-cast', 'h-show-versions', 'h-show-facts', 'h-show-asks', 'title-similar']);
	});
});
