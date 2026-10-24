// Which title page layout is shown while Luis compares them (?look=ask|apple|netflix).
// The choice is kept per browser so every title page opens in the same layout.
export type TitleLook = 'ask' | 'apple' | 'netflix';

export const TITLE_LOOKS: { value: TitleLook; label: string }[] = [
	{ value: 'ask', label: 'A · Ask' },
	{ value: 'apple', label: 'B · Apple' },
	{ value: 'netflix', label: 'C · Netflix' }
];

const KEY = 'holyarrs.title-look';
const known = (v: string | null): v is TitleLook => v === 'ask' || v === 'apple' || v === 'netflix';

function first(): TitleLook {
	if (typeof window === 'undefined') return 'ask';
	const q = new URLSearchParams(location.search).get('look');
	if (known(q)) return q;
	try {
		const s = localStorage.getItem(KEY);
		if (known(s)) return s;
	} catch {
		/* No storage: the default layout. */
	}
	return 'ask';
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
