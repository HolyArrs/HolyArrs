import { createLatestOnly, dateRangeProblem, details, downloadSteps, groupDownloads, groupLabel, historyQuery, isDownloadStep, kindGroups, kindTone, pageLine, readFilter } from './history.ts';
import { describe, expect, it } from 'vitest';
import type { EventRecord } from '../core/types.ts';

describe('history', () => {
	it('rejects a date range whose end is before its start', () => {
		expect(dateRangeProblem({ from: '2026-10-17', to: '2026-10-01' })).toBe('From date must be on or before To date.');
		expect(dateRangeProblem({ from: '2026-10-01', to: '2026-10-17' })).toBe('');
		expect(() => historyQuery({ groups: [], from: '2026-10-17', to: '2026-10-01', limit: 50, offset: 0 })).toThrow(
			'From date must be on or before To date.'
		);
	});

	it('lets only the newest overlapping request update history', async () => {
		const begin = createLatestOnly();
		let shown = '';
		const deferred = () => {
			let resolve!: (value: string) => void;
			const promise = new Promise<string>((done) => (resolve = done));
			return { promise, resolve };
		};
		const finish = async (request: Promise<string>) => {
			const isLatest = begin();
			const value = await request;
			if (isLatest()) shown = value;
		};
		const first = deferred();
		const second = deferred();
		const firstDone = finish(first.promise);
		const secondDone = finish(second.promise);
		second.resolve('newest');
		await secondDone;
		first.resolve('stale');
		await firstDone;
		expect(shown).toBe('newest');
	});

	it('names the group of every kind the loop writes', () => {
		for (const [kind, label] of [
			['title_added', 'Added'],
			['search_done', 'Searched'],
			['release_held', 'Held'],
			['release_chosen_by_hand', 'Chosen'],
			['download_sent', 'Sent to download'],
			['download_downloading', 'Downloading'],
			['download_failed', 'Failed'],
			['import_done', 'Imported'],
			['file_replaced', 'Imported'],
			['renamed', 'Renamed'],
			['title_removed', 'Deleted'],
			['library_rescanned', 'Scans'],
			['library_folder_unreachable', 'Scans'],
			['health_broken', 'Health'],
			['needs_decision', 'Needs a decision'],
			['ask_made', 'Requests'],
			['report_fixed', 'Reported problems'],
			['media_server_told', 'Alerts and media servers'],
			['something_new', 'Other']
		]) {
			expect(groupLabel(kind), kind).toBe(label);
		}
		expect(new Set(kindGroups.map((g) => g.key)).size).toBe(kindGroups.length);
	});

	it('colours rows by what happened', () => {
		expect(kindTone('download_failed')).toBe('fail');
		expect(kindTone('health_broken')).toBe('fail');
		expect(kindTone('health_recovered')).toBe('ok');
		expect(kindTone('import_done')).toBe('ok');
		expect(kindTone('release_held')).toBe('wait');
		expect(kindTone('renamed')).toBe('');
	});

	it('builds the API query from the filters', () => {
		const q = new URLSearchParams(historyQuery({ title: 7, groups: ['rename', 'health'], from: '2026-10-01', to: '2026-10-31', limit: 50, offset: 100 }));
		expect(q.get('title')).toBe('7');
		expect(q.get('kind')).toBe('renamed,rename_planned,health_*');
		expect(q.get('from')).toBe('2026-10-01');
		expect(q.get('to')).toBe('2026-10-31');
		expect(q.get('limit')).toBe('50');
		expect(q.get('offset')).toBe('100');
		expect(historyQuery({ groups: [], limit: 50, offset: 0 })).toBe('limit=50');
	});

	it('reads the filters from the page address and writes them back', () => {
		const f = readFilter(new URLSearchParams('title=3&kinds=grab,bogus,import&from=2026-10-01&to=nope&offset=50'));
		expect(f).toEqual({ title: 3, groups: ['grab', 'import'], from: '2026-10-01', to: undefined, limit: 50, offset: 50 });
		expect(readFilter(new URLSearchParams('from=2026-99-99&to=2026-02-30')).from).toBeUndefined();
		expect(readFilter(new URLSearchParams('from=2026-99-99&to=2026-02-30')).to).toBeUndefined();
		expect(readFilter(new URLSearchParams('from=2024-02-29')).from).toBe('2024-02-29');
	});

	it('carries the Source pick and the feed checks switch', () => {
		const f = readFilter(new URLSearchParams('source=4&feed=1'));
		expect(f.source).toBe(4);
		expect(f.feed).toBe(true);
		const q = new URLSearchParams(historyQuery(f));
		expect(q.get('source')).toBe('4');
		expect(q.get('feed')).toBe('1');
		const none = readFilter(new URLSearchParams('source=x&feed=0'));
		expect(none.source).toBeUndefined();
		expect(none.feed).toBeUndefined();
		expect(historyQuery(none)).toBe('limit=50');
	});

	it('shows reasons and paths', () => {
		expect(
			details({ Data: { reason: 'nothing could be placed', release: 'Arrival.2016.1080p', paths: [{ to: '/movies/Arrival (2016)/Arrival (2016).mkv' }], state: 'x' } })
		).toEqual([
			['Why', 'nothing could be placed'],
			['Release', 'Arrival.2016.1080p'],
			['File', '/movies/Arrival (2016)/Arrival (2016).mkv']
		]);
		expect(details({ Data: undefined })).toEqual([]);
	});

	it('says which rows are shown', () => {
		expect(pageLine(0, 0, 0)).toBe('No Events');
		expect(pageLine(120, 50, 50)).toBe('51 to 100 of 120');
	});
});

