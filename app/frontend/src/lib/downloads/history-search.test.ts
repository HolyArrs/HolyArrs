import { eventText } from './history-lines.ts';
import { dropAiTwins, agoShort, canFind, findTarget, rowLinks, dateLabel, isProblem, isRoutine, rowExtra, rowStep, searchSteps, stepVerb, filterVerb, dirQuery, readSearch, searchChips, stepKinds, textMatches, toggleStep, typeMatches, whoMatches } from './history-search.ts';
import { downloadSteps, type HistoryRow } from './history.ts';
import { describe, expect, it } from 'vitest';
import type { EventRecord } from '../core/types.ts';

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
		expect(stepKinds(['asked', 'sent'])).toEqual(['release_chosen', 'release_chosen_by_hand', 'download_sent', 'download_queued', 'ask_made', 'ai_ask_made']);
		expect(stepKinds(['decide'])).toEqual(['needs_decision', 'stopped_looking']);
		expect(stepKinds(['failed'])).not.toContain('needs_decision');
		expect(readSearch(new URLSearchParams('step=out,chosen,media,added')).steps).toEqual(['added', 'sent', 'imported']);
		expect(stepKinds(['imported'])).toContain('media_server_told');
		expect(stepKinds(['failed'])).toContain('download_failed');
		const all = stepKinds([]);
		expect(all).toContain('import_done');
		expect(all).not.toContain('search_done');
		expect(all).not.toContain('library_scanned');
		expect(all.length).toBeLessThanOrEqual(64);
		for (const k of ['account_changed', 'secrets_imported', 'backup_used']) expect(stepKinds(['system'])).toContain(k);
	});

	it('reads the search look and builds a chip per filter', () => {
		const s = readSearch(new URLSearchParams('look=pill&q=dune&step=failed,asked,bogus&type=movies&who=me&from=2026-10-26'));
		expect(s.steps).toEqual(['failed', 'asked']);
		expect(toggleStep(s.steps, 'failed')).toBe('asked');
		const chips = searchChips(s, new Date('2026-11-02T12:00:00Z'));
		expect(chips.map((c) => c.label)).toEqual(['Failed', 'Asked', 'Movies', 'You', '7 days']);
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
		const decide = one(ev('needs_decision', { Data: { reason: 'nothing could be placed; look at the import plan' } }));
		expect(rowExtra(decide, rowStep(decide))).toBe('Nothing could be placed');
		const gone = one(ev('file_missing', { Data: {} }));
		expect(rowExtra(gone, rowStep(gone))).toBe('File missing');
		const placed = one(ev('import_failed', { Data: {} }));
		expect(rowExtra(placed, rowStep(placed))).toBe('Nothing could be placed');
		const imp = dl(ev('import_done', { Data: { size_bytes: 2_000_000_000 } }), ev('download_sent'));
		expect(rowExtra(imp, rowStep(imp))).toMatch(/GB/);
		const sub = one(ev('subtitle_saved', { Data: { language: 'en' } }));
		expect(rowExtra(sub, rowStep(sub))).toBe('en');
		const bad = dl(ev('download_failed', { Data: { reason: 'Tracker said no.' } }), ev('download_sent'));
		expect(rowExtra(bad, rowStep(bad))).toBe('Tracker said no');
		const ren = one(ev('renamed'));
		expect(rowExtra(ren, rowStep(ren))).toBe('');
		const up = one(ev('file_replaced', { Data: { old_quality: '720p HDTV', new_quality: '1080p WEB-DL' } }));
		expect(rowExtra(up, rowStep(up))).toBe('720p HDTV → 1080p WEB-DL');
		const upHalf = one(ev('file_replaced', { Data: { new_quality: '1080p WEB-DL', release: 'Show.S01E01.2160p.WEB' } }));
		expect(rowExtra(upHalf, rowStep(upHalf))).toBe('4K');
	});

	it('names adds, releases out and turn-downs the way the Filter does', () => {
		expect(rowStep(one(ev('title_added'))).label).toBe('Added');
		expect(rowStep(one(ev('gate_opened'))).label).toBe('Out now');
		expect(rowStep(one(ev('ask_turned_down'))).label).toBe('Turned down');
		for (const s of ['added', 'declined']) expect(searchSteps.some((x) => x.key === s)).toBe(true);
		expect(stepKinds(['added'])).toContain('gate_opened');
		expect(rowStep(one(ev('library_folder_health', { Data: { health: 'ok' } }))).failed).toBe(false);
		expect(rowStep(one(ev('library_folder_health', { Data: { health: 'gone' } }))).failed).toBe(true);
	});

	it('gives a row with no title its own line, and the details the same step', () => {
		const sign = one(ev('signed_in'));
		expect(rowExtra(sign, rowStep(sign), undefined, 'Maya signed in')).toBe('Maya signed in');
		const titled = one(ev('renamed', { TitleID: 4 }));
		expect(rowExtra(titled, rowStep(titled), undefined, 'Renamed a file')).toBe('');
		expect(stepVerb(sign, { word: 'Changed', icon: 'settings', tone: 'idle' })).toEqual({ word: 'System', icon: 'info', tone: 'idle' });
	});

	it('treats AI apps as their own kind of who, with their asks and failures', () => {
		const ai = ev('ai_ask_made', { Actor: { kind: 'ai', id: 3 }, Data: { client: 'Claude' } });
		expect(whoMatches(ai, 'ai')).toBe(true);
		expect(whoMatches(ai, 'holyarrs')).toBe(false);
		expect(whoMatches(ai, 'people')).toBe(false);
		expect(rowStep(one(ai)).label).toBe('Asked');
		expect(rowStep(one(ev('ai_ask_approved'))).label).toBe('Approved');
		expect(rowStep(one(ev('ai_ask_turned_down'))).label).toBe('Turned down');
		const bad = one(ev('ai_action_failed', { Data: { error: 'not allowed for this key' } }));
		expect(rowStep(bad).failed).toBe(true);
		expect(rowExtra(bad, rowStep(bad))).toBe('Not allowed for this key');
		expect(stepKinds([])).toEqual(expect.arrayContaining(['ai_ask_*', 'ai_action_failed']));
		expect(stepKinds(['asked'])).toContain('ai_ask_made');
		expect(textMatches(ai, 'claude')).toBe(true);
	});

	it('shows one AI ask as one row, named and linked', () => {
		const ai = ev('ai_ask_made', { Actor: { kind: 'ai', id: 3 }, Summary: 'AI (Claude, key home) asked for Made for Love (2021) for Maya', Data: { ask_id: 9, client: 'Claude' } });
		const twin = ev('ask_made', { Actor: { kind: 'user', id: 3 }, Data: { ask_id: 9 } });
		const other = ev('ask_made', { Actor: { kind: 'user', id: 4 }, Data: { ask_id: 10 } });
		expect(dropAiTwins([ai, twin, other]).map((e) => e.ID)).toEqual([ai.ID, other.ID]);
		expect(eventText(ai).name).toBe('Made for Love');
		expect(rowLinks(one(ai)).map((l) => l.href)).toEqual(['/requests?ask=9']);
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
	it('finds the release, Slot and title to block for Find another', () => {
		expect(findTarget(row(ev('release_chosen', { TitleID: 7, SlotID: 9, Data: { release_id: 5 } })))).toEqual({ releaseId: 5, slotId: 9, titleId: 7 });
		expect(findTarget(row(ev('download_failed', { TitleID: 7, Data: { release_id: 5 } })))).toEqual({ releaseId: 5, slotId: 0, titleId: 7 });
		expect(findTarget(row(ev('download_failed', { TitleID: 7, DownloadID: 2 })))).toBeNull();
		expect(findTarget(row(ev('release_chosen', { Data: { release_id: 5 } })))).toBeNull();
		expect(findTarget(row(ev('ask_made', { TitleID: 7, Data: { release_id: 5 } })))).toBeNull();
	});
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
		expect(links[0].label).toBe('Title page');
		expect(rowLinks(row(ev('ask_made'))).map((l) => l.key)).toEqual(['requests']);
		expect(rowLinks(row(ev('signed_in')))).toEqual([]);
		expect(rowLinks(row(ev('ask_made', { Data: { ask_id: 12 } })))[0].href).toBe('/requests?ask=12');
		expect(rowLinks(row(ev('import_done', { Data: { download_id: 3 } })))[0].href).toBe('/downloads?open=3');
		const plan = rowLinks(row(ev('needs_decision', { SlotID: 8, title_name: 'Arrival' })), '/library/4');
		expect(plan.map((l) => [l.label, l.href])).toEqual([['Import plan', '/downloads?slot=8'], ['Title page', '/library/4']]);
	});
});

