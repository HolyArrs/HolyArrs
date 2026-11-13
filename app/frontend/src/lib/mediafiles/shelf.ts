// Phones have no side menu: the Library title menu switches between Movies, TV shows and Music,
// and also leads to the Library stats for people who look after the Library.
export type ShelfEntry = { key: 'movie' | 'show' | 'artist' | 'stats'; href: string };

export function shelfEntries(o: { on: (part: 'movies' | 'shows' | 'music') => boolean; kid: boolean; manage: boolean }): ShelfEntry[] {
	const kinds = (
		[
			['movie', 'movies'],
			['show', 'shows'],
			['artist', 'music']
		] as const
	).filter(([k, part]) => o.on(part) && !(k === 'artist' && o.kid));
	const list: ShelfEntry[] = kinds.map(([k]) => ({ key: k, href: `/library?kind=${k}` }));
	if (o.manage) list.push({ key: 'stats', href: '/library/stats' });
	return list;
}
