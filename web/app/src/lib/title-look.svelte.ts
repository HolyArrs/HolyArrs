// Where a title page puts its facts line and tagline while Luis compares the
// options (?look=both|band). The choice is kept per browser so every
// title opens with the same option.
export type TitleLook = 'decide' | 'both' | 'band';

export const TITLE_LOOKS: { value: TitleLook; label: string }[] = [
	{ value: 'decide', label: 'Decide in 5 s' },
	{ value: 'both', label: 'H3 · Facts in hero' },
	{ value: 'band', label: 'H4 · Facts in band' }
];

const KEY = 'holyarrs.hero-look-2';
const known = (v: string | null): v is TitleLook => v === 'decide' || v === 'both' || v === 'band';

function first(): TitleLook {
	if (typeof window === 'undefined') return 'decide';
	const q = new URLSearchParams(location.search).get('look');
	if (known(q)) return q;
	try {
		const s = localStorage.getItem(KEY);
		if (known(s)) return s;
	} catch {
		/* No storage: the default option. */
	}
	return 'decide';
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
