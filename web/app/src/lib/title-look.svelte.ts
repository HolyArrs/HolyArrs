// Where a title page shows its trailers while Luis compares the options
// (?look=small|left|right). The choice is kept per browser so every title
// opens with the same option.
export type TitleLook = 'small' | 'left' | 'right';

export const TITLE_LOOKS: { value: TitleLook; label: string }[] = [
	{ value: 'small', label: 'V1 · Small row' },
	{ value: 'left', label: 'V2 · Left' },
	{ value: 'right', label: 'V3 · Right' }
];

const KEY = 'holyarrs.trailer-look-2';
const known = (v: string | null): v is TitleLook => v === 'small' || v === 'left' || v === 'right';

function first(): TitleLook {
	if (typeof window === 'undefined') return 'small';
	const q = new URLSearchParams(location.search).get('look');
	if (known(q)) return q;
	try {
		const s = localStorage.getItem(KEY);
		if (known(s)) return s;
	} catch {
		/* No storage: the default option. */
	}
	return 'small';
}

export const titleLook = $state({ value: first() });

export function setTitleLook(v: TitleLook) {
	titleLook.value = v;
	try {
		localStorage.setItem(KEY, v);
	} catch {
		/* Kept for this visit only. */
	}
}
