import { describe, expect, it } from 'vitest';
import type { TitleView } from '../core/types.ts';
import type { Column } from './table.ts';
import { buildEntries, cell, rowCell, tone } from './library-view.ts';

const t = (id: number) => ({ id, kind: 'movie', slots: [] }) as unknown as TitleView;
const col = (id: string) => ({ id, width: '150px' }) as unknown as Column;

describe('buildEntries', () => {
	it('groups by series, sorts headings, loose last', () => {
		const m = new Map([[1, 20], [2, 10]]);
		const name = (id: number) => (id === 10 ? 'Alien' : 'Zorro');
		const e = buildEntries([t(1), t(2), t(3)], true, m, name);
		expect(e.map((x) => x.key)).toEqual(['h10', 'r2', 'h20', 'r1', 'h0', 'r3']);
		expect(e[4]).toMatchObject({ name: 'Not in a series', count: 1 });
		expect(new Set(e.map((x) => x.key)).size).toBe(e.length);
		expect(buildEntries([t(1)], false, m, name)).toEqual([{ type: 'row', key: 'r1', t: t(1) }]);
		expect(buildEntries([t(1)], true, m, name).some((x) => x.key === 'h0')).toBe(false);
	});
});

describe('rowCell / cell', () => {
	it('maps column ids to text', () => {
		const m = { id: 1, kind: 'movie', slots: [], year: 1999, keep_up_to_date: true, runtime_minutes: 125, digital_date: '2999-01-01' } as unknown as TitleView;
		expect(cell(m, col('year'), '')).toBe('1999');
		expect(cell(m, col('runtime'), '')).toBe('2h 5m');
		expect(cell(m, col('kept'), '')).toBe('Yes');
		expect(cell(m, col('release'), '')).toMatch(/^Home release/);
		expect(cell({ ...m, digital_date: '2000-01-01' } as TitleView, col('release'), '')).toMatch(/^Out/);
		expect(cell({ ...m, digital_date: undefined, cinema_date: '2999-01-01' } as TitleView, col('release'), '')).toMatch(/^Cinemas/);
		expect(cell(m, col('series'), 'Matrix')).toBe('Matrix');
		expect(cell(m, col('nope'), '')).toBe('');
		expect(cell({ ...m, score: 7.4, votes: 900 } as TitleView, col('score'), '')).toBe('★ 7.4');
		expect(cell({ ...m, score: 8, votes: 900 } as TitleView, col('score'), '')).toBe('★ 8.0');
		expect(cell({ ...m, score: 9.1, votes: 3 } as TitleView, col('score'), '')).toBe('');
		expect(cell(m, col('score'), '')).toBe('');
		expect(rowCell(m, col('episodes'), { have: 3, total: 10 }, '')).toBe('3/10');
		expect(rowCell(m, col('episodes'), { have: 0, total: 0 }, '')).toBe('');
	});
});

describe('tone', () => {
	const show = (in_library: number, episodes: number, missing: number, keep = true) =>
		({ kind: 'show', slots: [], keep_up_to_date: keep, show: { in_library, episodes, not_out_yet: 0, missing } }) as unknown as TitleView;
	it('picks the bar state', () => {
		expect(tone(show(0, 0, 0))).toBe('later');
		expect(tone(show(5, 5, 0))).toBe('full');
		expect(tone(show(0, 5, 5, false))).toBe('none');
		expect(tone(show(3, 5, 0))).toBe('later');
		expect(tone(show(3, 5, 2))).toBe('gap');
	});
});
