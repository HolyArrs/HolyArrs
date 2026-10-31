import { describe, expect, it } from 'vitest';
import { askedByOne, askers, inboxTabs, dateGroups, loadGroupBy, saveGroupBy, loadPanel, loadSize, movieRelease, readInfo, runtimeShort, savePanel, saveSize, seasonRange, showNotOut, showSummary, watchSeason } from './inbox.ts';

const seasons = [
	{ number: 0, episode_count: 2 },
	{ number: 1, episode_count: 7, year: 2020 },
	{ number: 2, episode_count: 10, year: 2021 },
	{ number: 3, episode_count: 13, year: 2022 },
	{ number: 4, episode_count: 10, year: 2023 },
	{ number: 5, episode_count: 6, year: 2024 },
	{ number: 6, episode_count: 16, year: 2026 }
];
const now = new Date('2026-10-27T12:00:00Z');

describe('approvals inbox', () => {
	it('groups by date, newest first, and remembers Group by per member', () => {
		const g = dateGroups([{ asked_at: '2026-10-01T10:00:00Z' }, { asked_at: '2026-10-27T11:00:00Z' }, { asked_at: '2026-10-24T10:00:00Z' }], now);
		expect(g.map((x) => x.name)).toEqual(['Today', 'This week', 'Older']);
		const m = new Map<string, string>();
		const store = { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
		expect(loadGroupBy(3, store)).toBe('type');
		saveGroupBy(3, 'date', store);
		expect(loadGroupBy(3, store)).toBe('date');
		expect(loadGroupBy(4, store)).toBe('type');
	});
	it('uses the glossary words on the tabs', () => {
		expect(inboxTabs.map((t) => t.label)).toEqual(['Pending', 'Approved', 'Declined', 'Available', 'Failed']);
	});

	it('says what a show ask is for, with season pills', () => {
		expect(showSummary({ seasons: [] }, seasons).line).toBe('6 seasons, 62 episodes');
		expect(showSummary({ seasons: [1, 2, 3] }, seasons)).toMatchObject({ line: 'Seasons 1–3 · 3 of 6 seasons, 30 episodes' });
		expect(showSummary({ seasons: [1, 2, 3] }, seasons).pills.map((p) => p.label)).toEqual(['S1: all 7', 'S2: all 10', 'S3: all 13']);
		expect(showSummary({ seasons: [2] }, seasons, { 2: [1, 2, 3, 4] }).line).toBe('4 episodes from season 2');
		const mixed = showSummary({ seasons: [] }, seasons, { 1: [1, 2, 3], 3: [4, 5, 6, 7], 6: [] });
		// 3 + 4 + all 16 (the spec's example line said 19; the pills add up to 23).
		expect(mixed.line).toBe('23 episodes from 3 seasons');
		expect(mixed.pills.map((p) => p.label)).toEqual(['S1: 3 of 7', 'S3: 4 of 13', 'S6: all 16']);
		expect(mixed.pills[0].episodes).toEqual([1, 2, 3]);
		expect(showSummary({ future_only: true }, seasons).line).toBe('New episodes from now on');
		// No metadata yet: the server's own words.
		expect(showSummary({ seasons: [1, 2], season_words: 'seasons 1 and 2' }, []).line).toBe('Seasons 1 and 2');
		expect(showSummary({ seasons: [] }, []).line).toBe('The entire show');
	});

	it('names season runs', () => {
		expect(seasonRange([2])).toBe('Season 2');
		expect(seasonRange([3, 1, 2])).toBe('Seasons 1–3');
		expect(seasonRange([1, 3, 5])).toBe('Seasons 1, 3 and 5');
	});

	it('says a runtime and whether a movie is out', () => {
		expect(runtimeShort(130)).toBe('2h 10m');
		expect(runtimeShort(48)).toBe('48m');
		expect(runtimeShort(120)).toBe('2h');
		expect(movieRelease({ dates: { theaters: '2019-10-11', digital: '2020-01-14' } }, now)).toEqual({ words: 'out now', notOut: '' });
		expect(movieRelease({ dates: { theaters: '2026-10-01', digital: '2026-12-20' } }, now)).toEqual({ words: 'in cinemas only', notOut: '' });
		expect(movieRelease({ dates: { theaters: '2027-03-03' } }, now)).toEqual({ words: 'in cinemas Mar 3, 2027', notOut: 'Not out yet · in cinemas Mar 3, 2027' });
		expect(movieRelease({ dates: {}, status: 'Released' }, now).words).toBe('out now');
		expect(movieRelease(null, now).words).toBe('');
	});

	it('says when part of a show ask is not out yet', () => {
		const info = { seasons, dates: {} };
		expect(watchSeason({ kind: 'show', seasons: [1, 2] }, info, now)).toBe(0);
		expect(watchSeason({ kind: 'show', seasons: [] }, info, now)).toBe(6);
		expect(showNotOut(6, [{ number: 1, air_day: '2027-03-03' }, { number: 2, air_day: '2027-03-10' }], now)).toBe('Not out yet · S6 airs Mar 3, 2027');
		expect(showNotOut(6, [{ number: 1, air_day: '2026-10-01' }, { number: 7, air_day: '2026-11-03' }], now)).toBe('Not out yet · S6 E7 airs Nov 3');
		expect(showNotOut(6, [{ number: 1, air_day: '2026-10-01' }], now)).toBe('');
	});

	it('reads the metadata parts it needs', () => {
		const i = readInfo({ seasons: [{ number: 1, episode_count: 10, year: 2020 }], dates: { first_air: '2020-08-14', x: 3 }, status: 'Returning Series', original_language: 'en' });
		expect(i).toEqual({ seasons: [{ number: 1, episode_count: 10, year: 2020 }], dates: { first_air: '2020-08-14' }, status: 'Returning Series', originalLanguage: 'en' });
		expect(readInfo({})).toEqual({ seasons: [], dates: {}, status: undefined, originalLanguage: undefined });
	});

	it('lists the people who asked and filters by one', () => {
		const maya = { id: 2, name: 'Maya' }, leo = { id: 3, name: 'Leo' };
		const asks = [{ asked_by: maya, people: [{ ...maya, asked_at: '' }] }, { asked_by: leo, people: [{ ...leo, asked_at: '' }, { ...maya, asked_at: '' }] }, { asked_by: maya, people: [] }];
		expect(askers(asks)).toEqual([{ id: 3, name: 'Leo', n: 1 }, { id: 2, name: 'Maya', n: 2 }]);
		expect(asks.filter((a) => askedByOne(a, 2)).length).toBe(3);
	});

	it('remembers the poster size and the panel per member', () => {
		const m = new Map<string, string>();
		const store = { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
		expect(loadSize(5, store)).toBe('m');
		saveSize(5, 'l', store);
		expect(loadSize(5, store)).toBe('l');
		expect(loadPanel(5, store)).toBe(true);
		savePanel(5, false, store);
		expect(loadPanel(5, store)).toBe(false);
		expect(loadPanel(6, store)).toBe(true);
	});
});