describe('reported problems in History', () => {
	const one = (e: EventRecord): HistoryRow => ({ key: `e${e.ID}`, head: e, events: [e], steps: [] });
	const row = one;
	it('has a step, a filter and a link to the reports', () => {
		expect(rowStep(one(ev('report_made'))).label).toBe('Reported');
		expect(rowStep(one(ev('report_reopened'))).label).toBe('Reported');
		expect(rowStep(one(ev('report_fixed'))).label).toBe('Fixed');
		expect(stepKinds(['reported'])).toEqual(['report_*']);
		expect(stepKinds([])).toContain('report_*');
		expect(rowLinks(row(ev('report_fixed', { Data: { report_id: 4 } }))).map((l) => l.href)).toEqual(['/requests/reports?show=all']);
		expect(rowLinks(row(ev('report_made'))).map((l) => l.label)).toEqual(['Problems']);
		expect(searchSteps.find((s) => s.key === 'reported')?.label).toBe('Reported');
	});
});

describe('row step marks in the Filter words', () => {
	const one = (e: EventRecord): HistoryRow => ({ key: `e${e.ID}`, head: e, events: [e], steps: [] });
	const mark = (r: HistoryRow) => filterVerb(r, eventText(r.head, { titles: new Map() }).tag).word;
	it('renames merged steps and keeps failures', () => {
		expect(mark(one(ev('gate_opened')))).toBe('Added');
		expect(mark(one(ev('import_done')))).toBe('Imported');
		expect(mark(one(ev('report_fixed')))).toBe('Reported');
		expect(mark(one(ev('download_failed')))).toBe('Failed');
		expect(mark(one(ev('library_folder_health', { Data: { health: 'ok' } })))).not.toBe('Failed');
	});
	it('names a grouped download by its furthest step', () => {
		const events = [ev('download_seeding'), ev('import_done'), ev('download_sent')];
		expect(mark({ key: 'd', head: events[0], events, steps: downloadSteps(events) })).toBe('Imported');
	});
});
