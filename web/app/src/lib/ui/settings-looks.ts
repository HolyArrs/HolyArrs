// The settings side menu views Luis is comparing (?menu=1…5, picked with
// the shared VariantBar). No parameter shows view 1.
export const MENUS = [
	{ value: 1, label: '1 Lines' },
	{ value: 2, label: '2 Text' },
	{ value: 3, label: '3 Dots' },
	{ value: 4, label: '4 Tiles' },
	{ value: 5, label: '5 Folds' }
];

/** menuView turns the bar's value into a view: 0 (no pick) is view 1. */
export function menuView(v: number): number {
	return Number.isInteger(v) && v >= 1 && v <= 5 ? v : 1;
}

/** One quiet colour per settings group, for the dots in view 3. */
export const GROUP_DOT: Record<string, string> = {
	general: '#8e8e93',
	library: '#0a7aff',
	copies: '#2fb350',
	people: '#a550d8',
	linked: '#f58b00',
	system: '#5f6168',
	account: '#8e8e93'
};
