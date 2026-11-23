import { describe, expect, it } from 'vitest';
import { withoutEditing } from './edit-in-place.ts';

describe('withoutEditing', () => {
	const rows = [{ id: 1 }, { id: 2 }, { id: 3 }];
	it('keeps every row when nothing is being changed', () => {
		expect(withoutEditing(rows, null)).toEqual(rows);
		expect(withoutEditing(rows, undefined)).toEqual(rows);
	});
	it('hides the row being changed', () => {
		expect(withoutEditing(rows, 2)).toEqual([{ id: 1 }, { id: 3 }]);
	});
	it('keeps every row for a new item', () => {
		expect(withoutEditing(rows, 0)).toEqual(rows);
	});
	it('takes a custom id', () => {
		const sites = [{ ID: 7 }, { ID: 8 }];
		expect(withoutEditing(sites, 8, (s) => s.ID)).toEqual([{ ID: 7 }]);
	});
});
