// Where a title page puts its facts line and tagline while Luis compares the
// options (?look=keep|swap|both). The choice is kept per browser so every
// title opens with the same option.
export type TitleLook = 'keep' | 'swap' | 'both';

export const TITLE_LOOKS: { value: TitleLook; label: string }[] = [
	{ value: 'keep', label: 'H1 · Keep' },
	{ value: 'swap', label: 'H2 · Swap' },
	{ value: 'both', label: 'H3 · Both in hero' }
];

const KEY = 'holyarrs.hero-look';
const known = (v: string | null): v is TitleLook => v === 'keep' || v === 'swap' || v === 'both';

function first(): TitleLook {
	if (typeof window === 'undefined') return 'keep';
	const q = new URLSearchParams(location.search).get('look');
	if (known(q)) return q;
	try {
		const s = localStorage.getItem(KEY);
		if (known(s)) return s;
	} catch {
		/* No storage: the default option. */
	}
	return 'keep';
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
