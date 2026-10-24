// Which season layout a show's title page uses while Luis compares them (?look=grid|row|cols).
// The choice is kept per browser so every show opens in the same layout.
import type { SeasonLayout } from './SeasonFolds.svelte';

export type TitleLook = SeasonLayout;

export const TITLE_LOOKS: { value: TitleLook; label: string }[] = [
	{ value: 'grid', label: 'S1 · Grid' },
	{ value: 'row', label: 'S2 · Row' },
	{ value: 'cols', label: 'S3 · Columns' }
];

const KEY = 'holyarrs.season-look';
const known = (v: string | null): v is TitleLook => v === 'grid' || v === 'row' || v === 'cols';

function first(): TitleLook {
	if (typeof window === 'undefined') return 'grid';
	const q = new URLSearchParams(location.search).get('look');
	if (known(q)) return q;
	try {
		const s = localStorage.getItem(KEY);
		if (known(s)) return s;
	} catch {
		/* No storage: the default layout. */
	}
	return 'grid';
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
