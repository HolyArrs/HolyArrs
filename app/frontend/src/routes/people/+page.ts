import { redirect, type Load } from '@sveltejs/kit';
import { to } from '#lib/core/paths.ts';

// People are found on Search (its People tab); the old list keeps any search words.
export const load: Load = ({ url }) => {
	const q = url.searchParams.get('q');
	redirect(307, to(`/search?kind=people${q ? `&q=${encodeURIComponent(q)}` : ''}`));
};
