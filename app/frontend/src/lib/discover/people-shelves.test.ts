import { byDept, chipDept, visibleChips, type ShelfMeta } from './people-shelves.ts';
import { describe, expect, it } from 'vitest';

const shelf = (departments: Record<string, number>): ShelfMeta => ({ id: 'library', title: 'From your Library', count: 20, departments });

describe('people shelves', () => {
	it('filters by department and always leaves writers out', () => {
		const people = [{ department: 'Acting' }, { department: 'Writing' }, { department: 'Sound' }, { department: 'Directing' }];
		expect(byDept(people, '')).toHaveLength(3);
		expect(byDept(people, chipDept('composers'))).toEqual([{ department: 'Sound' }]);
	});
	it('hides a chip that would show fewer than eight people', () => {
		const popular = Array.from({ length: 5 }, () => ({ department: 'Directing' }));
		const chips = visibleChips([shelf({ Acting: 12, Directing: 3, Sound: 7 })], popular).map((c) => c.value);
		expect(chips).toEqual(['popular', 'actors', 'directors']);
		expect(visibleChips([], []).map((c) => c.value)).toEqual(['popular']);
	});
});
