import { describe, expect, it } from 'vitest';
import { agoShort, canFind, rowLinks, dateLabel, isProblem, isRoutine, rowExtra, rowStep, dirQuery, readSearch, searchChips, stepKinds, textMatches, toggleStep, typeMatches, whoMatches } from './dirs.ts';
import type { EventRecord } from '../types.ts';
import { downloadSteps, type HistoryRow } from '../history.ts';

let id = 0;
const ev = (Kind: string, extra: Partial<EventRecord> = {}): EventRecord => ({ ID: ++id, At: '2026-11-01T10:00:00Z', Kind, Summary: Kind, TitleID: null, ...extra });

describe('history filters', () => {
	it('drops empty values and the offset when the address changes', () => {
		const q = new URLSearchParams('look=feed&title=4&offset=50');
		expect(dirQuery(q, { title: undefined, who: 'me' })).toBe('?look=feed&who=me');
	});

	it('tells who made an Event happen', () => {
		const mine = ev('ask_made', { Actor: { kind: 'user', id: 2 } });
		expect(whoMatches(mine, 'me', 2)).toBe(true);
		expect(whoMatches(mine, 'me', 3)).toBe(false);
		expect(whoMatches(mine, 'holyarrs')).toBe(false);
		expect(whoMatches(ev('search_done', { Actor: { kind: 'job' } }), 'holyarrs')).toBe(true);
	});

	it('maps steps to kinds and leaves routine work out', () => {
		expect(stepKinds(['asked', 'chosen'])).toEqual(['ask_made', 'release_chosen', 'release_chosen_by_hand']);
		expect(stepKinds(['failed'])).toContain('download_failed');
		const all = stepKinds([]);
		expect(all).toContain('import_done');
		expect(all).not.toContain('search_done');
		expect(all).not.toContain('library_scanned');
		expect(all.length).toBeLessThanOrEqual(50);
	});

	it('reads the search look and builds a chip per filter', () => {
		const s = readSearch(new URLSearchParams('look=pill&q=dune&step=failed,asked,bogus&type=movies&who=me&from=2026-10-26'));
		expect(s.steps).toEqual(['failed', 'asked']);
		expect(toggleStep(s.steps, 'failed')).toBe('asked');
		const chips = searchChips(s, new Date('2026-11-02T12:00:00Z'));
		expect(chips.map((c) => c.label)).toEqual(['Asked', 'Failed', 'Movies', 'You', 'Last 7 days']);
		expect(chips.find((c) => c.key === 'step.failed')?.change).toEqual({ step: 'asked' });
		expect(searchChips(readSearch(new URLSearchParams('q=x')))).toEqual([]);
		expect(dateLabel('2026-10-01', '2026-10-05')).toBe('2026-10-01 to 2026-10-05');
	});

	it('matches search text and type', () => {
		const e = ev('download_sent', { title_name: 'Dune', Data: { source_name: 'NZBgeek' }, Actor: { kind: 'user', id: 2 } });
		expect(textMatches(e, 'dun')).toBe(true);
		expect(textMatches(e, 'geek')).toBe(true);
		expect(textMatches(e, 'luis', new Map([[2, 'Luis']]))).toBe(true);
		expect(textMatches(e, 'alien')).toBe(false);
		expect(typeMatches('artist', 'music')).toBe(true);
		expect(typeMatches('show', 'movies')).toBe(false);
	});
});

