import { describe, expect, it } from 'vitest';
import { burstLine, byWhom, dayLabel, eventsQuery, feedItems, headVerb, notRoutineKinds, pickedKinds, routineKinds, sortRows, stories, tierKinds, tierOf, titleHref, verbOf } from './history-views.ts';
import { groupDownloads, kindGroups } from './history.ts';
import type { EventRecord } from './types.ts';

let id = 0;
function ev(Kind: string, At: string, extra: Partial<EventRecord> = {}): EventRecord {
	return { ID: ++id, At, Kind, Summary: extra.Summary ?? Kind, TitleID: null, ...extra };
}

describe('tiers', () => {
	it('files results, changes and upkeep apart', () => {
		expect(tierOf('import_done')).toBe('important');
		expect(tierOf('download_failed')).toBe('important');
		expect(tierOf('title_removed')).toBe('important');
		expect(tierOf('health_broken')).toBe('important');
		expect(tierOf('file_replaced')).toBe('changes');
		expect(tierOf('renamed')).toBe('changes');
		expect(tierOf('subtitle_saved')).toBe('changes');
		expect(tierOf('title_refreshed')).toBe('routine');
		expect(tierOf('search_done')).toBe('routine');
		expect(tierOf('albums_changed')).toBe('routine');
		expect(tierOf('something_new')).toBe('changes');
	});
	it('asks the server for every kind across the three tiers, once', () => {
		const all = [...tierKinds('important'), ...tierKinds('changes'), ...tierKinds('routine')];
		for (const g of kindGroups) for (const k of g.kinds) expect(all).toContain(k);
		expect(new Set(all).size).toBe(all.length);
		expect(tierKinds('changes')).toContain('file_replaced');
		expect(tierKinds('important')).not.toContain('file_replaced');
		expect(routineKinds).toContain('albums_changed');
		expect(notRoutineKinds).not.toContain('title_refreshed');
	});
	it('limits picked groups to the tier', () => {
		expect(tierKinds('important', ['scan'])).toEqual([]);
		expect(tierKinds('important', ['fail'])).toContain('download_failed');
		expect(pickedKinds([], notRoutineKinds)).toBe(notRoutineKinds);
		expect(pickedKinds(['scan'], notRoutineKinds)).toEqual([]);
		expect(pickedKinds(['fail'])).toContain('import_failed');
	});
	it('builds the events query with kinds replaced', () => {
		const f = { groups: ['scan'], limit: 50, offset: 0, title: 4 };
		expect(eventsQuery(f)).toContain('kind=');
		const q = new URLSearchParams(eventsQuery(f, { kinds: ['import_done', 'file_replaced'], limit: 12 }));
		expect(q.get('kind')).toBe('import_done,file_replaced');
		expect(q.get('limit')).toBe('12');
		expect(q.get('title')).toBe('4');
	});
});

describe('words', () => {
	it('names what happened', () => {
		expect(verbOf('import_done').word).toBe('Imported');
		expect(verbOf('file_replaced').word).toBe('Upgraded');
		expect(verbOf('download_failed')).toMatchObject({ word: 'Failed', tone: 'fail' });
		expect(verbOf('title_removed').word).toBe('Deleted');
		expect(verbOf('subtitle_saved').word).toBe('Subtitles');
	});
	it('says how far a folded download got', () => {
		const rows = groupDownloads([
			ev('download_failed', '2026-10-21T19:05:03Z', { DownloadID: 7 }),
			ev('download_sent', '2026-10-21T19:05:02Z', { DownloadID: 7 }),
			ev('file_replaced', '2026-10-21T19:05:01Z', { DownloadID: 8 }),
			ev('import_done', '2026-10-21T19:05:00Z', { DownloadID: 8 })
		]);
		expect(headVerb(rows[0]).word).toBe('Download failed');
		expect(headVerb(rows[1]).word).toBe('Upgraded');
	});
	it('says who did it', () => {
		expect(byWhom({ Actor: { kind: 'user', id: 3 }, Data: { asked_by: 'Maya' } })).toBe('Maya');
		expect(byWhom({ Actor: { kind: 'user', id: 1 }, Data: { approved_by: 'owner', asked_by: ['Maya'] } })).toBe('owner');
		expect(byWhom({ Actor: { kind: 'job', id: 9 } })).toBe('Background job');
		expect(byWhom({ Actor: { kind: 'system' } })).toBe('HolyArrs');
	});
	it('names days', () => {
		const now = new Date(2026, 9, 27, 12);
		expect(dayLabel('2026-10-27', now)).toBe('Today');
		expect(dayLabel('2026-10-26', now)).toBe('Yesterday');
		expect(dayLabel('2026-10-25', now)).toBe('Sun, Oct 25');
		expect(dayLabel('2025-10-25', now)).toContain('2025');
	});
	it('only links titles still in the Library', () => {
		const t = new Map([[5, {}]]);
		expect(titleHref(5, t)).toBe('/library/5');
		expect(titleHref(6, t)).toBeUndefined();
		expect(titleHref(null, t)).toBeUndefined();
	});
});

