import { describe, expect, it } from 'vitest';
import type { DownloadRow } from '../downloaders.ts';
import { pillKind, sortRows, tone } from './layouts.ts';

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

describe('sortRows', () => {
	it('sorts by speed and keeps rows without one last in both directions', () => {
		const rows = [row(1, 'downloading', { speed: 10 }), row(2, 'waiting'), row(3, 'downloading', { speed: 30 })];
		expect(sortRows(rows, 'speed', 'desc', nameOf).map((r) => r.id)).toEqual([3, 1, 2]);
		expect(sortRows(rows, 'speed', 'asc', nameOf).map((r) => r.id)).toEqual([1, 3, 2]);
	});
	it('sorts by state with problems first, then what moves, then what waits', () => {
		const rows = [row(1, 'waiting'), row(2, 'stalled'), row(3, 'downloading'), row(4, 'done'), row(5, 'needs_a_decision'), row(6, 'paused')];
		expect(sortRows(rows, 'state', 'asc', nameOf).map((r) => r.id)).toEqual([5, 2, 6, 3, 1, 4]);
	});
	it('sorts by title with numbers in order', () => {
		const rows = [row(1, 'done', { title: 'Show 10' }), row(2, 'done', { title: 'Show 9' })];
		expect(sortRows(rows, 'title', 'asc', nameOf).map((r) => r.id)).toEqual([2, 1]);
	});
});

describe('pillKind and tone', () => {
	it('colour a row by where it is', () => {
		expect(pillKind({ phase: 'downloading' })).toBe('downloading');
		expect(pillKind({ phase: 'stalled' })).toBe('failed');
		expect(pillKind({ phase: 'paused' })).toBe('turned_down');
		expect(pillKind({ phase: 'waiting' })).toBe('missing');
		expect(pillKind({ phase: 'done' })).toBe('in_library');
		expect(tone({ phase: 'importing' })).toBe('info');
		expect(tone({ phase: 'failed' })).toBe('fail');
		expect(tone({ phase: 'removed' })).toBe('idle');
	});
});
