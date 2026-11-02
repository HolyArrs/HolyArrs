import { redirect } from '@sveltejs/kit';
import type { Load } from '@sveltejs/kit';
import { to } from '#lib/paths.ts';

// The Requests page used to be called the Wishlist; old links, bookmarks and
// alerts still land here and go on to /requests with the query kept.
export const load: Load = ({ url }) => {
	redirect(307, to('/requests') + url.search + url.hash);
};
