import { describe, expect, it } from 'vitest';
import { artName, crumbFor, pageSections, qualityBadges, similarTitles } from './title-page.ts';
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
		expect(pageSections('movie', { manage: true }).map((s) => s.label)).toEqual(['Where to watch', 'Cast', 'Collection', 'More like this', 'About', 'Library']);
	});
	it('leaves the manager Library section out for requesters', () => {
		expect(pageSections('movie', { manage: false }).map((s) => s.id)).toEqual(['title-where', 'title-cast', 'title-collection', 'title-similar', 'h-about']);
		expect(pageSections('artist', { manage: false }).map((s) => s.id)).not.toContain('h-library');
	});
	it('starts a show page with its episodes and keeps Library last', () => {
		const ids = pageSections('show', { manage: true }).map((s) => s.id);
		expect(ids[0]).toBe('h-seasons');
		expect(ids.at(-1)).toBe('h-library');
		expect(pageSections('album', { manage: false }).map((s) => s.label)).toEqual(['Tracks', 'More by the artist', 'About']);
	});
});
