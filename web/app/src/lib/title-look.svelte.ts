// Which version of title page layout A is shown while Luis compares them (?look=ask|stack|strip).
// The choice is kept per browser so every title page opens in the same layout.
export type TitleLook = 'ask' | 'stack' | 'strip';

export const TITLE_LOOKS: { value: TitleLook; label: string }[] = [
	{ value: 'ask', label: 'A1 · Beside' },
	{ value: 'stack', label: 'A2 · Stacked' },
	{ value: 'strip', label: 'A3 · Strip' }
];

const KEY = 'holyarrs.title-look';
const known = (v: string | null): v is TitleLook => v === 'ask' || v === 'stack' || v === 'strip';

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
