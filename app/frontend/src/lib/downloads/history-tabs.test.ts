import { ALL, byDay, dayItems, dayLine, historyTabs, rowTab, showOf, tabKinds, tabOf } from './history-tabs.ts';
import { groupDownloads } from './history.ts';
import { describe, expect, it } from 'vitest';
import type { EventRecord } from '../core/types.ts';

let id = 0;
const ev = (Kind: string, At: string, extra: Partial<EventRecord> = {}): EventRecord => ({ ID: ++id, At, Kind, Summary: Kind, TitleID: null, ...extra });

describe('look tabs', () => {
	it('All asks for every Event; a tab for its subs; a sub for its own kinds', () => {
		expect(tabKinds(ALL)).toBeUndefined();
		expect(tabKinds('came')).toEqual(['import_done', 'import_partly_done', 'file_replaced', 'adopted']);
		expect(tabKinds('came', 'upgraded')).toEqual(['file_replaced']);
		expect(tabKinds('nope')).toBeUndefined();
	});

	it('no kind sits under two big tabs', () => {
		const seen = new Map<string, string>();
		for (const t of historyTabs) for (const s of t.subs) for (const k of s.kinds) {
			expect(seen.get(k), k).toBeUndefined();
			seen.set(k, t.key);
		}
	});

	it('reads and writes &show=', () => {
		expect(showOf('problems.imports')).toEqual({ show: 'problems', sub: 'imports' });
		expect(showOf('problems.zzz')).toEqual({ show: 'problems', sub: '' });
		expect(showOf('zzz')).toEqual({ show: ALL, sub: '' });
		expect(showOf(null)).toEqual({ show: ALL, sub: '' });
	});

	it('files a kind under its tab, prefixes included', () => {
		expect(tabOf('import_done')).toBe('came');
		expect(tabOf('subtitle_saved')).toBe('changes');
		expect(tabOf('library_folder_unreachable')).toBe('problems');
		expect(tabOf('title_refreshed')).toBe('routine');
		expect(tabOf('something_new')).toBe('');
	});
});

describe('rows', () => {
	it('counts a folded download by how far it got', () => {
		const [ok] = groupDownloads([ev('import_done', '2026-10-27T10:00:00Z', { DownloadID: 1 }), ev('download_sent', '2026-10-27T09:00:00Z', { DownloadID: 1 })]);
		const [bad] = groupDownloads([ev('download_failed', '2026-10-27T10:00:00Z', { DownloadID: 2 }), ev('download_sent', '2026-10-27T09:00:00Z', { DownloadID: 2 })]);
		const [going] = groupDownloads([ev('download_sent', '2026-10-27T09:00:00Z', { DownloadID: 3 })]);
		expect(rowTab(ok)).toBe('came');
		expect(rowTab(bad)).toBe('problems');
		expect(rowTab(going)).toBe('changes');
		const [fine] = groupDownloads([ev('library_folder_health', '2026-10-27T09:00:00Z', { Data: { health: 'ok' } })]);
		const [gone] = groupDownloads([ev('library_folder_health', '2026-10-27T09:00:00Z', { Data: { health: 'gone' } })]);
		expect(rowTab(fine)).toBe('changes');
		expect(rowTab(gone)).toBe('problems');
	});

});

describe('days', () => {
	const at = (h: number, d = 27) => new Date(2026, 9, d, h).toISOString();
	it('splits by local day and tallies each', () => {
		const days = byDay([ev('import_done', at(12)), ev('title_refreshed', at(11)), ev('title_refreshed', at(10)), ev('download_failed', at(9, 26))]);
		expect(days.map((d) => d.rows.length)).toEqual([3, 1]);
		expect(dayLine(days[0].tally)).toBe('1 came in · 2 routine');
		expect(dayLine(days[1].tally)).toBe('1 problem');
	});

	it('folds two or more routine rows in a row, keeps a lone one', () => {
		const rows = groupDownloads([ev('title_refreshed', at(12)), ev('folder_scanned', at(11)), ev('import_done', at(10)), ev('search_done', at(9))]);
		const items = dayItems(rows);
		expect(items.map((i) => i.type)).toEqual(['quiet', 'row', 'row']);
		expect(items[0].type === 'quiet' && items[0].rows.length).toBe(2);
	});
});
