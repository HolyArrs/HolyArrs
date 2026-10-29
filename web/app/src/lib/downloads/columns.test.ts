import { describe, expect, it } from 'vitest';
import { COLUMNS, IDS, defaults, fitTitle, dump, load, move, moveVisible, normalize, quality, toggle, visible } from './columns.ts';
import { stepToward } from './sections.ts';

describe('columns', () => {
	it('defaults show ten columns in order', () => {
		expect(visible(defaults())).toEqual(['poster', 'kind', 'title', 'proto', 'progress', 'status', 'speed', 'time', 'size', 'quality']);
		expect(defaults().widths).toMatchObject({ title: 310, progress: 66, status: 130, size: 130, time: 80, speed: 90, quality: 90 });
	});
	it('never shows a column the Downloaders do not send', () => {
		const l = defaults();
		l.hidden = [];
		expect(visible(l)).not.toContain('ratio');
		expect(COLUMNS.filter((c) => c.missing).map((c) => c.id)).toEqual(['age', 'ratio', 'requested', 'source', 'category']);
	});
	it('offers Seeds and Peers as optional torrent columns', () => {
		expect(visible(defaults())).not.toContain('seeds');
		const l = defaults();
		l.hidden = toggle({ ...l, hidden: toggle(l, 'seeds') }, 'peers');
		expect(visible(l)).toEqual(expect.arrayContaining(['seeds', 'peers']));
	});
	it('saves and loads order, visibility and widths', () => {
		const l = defaults();
		l.order = moveVisible(l, 'status', 1);
		l.hidden = toggle(l, 'quality');
		l.widths.title = 400;
		const back = load(dump(l), null);
		expect(visible(back)).toEqual(['poster', 'status', 'kind', 'title', 'proto', 'progress', 'speed', 'time', 'size']);
		expect(back.widths.title).toBe(400);
		back.hidden = toggle(back, 'quality');
		expect(visible(back)).toContain('quality');
	});
	it('carries over the first saved shape (widths only)', () => {
		const l = load(null, JSON.stringify({ title: 500, bar: 90, st: 140, size: 120, time: 70 }));
		expect(l.widths).toMatchObject({ title: 500, progress: 90, status: 140, size: 120, time: 70, speed: 90 });
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

describe('fitTitle', () => {
	it('gives way while unset, never under the minimum, and not once set', () => {
		const l = defaults();
		const others = 52 + 84 + 64 + 66 + 130 + 90 + 80 + 130 + 90;
		expect(fitTitle(l, 2000, 0, 44)).toBe(310);
		expect(fitTitle(l, others + 200, 0, 44)).toBe(200);
		expect(fitTitle(l, others + 10, 0, 44)).toBe(44);
		expect(fitTitle({ ...l, titleSet: true }, others + 200, 0, 44)).toBe(310);
		expect(load(dump({ ...l, titleSet: true }), null).titleSet).toBe(true);
		expect(load(null, JSON.stringify({ title: 500 })).titleSet).toBe(true);
	});
});
