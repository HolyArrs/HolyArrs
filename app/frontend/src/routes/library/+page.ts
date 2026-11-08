import { lastLibraryKind, type ShelfKind, partsNow } from '#lib/mediafiles/index.ts';
import { redirect, type Load } from '@sveltejs/kit';
import { session } from '#lib/core/session.svelte.ts';

// There is no mixed Library page: a link without a kind opens Movies, TV shows or Music (the one used last), keeping any other search words in the link.
export const load: Load = ({ url }) => {
	const k = url.searchParams.get('kind');
	if (k === 'movie' || k === 'show' || k === 'artist') return;
	const off = (part: string) => partsNow.answer?.parts.find((p) => p.part === part)?.on === false;
	const allowed = (
		[
			['movie', 'movies'],
			['show', 'shows'],
			['artist', 'music']
		] as [ShelfKind, string][]
	)
		.filter(([kind, part]) => !off(part) && !(kind === 'artist' && session.kid))
		.map(([kind]) => kind);
	const next = new URL(url);
	next.searchParams.set('kind', lastLibraryKind(allowed));
	redirect(307, next.pathname + next.search + next.hash);
};
