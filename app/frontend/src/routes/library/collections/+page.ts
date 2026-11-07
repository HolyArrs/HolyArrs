import { redirect } from '@sveltejs/kit';
import { to } from '#lib/core/paths.ts';

// Movie series now live in the Library: the old list opens Movies grouped by series.
export function load() {
	redirect(307, to('/library?kind=movie&group=series'));
}
