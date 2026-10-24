// Where a title page shows its trailers while Luis compares the options
// (?look=row|feature|chips). The choice is kept per browser so every title
// opens with the same option.
import type { TrailerLayout } from './TitleTrailers.svelte';

export type TitleLook = TrailerLayout;

export const TITLE_LOOKS: { value: TitleLook; label: string }[] = [
	{ value: 'row', label: 'T1 · Row' },
	{ value: 'feature', label: 'T2 · Featured' },
	{ value: 'chips', label: 'T3 · Pills' }
];

const KEY = 'holyarrs.trailer-look';
const known = (v: string | null): v is TitleLook => v === 'row' || v === 'feature' || v === 'chips';

function first(): TitleLook {
	if (typeof window === 'undefined') return 'row';
	const q = new URLSearchParams(location.search).get('look');
	if (known(q)) return q;
	try {
		const s = localStorage.getItem(KEY);
		if (known(s)) return s;
	} catch {
		/* No storage: the default option. */
	}
	return 'row';
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
