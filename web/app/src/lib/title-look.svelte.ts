// Where a title page puts its facts line and tagline while Luis compares the
// options (?look=both|band). The choice is kept per browser so every
// title opens with the same option.
export type TitleLook = 'quiet' | 'scores' | 'story' | 'decide' | 'both' | 'band';

export const TITLE_LOOKS: { value: TitleLook; label: string }[] = [
	{ value: 'quiet', label: 'V1 · Quiet' },
	{ value: 'scores', label: 'V2 · Scores up top' },
	{ value: 'story', label: 'V3 · Story first' },
	{ value: 'decide', label: 'V4 · Split' }
];

const KEY = 'holyarrs.hero-look-3';
const known = (v: string | null): v is TitleLook => v === 'quiet' || v === 'scores' || v === 'story' || v === 'decide' || v === 'both' || v === 'band';

function first(): TitleLook {
	if (typeof window === 'undefined') return 'quiet';
	const q = new URLSearchParams(location.search).get('look');
	if (known(q)) return q;
	try {
		const s = localStorage.getItem(KEY);
		if (known(s)) return s;
	} catch {
		/* No storage: the default option. */
	}
	return 'quiet';
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
