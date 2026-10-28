// The compact content styles Luis is comparing on every settings page
// (?look=1…5, picked with the shared VariantBar). 0 is today's style.
// The styles themselves live in settings-looks.css, keyed on data-look.
export const LOOKS = [
	{ value: 0, label: 'Now' },
	{ value: 1, label: '1 Panels' },
	{ value: 2, label: '2 Rows' },
	{ value: 3, label: '3 Form' },
	{ value: 4, label: '4 Grid' },
	{ value: 5, label: '5 Groups' }
];

/** lookFrom reads ?look= the way the bar does: 1…5, anything else is today's style. */
export function lookFrom(v: string | null): number {
	const n = Number(v);
	return Number.isInteger(n) && n >= 1 && n <= 5 ? n : 0;
}

/** Looks that drop the coloured icon tile next to a section title; the others draw a quiet glyph. */
export const NO_TILE = new Set([1, 3, 4]);
