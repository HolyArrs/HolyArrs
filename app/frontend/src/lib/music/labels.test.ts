import { albumKind, albumLine, albumState, albumStateOf, albumWords, albumYear, artistProgress, artistState, pressingLabel, toggle, trackLength, extraKinds, word, albumChoices, artistSearchToast, discography, discographyRow, latestRelease } from './labels.ts';
import { describe, expect, it } from 'vitest';
import type { ArtistInfo } from '../core/types.ts';

const info = (a: Partial<ArtistInfo>): ArtistInfo => ({
	mbid: 'x', status: 'running', new_albums: 'all', album_types: ['album'], secondary_types: ['studio'], release_statuses: ['official'],
	albums: 4, included: 3, tracks: 0, in_library: 0, missing: 0, not_out_yet: 0, better_wanted: 0, ...a
});

describe('music words', () => {
	it('names album kinds and years', () => {
		expect(albumKind({ type: 'album', secondary_types: [] })).toBe('Album');
		expect(albumKind({ type: 'album', secondary_types: ['live', 'dj-mix'] })).toBe('Album · Live · DJ mix');
		expect(albumKind({ type: '', secondary_types: [] })).toBe('No kind');
		expect(albumYear('2019-04-12')).toBe('2019');
		expect(albumYear(undefined)).toBe('');
		expect(word(albumChoices, 'latest')).toBe('The latest album');
	});
	it('says where an album is', () => {
		expect(albumState('partial')).toEqual({ label: 'Some tracks in', tone: 'wait' });
		expect(albumState('complete').tone).toBe('ok');
		const now = Date.parse('2026-10-13T00:00:00Z');
		expect(albumStateOf({ state: 'no_tracks', release_date: '2099-05-01T00:00:00Z' }, now).label).toBe('Not out yet');
		expect(albumStateOf({ state: 'no_tracks', release_date: '2020-05-01T00:00:00Z' }, now).label).toBe('No tracks yet');
		expect(albumLine({ included: false, tracks: 0, in_library: 0, missing: 0 })).toBe('Left out by the album kinds');
		expect(albumLine({ included: true, tracks: 0, in_library: 0, missing: 0 })).toBe('Tracks come with the next refresh');
		expect(albumLine({ included: true, tracks: 12, in_library: 8, missing: 4 })).toBe('8 of 12 tracks in the Library · 4 missing');
	});
	it('sums an artist up', () => {
		expect(artistProgress(undefined)).toBe('');
		expect(artistProgress(info({}))).toBe('3 of 4 albums included; tracks come with the next refresh.');
		expect(artistProgress(info({ tracks: 12, in_library: 3, missing: 7, not_out_yet: 2 }))).toBe('3 of 12 tracks in the Library · 7 missing · 2 not out yet');
		expect(artistState(info({ tracks: 4, missing: 2 }))).toEqual({ state: 'wanted', label: '2 missing' });
		expect(artistState(info({ tracks: 4, in_library: 4 })).label).toBe('In library');
		expect(artistState(info({ tracks: 5, in_library: 3, better_wanted: 3 }))).toEqual({ state: 'in_library', label: '3 tracks in library' });
		expect(artistState(info({ tracks: 5, in_library: 1 })).label).toBe('1 track in library');
		expect(artistState(info({ tracks: 5, in_library: 3, missing: 2 })).label).toBe('2 missing');
		expect(artistState(info({})).label).toBe('');
	});
	it('says lengths, Pressings and asked albums', () => {
		expect(trackLength(185000)).toBe('3:05');
		expect(trackLength(undefined)).toBe('');
		expect(pressingLabel({ name: 'Harbour Songs', disambiguation: 'deluxe', date: '2019-04-12', country: 'GB', formats: ['CD', 'CD'], track_count: 4 })).toBe(
			'Harbour Songs (deluxe) · 2019 · GB · CD · 4 tracks'
		);
		expect(pressingLabel({ name: 'Single', formats: [], track_count: 1 })).toBe('Single · 1 track');
		expect(albumWords([])).toBe('every album');
		expect(albumWords([{ mbid: 'a', name: 'A' }, { mbid: 'b', name: 'B' }])).toBe('A and B');
		expect(albumWords(['A', 'B', 'C', 'D', 'E'].map((n) => ({ mbid: n, name: n })))).toBe('A, B and 3 more albums');
	});
	it('keeps kind lists in their own order', () => {
		expect(toggle(['live'], 'studio', true, extraKinds)).toEqual(['studio', 'live']);
		expect(toggle(['studio', 'live'], 'studio', false, extraKinds)).toEqual(['live']);
	});
});

describe('artist search toast', () => {
	it('says how many albums were searched and how many were left out', () => {
		expect(artistSearchToast(0)).toBe('No album is missing a track right now.');
		expect(artistSearchToast(1)).toBe('Searching for 1 album, a little apart. Home shows what they find.');
		expect(artistSearchToast(200, 37)).toContain('37 more are left for the next Search now');
		expect(artistSearchToast(200, 1)).toContain('1 more is left');
	});
});

describe('discography', () => {
	const a = (type: string, secondary_types: string[] = [], release_date?: string) => ({ type, secondary_types, release_date });
	it('groups albums into rows in page order, newest first', () => {
		const rows = discography([a('single', [], '2019-01-01'), a('album', [], '2018-01-01'), a('album', ['live'], '2020-01-01'), a('album', [], '2021-05-01'), a('ep', [], '2022-01-01'), a('album', ['compilation'])]);
		expect(rows.map((r) => r.row)).toEqual(['albums', 'singles', 'live', 'compilations']);
		expect(rows[0].albums.map((x) => x.release_date)).toEqual(['2021-05-01', '2018-01-01']);
		expect(rows[1].albums.map((x) => x.type)).toEqual(['ep', 'single']);
		expect(discographyRow(a('album', ['soundtrack']))).toBe('other');
	});
	it('picks the newest album or EP that is out as the latest release', () => {
		const now = Date.parse('2024-01-01');
		expect(latestRelease([a('album', [], '2020-01-01'), a('album', [], '2030-01-01'), a('album', ['live'], '2023-01-01')], now)?.release_date).toBe('2020-01-01');
		expect(latestRelease([a('album')], now)).toBeNull();
	});
});
