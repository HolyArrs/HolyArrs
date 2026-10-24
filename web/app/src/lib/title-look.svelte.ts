// Which option for the block under the title page hero is shown while Luis
// compares them (?look=card|strip|tiles). The choice is kept per browser so
// every title opens in the same option.
export type TitleLook = 'card' | 'strip' | 'tiles';

export const TITLE_LOOKS: { value: TitleLook; label: string }[] = [
	{ value: 'card', label: 'I1 · Card' },
	{ value: 'strip', label: 'I2 · Strip' },
	{ value: 'tiles', label: 'I3 · Tiles' }
];

const KEY = 'holyarrs.info-look';
const known = (v: string | null): v is TitleLook => v === 'card' || v === 'strip' || v === 'tiles';

function first(): TitleLook {
	if (typeof window === 'undefined') return 'card';
	const q = new URLSearchParams(location.search).get('look');
	if (known(q)) return q;
	try {
		const s = localStorage.getItem(KEY);
		if (known(s)) return s;
	} catch {
		/* No storage: the default layout. */
	}
	return 'card';
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
