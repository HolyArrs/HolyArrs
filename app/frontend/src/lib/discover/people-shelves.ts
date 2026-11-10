import { t } from '#lib/core/i18n/i18n.svelte.ts';
export type PeopleFilter = 'popular' | 'actors' | 'directors' | 'composers';

export type ShelfMeta = { id: string; title: string; count: number; departments: Record<string, number> };
export type ShelfPerson = { id: string; name: string; department: string; profile: string; note: string };

// Search's People sections: From your Library and In what's hot right now (from /people/shelves), then the popular list. A section shows only with at least SHELF_MIN people; a chip shows only when it would show that many.
export const SHELF_MIN = 8;

// Rows a section shows at first, and adds each time its end nears.
export const PEOPLE_ROWS = 2;

export const PEOPLE_CHIPS: { value: PeopleFilter; label: string; dept: string }[] = [
	{ value: 'popular', get label() { return t('discover.peopleShelves.popular'); }, dept: '' },
	{ value: 'actors', get label() { return t('discover.peopleShelves.actors'); }, dept: 'Acting' },
	{ value: 'directors', get label() { return t('discover.peopleShelves.directors'); }, dept: 'Directing' },
	{ value: 'composers', get label() { return t('discover.peopleShelves.composers'); }, dept: 'Sound' }
];

export const chipDept = (f: PeopleFilter) => PEOPLE_CHIPS.find((c) => c.value === f)?.dept ?? '';

// Writers, creators and other crew never show on the People tab.
export const PEOPLE_DEPTS = ['Acting', 'Directing', 'Sound'];

/** The people of one department ('' for every shown one). */
export function byDept<T extends { department?: string }>(people: T[], dept: string): T[] {
	return people.filter((p) => (dept ? p.department === dept : PEOPLE_DEPTS.includes(p.department ?? '')));
}

/** The chips worth showing: Popular always, the others with SHELF_MIN people across the sections and the loaded popular list. */
export function visibleChips(shelves: ShelfMeta[], popular: { department?: string }[]) {
	return PEOPLE_CHIPS.filter((c) => {
		if (!c.dept) return true;
		const n = shelves.reduce((sum, s) => sum + (s.departments[c.dept] ?? 0), 0) + byDept(popular, c.dept).length;
		return n >= SHELF_MIN;
	});
}
