import { describe, expect, it } from 'vitest';
import { keepOffNote, newSeason, newestSeason, episodeCode, searchWord, seasonLooking, episodeLabel, episodeWhy, chosenWhy, label, seasonChoices, seasonLine, seasonName, seasonWords, showProgress } from './labels.ts';
import { counts, matches, ofKind } from '../mediafiles/library.ts';
import { airedEpisodes, progressOf } from '../mediafiles/table.ts';
import type { EpisodeView, SeasonSummary, ShowInfo, TitleView } from '../core/types.ts';

function season(number: number, o: Partial<SeasonSummary> = {}): SeasonSummary {
	return { number, name: seasonName(number), keep_up_to_date: true, episodes: 3, aired: 3, in_library: 0, missing: 0, not_out_yet: 0, better_wanted: 0, ...o };
}

function info(seasons: SeasonSummary[]): ShowInfo {
	const sum = (k: 'missing' | 'not_out_yet' | 'in_library' | 'better_wanted' | 'episodes') => seasons.reduce((n, s) => n + s[k], 0);
	return {
		details_source: 'tmdb', episode_order: 'aired', new_seasons: 'all', status: 'running', seasons,
		episodes: sum('episodes'), missing: sum('missing'), not_out_yet: sum('not_out_yet'), in_library: sum('in_library'), better_wanted: sum('better_wanted')
	};
}

function show(name: string, s: ShowInfo): TitleView {
	return {
		id: 9, kind: 'show', name, ids: null, keep_up_to_date: true, why_added: 'by_hand', release_gate: 'released', gate_delay_days: 0,
		artwork: [], stage: 'running', gate_open: true, gate_reason: '', versions: [], slots: [], show: s
	};
}

function ep(o: Partial<EpisodeView> = {}): EpisodeView {
	return { id: 1, season: 1, episode: 2, keep_up_to_date: true, ids: {}, slots: [{ id: 1, version: 'Main', version_id: 1, state: 'wanted', state_label: 'Missing' }], ...o };
}

describe('show words', () => {
	it('says which seasons an ask is for', () => {
		expect(seasonWords([])).toBe('the whole show');
		expect(seasonWords(undefined)).toBe('the whole show');
		expect(seasonWords([2])).toBe('season 2');
		expect(seasonWords([1, 2, 4])).toBe('seasons 1, 2 and 4');
	});
	it('names seasons and episodes in the show order', () => {
		expect(seasonName(0)).toBe('Specials');
		expect(episodeCode(1, 2)).toBe('S01E02');
		expect(episodeLabel({ season: 1, episode: 2, air_day: '2025-01-12' }, 'daily')).toBe('2025-01-12 (S01E02)');
		expect(episodeLabel({ season: 2, episode: 1, absolute: 27 }, 'absolute')).toBe('27 (S02E01)');
		expect(episodeLabel({ season: 2, episode: 1 }, 'absolute')).toBe('S02E01');
		expect(label(seasonChoices, 'latest')).toBe('The latest season');
	});
	it('sums up a season and a show', () => {
		expect(seasonLine(season(1, { in_library: 2, missing: 1 }))).toBe('2 of 3 aired in the Library · 1 missing');
		const s = info([season(1, { in_library: 3 }), season(2, { aired: 1, episodes: 2, missing: 1, not_out_yet: 1 })]);
		expect(showProgress(s)).toBe('3 of 4 aired episodes in the Library');
	});
	it('counts aired episodes the same way in the list, its tooltip and the show page', () => {
		// Specials not kept up to date count nowhere; an episode not aired yet (one with no Slot too) is not counted.
		const s = info([season(0, { keep_up_to_date: false, aired: 80, episodes: 80 }), season(1, { aired: 3 }), season(2, { aired: 1, episodes: 3, not_out_yet: 1 })]);
		expect(airedEpisodes(s)).toBe(4);
		expect(progressOf({ kind: 'show', show: s } as TitleView).total).toBe(4);
		expect(showProgress(s)).toBe('0 of 4 aired episodes in the Library');
	});
	it('says why an episode is or is not looked for', () => {
		expect(episodeWhy(ep(), true, false)).toBe('Keep up to date is off for the show.');
		expect(episodeWhy(ep(), false, true)).toBe('Keep up to date is off for this season.');
		expect(episodeWhy(ep({ keep_up_to_date: false }), true, true)).toBe('Keep up to date is off for this episode.');
		expect(episodeWhy(ep({ slots: [{ id: 1, version: 'Main', version_id: 1, state: 'not_out_yet', state_label: 'Not out yet' }] }), true, true)).toBe('No air date yet.');
		expect(episodeWhy(ep({ slots: [{ id: 1, version: 'Main', version_id: 1, state: 'wanted', state_label: 'Missing', reason: 'searching shows comes later' }] }), true, true)).toBe(
			'searching shows comes later'
		);
		expect(episodeWhy(ep(), true, true)).toBe('');
	});
});

