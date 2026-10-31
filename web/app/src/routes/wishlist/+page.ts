import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// The Requests page used to be called the Wishlist; old links, bookmarks and
// alerts still land here and go on to /requests with the query kept.
export const load: PageLoad = ({ url }) => {
	redirect(308, '/requests' + url.search + url.hash);
};
