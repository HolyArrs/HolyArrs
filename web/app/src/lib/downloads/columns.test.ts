import { describe, expect, it } from 'vitest';
import { COLUMNS, IDS, defaults, dump, load, move, moveVisible, normalize, quality, shortAge, toggle, visible } from './columns.ts';
import { stepToward } from './sections.ts';

describe('columns', () => {
	it('defaults show eight columns in order', () => {
		expect(visible(defaults())).toEqual(['poster', 'kind', 'title', 'proto', 'progress', 'status', 'size', 'requested']);
		expect(defaults().widths).toMatchObject({ title: 310, progress: 120, status: 150, size: 100, quality: 90 });
		expect(COLUMNS.find((c) => c.id === 'proto')?.label).toBe('Downloader');
	});
	it('offers every column now that the Downloaders send them all', () => {
		const l = defaults();
		l.hidden = [];
		expect(visible(l)).toContain('age');
		expect(COLUMNS.filter((c) => c.missing)).toEqual([]);
		expect(COLUMNS.find((c) => c.id === 'source')?.label).toBe('Found on');
	});
	it('reads a post age short', () => {
		const now = Date.parse('2026-11-02T12:00:00Z');
		expect(shortAge('2026-11-02T07:00:00Z', now)).toBe('5 h');
		expect(shortAge('2026-10-30T12:00:00Z', now)).toBe('3 d');
		expect(shortAge('2024-10-01T12:00:00Z', now)).toBe('2 y');
		expect(shortAge(undefined, now)).toBe('');
		expect(shortAge('nope', now)).toBe('');
	});
	it('offers seeds and peers as one optional Swarm column', () => {
		expect(visible(defaults())).not.toContain('swarm');
		const l = defaults();
		l.hidden = toggle(l, 'swarm');
		expect(visible(l)).toContain('swarm');
	});
	it('saves and loads order, visibility and widths', () => {
		const l = defaults();
		l.order = moveVisible(l, 'status', 1);
		l.hidden = toggle(l, 'size');
		l.widths.title = 400;
		const back = load(dump(l), null);
		expect(visible(back)).toEqual(['poster', 'status', 'kind', 'title', 'proto', 'progress', 'requested']);
		expect(back.widths.title).toBe(400);
		back.hidden = toggle(back, 'size');
		expect(visible(back)).toContain('size');
	});
	it('carries over the first saved shape (widths only)', () => {
		const l = load(null, JSON.stringify({ title: 500, bar: 90, st: 140, size: 120, time: 70 }));
		expect(l.widths).toMatchObject({ title: 500, progress: 90, status: 140, size: 120, quality: 90 });
		expect(visible(l)).toEqual(visible(defaults()));
	});
	it('ignores junk and unknown versions', () => {
		expect(load('{bad', null)).toEqual(defaults());
		expect(load(JSON.stringify({ v: 9, order: ['size'] }), null)).toEqual(defaults());
	});
	it('normalize drops unknown ids and adds new ones in their default spot', () => {
		const n = normalize({ order: ['size', 'nope', 'title'] as never, hidden: [], widths: { title: 99999 } as never });
		expect(n.order.length).toBe(IDS.length);
		expect(n.order.indexOf('size')).toBeLessThan(n.order.indexOf('title'));
		expect(n.order.indexOf('proto')).toBe(n.order.indexOf('title') + 1);
		expect(n.widths.title).toBe(3000);
	});
	it('move and moveVisible', () => {
		expect(move(['title', 'progress', 'status'] as never, 'status', 0)).toEqual(['status', 'title', 'progress']);
		const l = defaults();
		expect(visible({ ...l, order: moveVisible(l, 'title', 99) }).at(-1)).toBe('title');
	});
	it('reads quality from a Release name', () => {
		expect(quality('Movie.2023.2160p.UHD.BluRay.HDR10.x265')).toBe('2160p HDR');
		expect(quality('Show.S01E01.1080p.WEB-DL.DV.H.265')).toBe('1080p DV');
		expect(quality('Show.S01E01.720p.HDTV')).toBe('720p');
		expect(quality('Artist - Album (2020) FLAC')).toBe('');
	});
	it('stepToward eases to the newest reading', () => {
		expect(stepToward(0, 1000000)).toBe(600000);
		expect(stepToward(999500, 1000000)).toBe(1000000);
	});
});

describe('Title width', () => {
	it('is only kept once the Title itself is sized', () => {
		const l = defaults();
		expect(l.titleSet).toBeFalsy();
		l.widths.size = 200;
		expect(load(dump(l), null).titleSet).toBe(false);
		expect(load(dump({ ...l, titleSet: true }), null).titleSet).toBe(true);
		expect(load(null, JSON.stringify({ title: 500 })).widths.title).toBe(500);
	});
});

import { VERSION, nextSort, sortRows } from './columns.ts';
describe('sorting', () => {
	it('cycles a column: first way, flipped, cleared', () => {
		expect(nextSort(undefined, 'title')).toEqual({ id: 'title', desc: false });
		expect(nextSort({ id: 'title', desc: false }, 'title')).toEqual({ id: 'title', desc: true });
		expect(nextSort({ id: 'title', desc: true }, 'title')).toBeUndefined();
		expect(nextSort(undefined, 'ratio')).toEqual({ id: 'ratio', desc: true });
		expect(nextSort({ id: 'title', desc: true }, 'size')).toEqual({ id: 'size', desc: true });
	});
	it('sorts numbers and words with empties last', () => {
		const rows = [{ v: 2 }, { v: undefined }, { v: 10 }, { v: 1 }];
		expect(sortRows(rows, (r) => r.v, false).map((r) => r.v)).toEqual([1, 2, 10, undefined]);
		expect(sortRows(rows, (r) => r.v, true).map((r) => r.v)).toEqual([10, 2, 1, undefined]);
		expect(sortRows([{ v: 'b' }, { v: '' }, { v: 'A' }], (r) => r.v, false).map((r) => r.v)).toEqual(['A', 'b', '']);
	});
	it('keeps the sort in the saved layout and optional columns hidden', () => {
		const l = load(dump({ ...defaults(), sort: { id: 'ratio', desc: true } }), null);
		expect(l.sort).toEqual({ id: 'ratio', desc: true });
		expect(load(JSON.stringify({ v: VERSION, sort: { id: 'nope' } }), null).sort).toBeUndefined();
		const d = defaults();
		expect(d.hidden).toEqual(expect.arrayContaining(['swarm', 'queue', 'tracker', 'added', 'ratio', 'quality']));
	});
});

describe('columns added later', () => {
	it('start hidden in an older saved layout, which keeps its own choices', () => {
		const old = JSON.stringify({ v: VERSION, order: IDS.filter((id) => !['swarm', 'queue', 'tracker'].includes(id)), hidden: ['added'], widths: {} });
		const l = load(old, null);
		expect(l.hidden).toEqual(expect.arrayContaining(['swarm', 'queue', 'tracker', 'added']));
		expect(visible(l)).toContain('ratio');
	});
});
