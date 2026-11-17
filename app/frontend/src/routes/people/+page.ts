import { redirect, type Load } from '@sveltejs/kit';
import { resolve } from '$app/paths';

// People are found on Search (its People tab); the old list keeps any search words.
export const load: Load = ({ url }) => {
	const q = url.searchParams.get('q');
	redirect(307, resolve('/search') + `?kind=people${q ? `&q=${encodeURIComponent(q)}` : ''}`);
};