describe('shows in the Library filters', () => {
	const a = show('Harbour Lights', info([season(1, { in_library: 3 }), season(2, { missing: 1, not_out_yet: 1 })]));
	const b = show('Moss Parade', info([season(1, { in_library: 3, better_wanted: 1 })]));
	it('counts a show when any kept episode is in that state', () => {
		expect(matches(a, 'missing')).toBe(true);
		expect(matches(a, 'not_out_yet')).toBe(true);
		expect(matches(b, 'missing')).toBe(false);
		expect(matches(b, 'better')).toBe(true);
		expect(counts([a, b])).toEqual({ all: 2, missing: 1, not_out_yet: 1, better: 1, have: 2 });
	});
	it('picks by kind', () => {
		expect(ofKind(a, 'show')).toBe(true);
		expect(ofKind(a, 'movie')).toBe(false);
		expect(ofKind({ kind: 'movie' }, 'all')).toBe(true);
	});
});

describe('searching episodes', () => {
	it('words the search button by Slot state', () => {
		expect(searchWord('wanted')).toBe('Search now');
		expect(searchWord('not_out_yet')).toBe('Search now');
		expect(searchWord('better_wanted')).toBe('Find a better copy');
		expect(searchWord('in_library')).toBe('');
		expect(searchWord('downloading')).toBe('');
		expect(searchWord(undefined)).toBe('');
	});
	it('counts what a season search could fill', () => {
		const e = (state: string) => ({ id: 1, season: 1, episode: 1, keep_up_to_date: true, ids: {}, slots: [{ id: 1, version: 'Main', version_id: 1, state, state_label: '' }] }) as EpisodeView;
		expect(seasonLooking([e('wanted'), e('better_wanted'), e('in_library'), e('not_out_yet'), e('chosen')])).toBe(2);
	});
});

describe('the season a show opens on', () => {
	const s = (number: number, x: Partial<SeasonSummary> = {}) => ({ number, aired: 10, not_out_yet: 0, ...x });
	it('opens on the newest season with aired or upcoming episodes', () => {
		expect(newestSeason([s(0), s(1), s(2), s(3, { aired: 0, not_out_yet: 4 })])).toBe(3);
		expect(newestSeason([s(1), s(2), s(3, { aired: 0, not_out_yet: 0 })])).toBe(2);
		expect(newestSeason([s(0)])).toBe(0);
		expect(newestSeason([])).toBeUndefined();
	});
	it('marks the newest season New while it airs', () => {
		expect(newSeason([s(1), s(6, { aired: 3, not_out_yet: 9 })])).toBe(6);
		expect(newSeason([s(1), s(2, { next_air_date: '2026-11-01' })])).toBe(2);
		expect(newSeason([s(1), s(2)])).toBeUndefined();
		const now = new Date('2026-10-20');
		expect(newSeason([{ number: 1, episodes: 8, year: 2023 }, { number: 2, episodes: 8, year: 2026 }], now)).toBe(2);
		expect(newSeason([{ number: 1, episodes: 8, year: 2020 }], now)).toBeUndefined();
	});
	it('says Keep up to date is off once for the season', () => {
		expect(keepOffNote(true, false)).toBe('Keep up to date is off for the show.');
		expect(keepOffNote(false, true)).toBe('Keep up to date is off for this season.');
		expect(keepOffNote(true, true)).toBe('');
	});
});

describe('packWindowValue', () => {
	it('keeps a season pack window between 1 and 10, and 0 when off', async () => {
		const { packWindowValue } = await import('./labels.ts');
		expect(packWindowValue(0)).toBe(0);
		expect(packWindowValue(2)).toBe(2);
		expect(packWindowValue(14)).toBe(10);
		expect(packWindowValue(-3)).toBe(0);
		expect(packWindowValue(2.6)).toBe(3);
		expect(packWindowValue(Number.NaN)).toBe(1);
	});
});

describe('chosenWhy', () => {
	it('gives the reason under a chosen episode', () => {
		expect(chosenWhy({ state: 'chosen', reason: 'No Downloader has confirmed this Release yet. You can choose another one.' })).toBe('No Downloader has confirmed this Release yet. You can choose another one.');
	});
	it('stays empty for other states or no reason', () => {
		expect(chosenWhy({ state: 'chosen' })).toBe('');
		expect(chosenWhy({ state: 'wanted', reason: 'x' })).toBe('');
		expect(chosenWhy(undefined)).toBe('');
	});
});
