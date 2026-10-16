import { describe, expect, it } from 'vitest';
import { details, filterParams, groupLabel, historyQuery, kindGroups, kindTone, pageLine, readFilter } from './history.ts';

describe('history', () => {
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
			['ask_made', 'Wishlist'],
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
		expect(filterParams(f)).toBe('?title=3&kinds=grab%2Cimport&from=2026-10-01&offset=50');
		expect(filterParams(readFilter(new URLSearchParams('title=-1')))).toBe('');
	});

	it('shows reasons and paths', () => {
		expect(
			details({ Data: { reason: 'nothing could be placed', release: 'Arrival.2016.1080p', paths: [{ to: '/movies/Arrival (2016)/Arrival (2016).mkv' }], state: 'x' } })
		).toEqual([
			['Why', 'nothing could be placed'],
			['Release', 'Arrival.2016.1080p'],
			['Placed at', '/movies/Arrival (2016)/Arrival (2016).mkv']
		]);
		expect(details({ Data: undefined })).toEqual([]);
	});

	it('says which rows are shown', () => {
		expect(pageLine(0, 0, 0)).toBe('No Events');
		expect(pageLine(120, 50, 50)).toBe('51 to 100 of 120');
	});
});
