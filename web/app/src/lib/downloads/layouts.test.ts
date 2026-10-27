import { describe, expect, it } from 'vitest';
import type { DownloadRow } from '../downloaders.ts';
import { doneToday, finished, groupByTitle, lanes, note, pillKind, queue, sortRows } from './layouts.ts';

const row = (id: number, phase: string, more: Partial<DownloadRow> = {}): DownloadRow => ({
	id,
	phase,
	state: phase,
	label: phase,
	text: '',
	progress: 0,
	speed: 0,
	active: true,
	name: `Release.${id}`,
	title: `Title ${id}`,
	version: 'Main',
	downloader: 'qBittorrent',
	size: 0,
	updated_at: '2026-10-21T19:05:24Z',
	can_act: true,
	...more
});
const nameOf = (r: DownloadRow) => r.title;

describe('note', () => {
	it('drops a message that only repeats the percentage', () => {
		expect(note({ text: '18% done.' })).toBeNull();
		expect(note({ text: '55%' })).toBeNull();
		expect(note({ text: '' })).toBeNull();
	});
	it('keeps a message that explains something', () => {
		expect(note({ text: 'No progress for 9 hours.' })).toBe('No progress for 9 hours.');
		expect(note({ text: 'Someone paused it in the Downloader. HolyArrs waits.' })).toContain('paused');
	});
});

describe('queue', () => {
	it('puts problems first, then what moves, then what waits, and leaves out finished rows', () => {
		const rows = [row(1, 'waiting'), row(2, 'downloading', { progress: 0.2 }), row(3, 'stalled'), row(4, 'done'), row(5, 'failed'), row(6, 'downloading', { progress: 0.6 }), row(7, 'paused'), row(8, 'needs_a_decision')];
		expect(queue(rows).map((r) => r.id)).toEqual([8, 5, 3, 7, 6, 2, 1]);
	});
});

describe('lanes', () => {
	it('splits rows into downloading, waiting, attention and done', () => {
		const l = lanes([row(1, 'waiting'), row(2, 'downloading'), row(3, 'stalled'), row(4, 'done'), row(5, 'paused'), row(6, 'removed'), row(7, 'importing')]);
		expect(l.downloading.map((r) => r.id).sort()).toEqual([2, 7]);
		expect(l.waiting.map((r) => r.id)).toEqual([1]);
		expect(l.attention.map((r) => r.id)).toEqual([3, 5]);
		expect(l.done.map((r) => r.id)).toEqual([4]);
	});
	it('finished is newest first; doneToday keeps the last 24 hours', () => {
		const a = row(1, 'done', { finished_at: '2026-10-20T10:00:00Z' });
		const b = row(2, 'done', { finished_at: '2026-10-21T10:00:00Z' });
		expect(finished([a, b]).map((r) => r.id)).toEqual([2, 1]);
		expect(doneToday([a, b], Date.parse('2026-10-21T20:00:00Z')).map((r) => r.id)).toEqual([2]);
	});
});

describe('groupByTitle', () => {
	it('puts the episodes of one show together, in episode order, where the first one was', () => {
		const rows = [row(1, 'downloading', { title_id: 9, covers: 'S02E04' }), row(2, 'stalled', { title_id: 5 }), row(3, 'downloading', { title_id: 9, covers: 'S02E03' })];
		const g = groupByTitle(rows, nameOf);
		expect(g.map((x) => x.key)).toEqual(['t9', 't5']);
		expect(g[0].rows.map((r) => r.covers)).toEqual(['S02E03', 'S02E04']);
	});
});

describe('sortRows', () => {
	it('sorts by speed and keeps rows without one last in both directions', () => {
		const rows = [row(1, 'downloading', { speed: 10 }), row(2, 'waiting'), row(3, 'downloading', { speed: 30 })];
		expect(sortRows(rows, 'speed', 'desc', nameOf).map((r) => r.id)).toEqual([3, 1, 2]);
		expect(sortRows(rows, 'speed', 'asc', nameOf).map((r) => r.id)).toEqual([1, 3, 2]);
	});
	it('sorts by state with problems first', () => {
		const rows = [row(1, 'waiting'), row(2, 'stalled'), row(3, 'downloading')];
		expect(sortRows(rows, 'state', 'asc', nameOf).map((r) => r.id)).toEqual([2, 3, 1]);
	});
});

describe('pillKind', () => {
	it('colours a row by where it is', () => {
		expect(pillKind({ phase: 'downloading' })).toBe('downloading');
		expect(pillKind({ phase: 'stalled' })).toBe('failed');
		expect(pillKind({ phase: 'paused' })).toBe('turned_down');
		expect(pillKind({ phase: 'waiting' })).toBe('missing');
		expect(pillKind({ phase: 'done' })).toBe('in_library');
	});
});