describe('history download rows', () => {
	const ev = (ID: number, Kind: string, DownloadID?: number): EventRecord => ({ ID, At: `2026-10-1${ID % 10}T10:00:00Z`, Kind, Summary: Kind, TitleID: 1, DownloadID });

	it('folds the steps of one download into one row where its newest Event is', () => {
		const rows = groupDownloads([ev(9, 'import_done', 4), ev(8, 'renamed'), ev(7, 'download_finished', 4), ev(6, 'download_sent', 4), ev(5, 'download_sent', 5)]);
		expect(rows.map((r) => r.key)).toEqual(['d4', 'e8', 'd5']);
		expect(rows[0].head.ID).toBe(9);
		expect(rows[0].events.map((e) => e.ID)).toEqual([9, 7, 6]);
		expect(rows[0].steps.map((s) => s.state)).toEqual(['done', 'done', 'done']);
		expect(rows[1].steps).toEqual([]);
		expect(rows[2].steps.map((s) => s.state)).toEqual(['done', 'waiting', 'waiting']);
	});

	it('marks where a download failed', () => {
		expect(downloadSteps([ev(2, 'download_failed', 1), ev(1, 'download_sent', 1)]).map((s) => s.state)).toEqual(['done', 'failed', 'waiting']);
		expect(downloadSteps([ev(3, 'import_rejected', 1), ev(2, 'download_finished', 1)]).map((s) => s.state)).toEqual(['done', 'done', 'failed']);
		expect(downloadSteps([ev(4, 'import_done', 1), ev(3, 'import_failed', 1)]).map((s) => s.state)).toEqual(['done', 'done', 'done']);
	});

	it('leaves Events without a download, or that are not download steps, alone', () => {
		expect(isDownloadStep(ev(1, 'download_sent'))).toBe(false);
		expect(isDownloadStep(ev(1, 'renamed', 3))).toBe(false);
		expect(isDownloadStep(ev(1, 'download_vanished', 3))).toBe(true);
		const rows = groupDownloads([ev(2, 'search_done'), ev(1, 'search_done')]);
		expect(rows).toHaveLength(2);
	});
	it('folds the pick that started a download, and the decision it waits on, into its row', () => {
		const at = (e: EventRecord, x: Partial<EventRecord>) => ({ ...e, ...x });
		const rows = groupDownloads([
			at(ev(6, 'needs_decision'), { SlotID: 8 }),
			at(ev(5, 'import_failed', 4), { SlotID: 8 }),
			at(ev(4, 'download_sent', 4), { Data: { release_id: 70 } }),
			at(ev(3, 'release_chosen'), { Data: { release_id: 70 } }),
			at(ev(2, 'release_chosen'), { Data: { release_id: 99 } })
		]);
		expect(rows.map((r) => [r.key, r.events.length])).toEqual([['d4', 4], ['e2', 1]]);
		expect(rows[0].head.Kind).toBe('needs_decision');
	});
});
