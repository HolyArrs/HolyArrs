import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { to } from '#lib/paths.ts';

// The Requests page used to be called the Wishlist; old links, bookmarks and
// alerts still land here and go on to /requests with the query kept.
export const load: PageLoad = ({ url }) => {
	redirect(307, to('/requests') + url.search + url.hash);
};
