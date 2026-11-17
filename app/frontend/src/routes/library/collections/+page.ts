import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';

// Movie series live in the Library: the old list opens Movies grouped by series.
export function load() {
	redirect(307, resolve('/library') + '?kind=movie&group=series');
}
