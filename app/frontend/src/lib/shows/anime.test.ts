import { describe, expect, it } from 'vitest';
import { animeEpisode, animePath, seasonAnimeLine, titleLine, type AnimeView } from './anime.ts';

const view: AnimeView = {
	anime: true,
	mapped: true,
	alt_names: ['Gousei Shou', '合成ショー'],
	titles: [],
	seasons: [
		{ number: 0, anilist: 14 },
		{ number: 1, anilist: 11, absolute_from: 1, title: { anilist: 11, romaji: 'Gousei Shou', english: 'Synthetic Show', native: '合成ショー' } },
		{
			number: 2,
			anilist: 12,
			absolute_from: 13,
			title: { anilist: 12, romaji: 'Gousei Shou 2nd Season', english: 'gousei shou 2nd season' },
			parts: [
				{ anilist: 12, offset: 0 },
				{ anilist: 13, offset: 12, title: { anilist: 13, romaji: 'Gousei Shou Part 2', english: 'Synthetic Show Part 2' } }
			]
		}
	]
};

describe('anime title page', () => {
	it('opens the anime lookup address for a show', () => {
		expect(animePath('show', '9000')).toBe('/metadata/show/9000/anime');
	});
	it('shows each title variant once on the title line', () => {
		expect(titleLine(view.seasons[1].title)).toBe('Gousei Shou · Synthetic Show · 合成ショー');
		expect(titleLine(view.seasons[2].title)).toBe('Gousei Shou 2nd Season');
		expect(titleLine(undefined)).toBe('');
	});
	it('shows title, absolute range and later parts on a season line', () => {
		expect(seasonAnimeLine(view, 1, 12)).toBe('Gousei Shou · Synthetic Show · 合成ショー · Absolute episodes 1–12');
		expect(seasonAnimeLine(view, 2, 24)).toBe('Gousei Shou 2nd Season · Absolute episodes 13–36 · Part 2 from episode 13 (Gousei Shou Part 2)');
		expect(seasonAnimeLine(view, 0, 2)).toBe('');
		expect(seasonAnimeLine(view, 5, 2)).toBe('');
		expect(seasonAnimeLine(null, 1, 12)).toBe('');
	});
	it('counts an episode from the start of its part', () => {
		expect(animeEpisode(view, 1, 5)).toEqual({ anilist: 11, episode: 5, absolute: 5 });
		expect(animeEpisode(view, 2, 12)).toEqual({ anilist: 12, episode: 12, absolute: 24 });
		expect(animeEpisode(view, 2, 13)).toEqual({ anilist: 13, episode: 1, absolute: 25 });
		expect(animeEpisode(view, 0, 1)).toEqual({ anilist: 14, episode: 1, absolute: undefined });
		expect(animeEpisode(null, 3, 4)).toEqual({ episode: 4 });
	});
});
