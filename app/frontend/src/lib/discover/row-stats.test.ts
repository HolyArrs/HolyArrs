import { describe, expect, it } from 'vitest';
import { rowStatLines, NAMED_ROWS, type RowStat } from './row-stats.ts';

const words = (s: string) => `[${s}]`;
const own = (type: string) => `own:${type}`;

describe('rowStatLines', () => {
	const cases: { name: string; rows: RowStat[]; want: { key: string; row: string; opens: number; asks: number }[] }[] = [
		{ name: 'nothing counted', rows: [], want: [] },
		{
			name: 'a fixed row keeps its server title, shown in the picked language',
			rows: [{ row_type: 'hidden', title: 'Hidden gems', shown: 4, opens: 2, asks: 1 }],
			want: [{ key: 'hidden', row: '[Hidden gems]', opens: 2, asks: 1 }]
		},
		{
			name: 'a row named after a title, person or genre gets our own name',
			rows: [{ row_type: 'because_have', shown: 3, opens: 1, asks: 0 }, { row_type: 'hero', shown: 0, opens: 0, asks: 2 }],
			want: [{ key: 'because_have', row: 'own:because_have', opens: 1, asks: 0 }, { key: 'hero', row: 'own:hero', opens: 0, asks: 2 }]
		},
		{
			name: 'an unknown row with no title shows its key, never blank',
			rows: [{ row_type: 'something_new', shown: 1, opens: 0, asks: 0 }],
			want: [{ key: 'something_new', row: 'something_new', opens: 0, asks: 0 }]
		},
		{
			name: 'the server order (busiest first) is kept',
			rows: [{ row_type: 'trending', title: 'Trending this week everywhere', shown: 9, opens: 5, asks: 3 }, { row_type: 'classics', title: 'Classics', shown: 9, opens: 1, asks: 0 }],
			want: [{ key: 'trending', row: '[Trending this week everywhere]', opens: 5, asks: 3 }, { key: 'classics', row: '[Classics]', opens: 1, asks: 0 }]
		}
	];
	for (const c of cases) {
		it(c.name, () => {
			const got = rowStatLines(c.rows, words, own).map(({ key, row, opens, asks }) => ({ key, row, opens, asks }));
			expect(got).toEqual(c.want);
		});
	}
	it('names every row the server cannot title', () => {
		expect(NAMED_ROWS).toEqual(expect.arrayContaining(['because_have', 'because_asked', 'person', 'genre', 'popular_genre', 'popular_decade', 'decade', 'mood', 'hero']));
	});
});
