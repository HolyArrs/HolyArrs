import { describe, expect, it } from 'vitest';
import { actsOf, agoShort, rowActs, rowLinks, dateLabel, isProblem, rowExtra, rowStep, dirQuery, lookOf, readDir, readSearch, searchChips, stepKinds, storyOf, textMatches, titleCards, toggleStep, typeMatches, whoMatches } from './dirs.ts';
import type { EventRecord } from '../types.ts';
import { downloadSteps, type HistoryRow } from '../history.ts';

let id = 0;
const ev = (Kind: string, extra: Partial<EventRecord> = {}): EventRecord => ({ ID: ++id, At: '2026-11-01T10:00:00Z', Kind, Summary: Kind, TitleID: null, ...extra });

describe('history preview looks', () => {
	it('reads the look, falling back to today', () => {
		expect(lookOf('table')).toBe('table');
		expect(lookOf('nope')).toBe('plain');
		expect(lookOf(null)).toBe('plain');
	});

	it('keeps ?look= and drops empty values when the address changes', () => {
		const q = new URLSearchParams('look=feed&title=4&offset=50');
		expect(dirQuery(q, { title: undefined, who: 'me' })).toBe('?look=feed&who=me');
		expect(readDir(new URLSearchParams('show=problems.imports&who=people')).who).toBe('people');
		expect(readDir(new URLSearchParams('show=problems.imports')).sub).toBe('imports');
	});

	it('tells who made an Event happen', () => {
		const mine = ev('ask_made', { Actor: { kind: 'user', id: 2 } });
		expect(whoMatches(mine, 'me', 2)).toBe(true);
		expect(whoMatches(mine, 'me', 3)).toBe(false);
		expect(whoMatches(mine, 'holyarrs')).toBe(false);
		expect(whoMatches(ev('search_done', { Actor: { kind: 'job' } }), 'holyarrs')).toBe(true);
	});

	it('lays out a story; a newer success clears an older failure', () => {
		const s = storyOf([ev('import_done'), ev('import_failed'), ev('download_failed'), ev('ask_made')]);
		const by = Object.fromEntries(s.map((x) => [x.key, x.state]));
		expect(by.imported).toBe('done');
		expect(by.downloaded).toBe('failed');
		expect(by.requested).toBe('done');
		expect(by.upgraded).toBe('waiting');
	});

	it('folds Events into one card per title, newest first', () => {
		const cards = titleCards([ev('import_done', { TitleID: 1, title_name: 'Dune' }), ev('download_sent', { TitleID: 1 }), ev('import_failed', { TitleID: 2, title_name: 'Alien' }), ev('health_broken')]);
		expect(cards.map((c) => [c.name, c.count, c.problem])).toEqual([['Dune', 2, false], ['Alien', 1, true]]);
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
	});

	it('says how long ago in a few letters', () => {
		const now = new Date(2026, 10, 2, 12, 0);
		expect(agoShort(new Date(2026, 10, 2, 11, 58).toISOString(), now)).toBe('2m');
		expect(agoShort(new Date(2026, 10, 2, 9, 0).toISOString(), now)).toBe('3h');
		expect(agoShort(new Date(2026, 10, 1, 9, 0).toISOString(), now)).toBe('Yesterday');
		expect(agoShort(new Date(2026, 9, 20).toISOString(), now)).toBe('Oct 20');
	});
});

describe('history detail actions (?acts=)', () => {
	const row = (...events: EventRecord[]): HistoryRow => ({ key: 'r' + events[0].ID, head: events[0], events, steps: [] });
	it('reads the version, falling back to find', () => {
		expect(actsOf('retry')).toBe('retry');
		expect(actsOf('view')).toBe('view');
		expect(actsOf('x')).toBe('find');
		expect(actsOf(null)).toBe('find');
	});
	it('offers Find another on release events, Retry first on failures, nothing in view', () => {
		const failed = row(ev('download_failed'));
		expect(rowActs(failed, 'find')).toEqual(['find']);
		expect(rowActs(failed, 'retry')).toEqual(['retry', 'find']);
		expect(rowActs(failed, 'view')).toEqual([]);
		expect(rowActs(row(ev('release_chosen')), 'retry')).toEqual(['find']);
		expect(rowActs(row(ev('ask_made')), 'find')).toEqual([]);
		expect(rowActs(row(ev('search_failed')), 'retry')).toEqual(isProblem('search_failed') ? ['retry'] : []);
	});
	it('links to the title, the download and Requests for asks', () => {
		const links = rowLinks(row(ev('download_sent', { DownloadID: 7, title_name: 'Dune' })), '/title/movie/1');
		expect(links.map((l) => l.href)).toEqual(['/title/movie/1', '/downloads?open=7']);
		expect(links[0].label).toBe('Dune');
		expect(rowLinks(row(ev('ask_made'))).map((l) => l.key)).toEqual(['downloads', 'requests']);
		expect(rowLinks(row(ev('import_done', { Data: { download_id: 3 } })))[0].href).toBe('/downloads?open=3');
	});
});