describe('search look rows', () => {
	const one = (e: EventRecord): HistoryRow => ({ key: `e${e.ID}`, head: e, events: [e], steps: [] });
	const dl = (...events: EventRecord[]): HistoryRow => ({ key: 'd1', head: events[0], events, steps: downloadSteps(events) });

	it('names the step in plain words', () => {
		expect(rowStep(one(ev('ask_made'))).label).toBe('Asked');
		expect(rowStep(one(ev('media_server_told'))).label).toBe('In media server');
		expect(rowStep(one(ev('file_replaced'))).label).toBe('Upgraded');
		expect(rowStep(one(ev('title_added')), 'Added').label).toBe('Added');
		expect(rowStep(dl(ev('import_done'), ev('download_finished'), ev('download_sent'))).label).toBe('Imported');
		expect(rowStep(dl(ev('download_downloading'), ev('download_sent'))).label).toBe('Downloading');
		const f = rowStep(dl(ev('download_failed'), ev('download_sent')));
		expect(f).toEqual({ label: 'Failed', icon: 'x', failed: true });
		expect(isProblem('library_folder_gone')).toBe(true);
		expect(isProblem('import_done')).toBe(false);
	});

	it('adds at most one extra piece, never a placeholder', () => {
		const who = () => 'Maya';
		const ask = one(ev('ask_approved'));
		expect(rowExtra(ask, rowStep(ask), who)).toBe('Maya');
		const chose = one(ev('release_chosen', { Data: { release: 'Show.S01E01.1080p.WEB' } }));
		expect(rowExtra(chose, rowStep(chose))).toBe('1080p');
		const imp = dl(ev('import_done', { Data: { size_bytes: 2_000_000_000 } }), ev('download_sent'));
		expect(rowExtra(imp, rowStep(imp))).toMatch(/GB/);
		const sub = one(ev('subtitle_saved', { Data: { language: 'en' } }));
		expect(rowExtra(sub, rowStep(sub))).toBe('EN');
		const bad = dl(ev('download_failed', { Data: { reason: 'Tracker said no.' } }), ev('download_sent'));
		expect(rowExtra(bad, rowStep(bad))).toBe('Tracker said no');
		const ren = one(ev('renamed'));
		expect(rowExtra(ren, rowStep(ren))).toBe('');
		const up = one(ev('file_replaced', { Data: { old_quality: '720p HDTV', new_quality: '1080p WEB-DL' } }));
		expect(rowExtra(up, rowStep(up))).toBe('720p HDTV → 1080p WEB-DL');
		const upHalf = one(ev('file_replaced', { Data: { new_quality: '1080p WEB-DL', release: 'Show.S01E01.2160p.WEB' } }));
		expect(rowExtra(upHalf, rowStep(upHalf))).toBe('4K');
	});

	it('labels the System kinds without calling them routine', () => {
		for (const k of ['signed_in', 'sign_in_failed', 'signed_out', 'api_key_created', 'api_key_deleted', 'settings_changed']) {
			expect(rowStep(one(ev(k))).label).toBe('System');
			expect(isRoutine(k)).toBe(false);
		}
		expect(stepKinds(['system'])).toContain('settings_changed');
	});

	it('says how long ago in a few letters', () => {
		const now = new Date(2026, 10, 2, 12, 0);
		expect(agoShort(new Date(2026, 10, 2, 11, 58).toISOString(), now)).toBe('2m');
		expect(agoShort(new Date(2026, 10, 2, 9, 0).toISOString(), now)).toBe('3h');
		expect(agoShort(new Date(2026, 10, 1, 9, 0).toISOString(), now)).toBe('Yesterday');
		expect(agoShort(new Date(2026, 9, 20).toISOString(), now)).toBe('Oct 20');
	});
});

describe('history detail actions', () => {
	const row = (...events: EventRecord[]): HistoryRow => ({ key: 'r' + events[0].ID, head: events[0], events, steps: [] });
	it('offers Find another only on events tied to an actual release', () => {
		expect(canFind(row(ev('release_chosen', { Data: { release: 'Dune.2021.2160p' } })))).toBe(true);
		expect(canFind(row(ev('download_failed', { DownloadID: 4 })))).toBe(true);
		expect(canFind(row(ev('import_failed', { Data: { download_id: 3 } })))).toBe(true);
		expect(canFind(row(ev('file_replaced', { Data: { release: 'x' } })))).toBe(true);
		expect(canFind(row(ev('release_chosen')))).toBe(false);
		expect(canFind(row(ev('adopted', { Data: { release: 'x' } })))).toBe(false);
		expect(canFind(row(ev('health_broken', { DownloadID: 2 })))).toBe(false);
		expect(canFind(row(ev('ask_made')))).toBe(false);
		expect(canFind(row(ev('title_refreshed')))).toBe(false);
	});
	it('tells routine work apart', () => {
		expect(isRoutine('title_refreshed')).toBe(true);
		expect(isRoutine('import_done')).toBe(false);
		expect(isProblem('download_failed')).toBe(true);
	});
	it('links to the title, the download and Requests for asks', () => {
		const links = rowLinks(row(ev('download_sent', { DownloadID: 7, title_name: 'Dune' })), '/title/movie/1');
		expect(links.map((l) => l.href)).toEqual(['/title/movie/1', '/downloads?open=7']);
		expect(links[0].label).toBe('Dune');
		expect(rowLinks(row(ev('ask_made'))).map((l) => l.key)).toEqual(['downloads', 'requests']);
		expect(rowLinks(row(ev('import_done', { Data: { download_id: 3 } })))[0].href).toBe('/downloads?open=3');
	});
});
