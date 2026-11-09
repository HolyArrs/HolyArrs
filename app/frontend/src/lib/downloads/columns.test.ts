import { COLUMNS, IDS, defaults, dump, load, move, moveVisible, normalize, quality, ratioText, shortAge, swarmNum, toggle, visible } from './columns.ts';
import { describe, expect, it } from 'vitest';

describe('columns', () => {
	it('defaults show seven columns in order', () => {
		expect(visible(defaults())).toEqual(['poster', 'title', 'proto', 'progress', 'status', 'size', 'requested']);
		expect(defaults().widths).toMatchObject({ title: 310, progress: 130, status: 120, size: 60, quality: 80 });
		expect(COLUMNS.find((c) => c.id === 'proto')?.label).toBe('Downloader');
	});
	it('offers every column now that the Downloaders send them all', () => {
		const l = defaults();
		l.hidden = [];
		expect(visible(l)).toContain('added');
		expect(COLUMNS.filter((c) => c.missing)).toEqual([]);
		expect(COLUMNS.find((c) => c.id === 'source')?.label).toBe('Source');
	});
	it('reads a post age short', () => {
		const now = Date.parse('2026-11-02T12:00:00Z');
		expect(shortAge('2026-11-02T07:00:00Z', now)).toBe('5 h');
		expect(shortAge('2026-10-30T12:00:00Z', now)).toBe('3 d');
		expect(shortAge('2024-10-01T12:00:00Z', now)).toBe('2 y');
		expect(shortAge(undefined, now)).toBe('');
		expect(shortAge('nope', now)).toBe('');
	});
	it('keeps swarm numbers and ratios to four characters', () => {
		expect(swarmNum(9999)).toBe('9999');
		expect(swarmNum(12345)).toBe('12k');
		expect(swarmNum(undefined)).toBe('0');
		expect(ratioText(0.5123)).toBe('0.51');
		expect(ratioText(99)).toBe('99.0');
		expect(ratioText(123.4)).toBe('123');
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
		expect(visible(back)).toEqual(['poster', 'status', 'title', 'proto', 'progress', 'requested']);
		expect(back.widths.title).toBe(400);
		back.hidden = toggle(back, 'size');
		expect(visible(back)).toContain('size');
	});
	it('carries over the first saved shape (widths only)', () => {
		const l = load(null, JSON.stringify({ title: 500, bar: 90, st: 140, size: 120, time: 70 }));
		expect(l.widths).toMatchObject({ title: 500, progress: 90, status: 140, size: 120, quality: 80 });
		expect(visible(l)).toEqual(visible(defaults()));
	});
	it('carries a version 2 layout over, swapping its old default widths for the new ones', () => {
		const old = JSON.stringify({ v: 2, order: IDS, hidden: ['kind'], widths: { size: 100, status: 170, title: 400 }, sort: { id: 'size', desc: true } });
		const l = load(old, null);
		expect(l.widths).toMatchObject({ size: 60, status: 170, title: 400, progress: 130 });
		expect(l.hidden).toContain('kind');
		expect(l.sort).toEqual({ id: 'size', desc: true });
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
		expect(nextSort(undefined, 'swarm')).toEqual({ id: 'swarm', desc: true, facet: 'seeds' });
		expect(nextSort({ id: 'swarm', desc: true, facet: 'seeds' }, 'swarm', 'ratio')).toEqual({ id: 'swarm', desc: true, facet: 'ratio' });
		expect(nextSort({ id: 'swarm', desc: true, facet: 'ratio' }, 'swarm', 'ratio')).toEqual({ id: 'swarm', desc: false, facet: 'ratio' });
		expect(nextSort({ id: 'title', desc: true }, 'size')).toEqual({ id: 'size', desc: true });
	});
	it('sorts numbers and words with empties last', () => {
		const rows = [{ v: 2 }, { v: undefined }, { v: 10 }, { v: 1 }];
		expect(sortRows(rows, (r) => r.v, false).map((r) => r.v)).toEqual([1, 2, 10, undefined]);
		expect(sortRows(rows, (r) => r.v, true).map((r) => r.v)).toEqual([10, 2, 1, undefined]);
		expect(sortRows([{ v: 'b' }, { v: '' }, { v: 'A' }], (r) => r.v, false).map((r) => r.v)).toEqual(['A', 'b', '']);
	});
	it('keeps the sort in the saved layout and optional columns hidden', () => {
		const l = load(dump({ ...defaults(), sort: { id: 'swarm', desc: true, facet: 'ratio' } }), null);
		expect(l.sort).toEqual({ id: 'swarm', desc: true, facet: 'ratio' });
		expect(load(JSON.stringify({ v: VERSION, sort: { id: 'nope' } }), null).sort).toBeUndefined();
		const d = defaults();
		expect(d.hidden).toEqual(expect.arrayContaining(['kind', 'swarm', 'tracker', 'added', 'quality']));
	});
});

describe('columns added later', () => {
	it('start hidden in an older saved layout, which keeps its own choices', () => {
		const old = JSON.stringify({ v: VERSION, order: IDS.filter((id) => !['swarm', 'tracker'].includes(id)), hidden: ['added'], widths: {} });
		const l = load(old, null);
		expect(l.hidden).toEqual(expect.arrayContaining(['swarm', 'tracker', 'added']));
		expect(visible(l)).toContain('kind');
	});
});