describe('feed', () => {
	const titles = new Map([[1, { kind: 'artist' }], [2, { kind: 'artist' }], [3, { kind: 'artist' }], [9, { kind: 'movie' }]]);
	it('folds a run of refreshes into one line', () => {
		const line = burstLine([1, 2, 3].map((t) => ev('title_refreshed', '2026-10-27T04:00:00Z', { TitleID: t, title_name: `A${t}`, Summary: `Refreshed details of A${t} from MusicBrainz` })), titles);
		expect(line).toBe('Refreshed 3 artists from MusicBrainz');
		expect(burstLine([ev('search_done', 'x', { TitleID: 9, title_name: 'Slow Horses' }), ev('search_done', 'x', { TitleID: 9, title_name: 'Slow Horses' })], titles)).toBe('2 searches for Slow Horses');
	});
	it('puts days first, folds routine runs, keeps important rows on their own', () => {
		const now = new Date('2026-10-27T12:00:00');
		const items = feedItems([
			ev('title_refreshed', '2026-10-27T10:00:00', { TitleID: 1 }),
			ev('albums_changed', '2026-10-27T09:59:00', { TitleID: 1 }),
			ev('title_refreshed', '2026-10-27T09:58:00', { TitleID: 2 }),
			ev('import_done', '2026-10-27T09:00:00', { TitleID: 9, DownloadID: 4 }),
			ev('title_refreshed', '2026-10-27T08:00:00', { TitleID: 3 }),
			ev('title_removed', '2026-10-26T08:00:03', { TitleID: 11 }),
			ev('title_removed', '2026-10-26T08:00:02', { TitleID: 12 }),
			ev('title_removed', '2026-10-26T08:00:01', { TitleID: 13 })
		], titles, now);
		expect(items.map((i) => i.type)).toEqual(['day', 'burst', 'row', 'row', 'day', 'burst']);
		expect(items[0]).toMatchObject({ label: 'Today', count: 5 });
		expect(items[2]).toMatchObject({ tier: 'important' });
		expect(items[5]).toMatchObject({ tier: 'important' });
	});
});

describe('feed families', () => {
	it('folds interleaved refreshes and searches into one line each', () => {
		const items = feedItems([
			ev('title_refreshed', '2026-10-27T10:00:00', { TitleID: 1 }),
			ev('search_done', '2026-10-27T09:59:00', { TitleID: 9, title_name: 'Slow Horses' }),
			ev('title_refreshed', '2026-10-27T09:58:00', { TitleID: 2 }),
			ev('search_done', '2026-10-27T09:57:00', { TitleID: 9, title_name: 'Slow Horses' }),
			ev('folder_scanned', '2026-10-27T09:56:00')
		], new Map(), new Date('2026-10-27T12:00:00'));
		expect(items.map((i) => i.type)).toEqual(['day', 'burst', 'burst', 'row']);
		expect(items[2]).toMatchObject({ line: '2 searches for Slow Horses' });
	});
});

describe('stories', () => {
	it('tells one title oldest first with repeats folded', () => {
		const s = stories([
			ev('import_done', '2026-10-21T19:05:05Z', { TitleID: 9, title_name: 'Se7en' }),
			ev('download_sent', '2026-10-21T19:05:03Z', { TitleID: 9, title_name: 'Se7en' }),
			ev('search_done', '2026-10-21T19:05:02Z', { TitleID: 9, title_name: 'Se7en' }),
			ev('search_done', '2026-10-21T19:05:01Z', { TitleID: 9, title_name: 'Se7en' }),
			ev('title_refreshed', '2026-10-27T01:00:00Z', { TitleID: 1, title_name: 'A1' }),
			ev('health_broken', '2026-10-20T01:00:00Z')
		]);
		expect(s.map((x) => x.name)).toEqual(['A1', 'Se7en', 'HolyArrs itself']);
		expect(s[0].quiet).toBe(true);
		expect(s[1].quiet).toBe(false);
		expect(s[1].steps.map((x) => [x.verb.word, x.count])).toEqual([['Searched', 2], ['Sent', 1], ['Imported', 1]]);
	});
});

describe('table', () => {
	it('sorts a page by a column', () => {
		const rows = groupDownloads([ev('renamed', '2026-10-27T01:00:00Z', { title_name: 'b' }), ev('renamed', '2026-10-27T02:00:00Z', { title_name: 'a' })]);
		expect(sortRows(rows, 'title', 1).map((r) => r.head.title_name)).toEqual(['a', 'b']);
		expect(sortRows(rows, 'at', 1).map((r) => r.head.title_name)).toEqual(['b', 'a']);
	});
});
