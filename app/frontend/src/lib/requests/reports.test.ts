import { episodeWord, factsLine, filterOf, hasFile, kindHint, kindIcon, kindLabel, partWords, quickReplies, listPath, partLine, reportBody, reportHref, reportKinds, searchBody, sortReports, stateCounts, titleLine, whoName } from './reports.ts';
import { describe, expect, it } from 'vitest';

describe('reported problems', () => {
	it('names the five problem kinds in order', () => {
		expect(reportKinds.map((k) => k.label)).toEqual(['Picture', 'Sound', 'Subtitles', "Won't play", 'Other']);
		expect(kindLabel('wont_play')).toBe("Won't play");
		expect(kindLabel('something_new')).toBe('something_new');
	});

	it('sends only what was picked, with the note trimmed', () => {
		expect(reportBody({ title_id: 4, kind: 'sound', note: '  out of sync  ' })).toEqual({ title_id: 4, kind: 'sound', note: 'out of sync' });
		expect(reportBody({ title_id: 4, kind: 'picture', note: '', season: null, item_id: 0, version_id: undefined })).toEqual({ title_id: 4, kind: 'picture' });
		expect(reportBody({ title_id: 4, kind: 'other', season: 0, item_id: 12, album_id: 3, version_id: 2 })).toEqual({ title_id: 4, kind: 'other', season: 0, item_id: 12, album_id: 3, version_id: 2 });
		expect((reportBody({ title_id: 1, kind: 'other', note: 'x'.repeat(600) }).note as string).length).toBe(500);
	});

	it('gives each kind its picture, hint and quick replies', () => {
		expect(reportKinds.map((k) => kindIcon(k.kind))).toEqual(['image', 'sound', 'subtitle', 'play', 'info']);
		expect(kindIcon('something_new')).toBe('info');
		expect(kindHint('sound')).toMatch(/No sound/);
		expect(kindHint('something_new')).toBe('');
		expect(quickReplies('subtitles')[0]).toBe('Added new subtitles');
		expect(quickReplies('other')).toHaveLength(3);
	});

	it('words the line under a title', () => {
		expect(factsLine({ title_kind: 'show', year: 2008, where: 'Season 2' })).toBe('TV show · 2008 · Season 2');
		expect(factsLine({ title_kind: 'movie', year: 2014, where: '(Main)' })).toBe('Movie · 2014 · Main version');
		expect(factsLine({ title_kind: 'artist' })).toBe('Music');
		expect(partWords({ where: 'S02E03', item_name: 'The Mill' })).toBe('S02E03 · The Mill');
	});

	it('asks the server for one filter', () => {
		expect(listPath('open')).toBe('/reports?state=open');
		expect(listPath('all')).toBe('/reports');
		expect(listPath('fixed', { titleId: 7, mine: true, limit: 50 })).toBe('/reports?state=fixed&title_id=7&mine=1&limit=50');
		expect(filterOf('fixed')).toBe('fixed');
		expect(filterOf('all')).toBe('all');
		expect(filterOf('bogus')).toBe('open');
		expect(filterOf(null)).toBe('open');
	});

	it('counts and sorts: open first, then the newest', () => {
		const rs = [
			{ id: 1, state: 'fixed', created_at: '2026-11-03T10:00:00Z' },
			{ id: 2, state: 'open', created_at: '2026-11-01T10:00:00Z' },
			{ id: 3, state: 'open', created_at: '2026-11-02T10:00:00Z' }
		];
		expect(stateCounts(rs)).toEqual({ open: 2, fixed: 1, all: 3 });
		expect(sortReports(rs).map((r) => r.id)).toEqual([3, 2, 1]);
		expect(rs.map((r) => r.id)).toEqual([1, 2, 3]);
	});

	it('says which part and whose report in words', () => {
		expect(titleLine({ title_name: 'Driftwood Moon', year: 2024 })).toBe('Driftwood Moon (2024)');
		expect(titleLine({ title_name: 'Driftwood Moon' })).toBe('Driftwood Moon');
		expect(partLine({ where: 'S02E03', item_name: 'The Mill' })).toBe('S02E03 · The Mill');
		expect(partLine({ where: 'Season 2' })).toBe('Season 2');
		expect(partLine({ where: '', item_name: '' })).toBe('');
		expect(partLine({ where: 'Lanterns, track 4: Night Bus', item_name: 'Night Bus' })).toBe('Lanterns, track 4: Night Bus');
		expect(whoName({ id: 2, name: 'kim' })).toBe('kim');
		expect(whoName({ id: 2, name: 'kim', display_name: 'Kim R' })).toBe('Kim R');
		expect(whoName(undefined)).toBe('');
	});

	it('opens the right page for a report', () => {
		expect(reportHref({ title_id: 4, title_kind: 'movie' })).toBe('/library/4');
		expect(reportHref({ title_id: 4, title_kind: 'show', season: 2 })).toBe('/library/4?tab=library');
		expect(reportHref({ title_id: 9, title_kind: 'artist', album_id: 3 })).toBe('/library/9/albums/3');
	});

	it('narrows Find a better copy to the reported part', () => {
		expect(searchBody({ title_kind: 'movie', item_id: 5, version_id: 2 })).toEqual({});
		expect(searchBody({ title_kind: 'show', season: 2, item_id: 41, version_id: 3 })).toEqual({ item_id: 41, version_id: 3 });
		expect(searchBody({ title_kind: 'show', season: 0 })).toEqual({ season: 0 });
		expect(searchBody({ title_kind: 'show' })).toEqual({});
		expect(searchBody({ title_kind: 'artist', album_id: 7 })).toEqual({ album_id: 7 });
		expect(searchBody({ title_kind: 'artist', album_id: 7, item_id: 70 })).toEqual({ item_id: 70 });
	});

	it('words episodes and knows which have a file', () => {
		expect(episodeWord({ episode: 3, name: 'The Mill' })).toBe('E03 · The Mill');
		expect(episodeWord({ episode: 12 })).toBe('E12');
		expect(hasFile({ slots: [{ state: 'missing', file: null }] })).toBe(false);
		expect(hasFile({ slots: [{ state: 'missing', file: null }, { state: 'in_library' }] })).toBe(true);
		expect(hasFile({ slots: [{ file: { ID: 1 } }] })).toBe(true);
		expect(hasFile({ slots: [{ state: 'missing', file_id: 8 }] })).toBe(true);
		expect(hasFile({})).toBe(false);
	});
});
