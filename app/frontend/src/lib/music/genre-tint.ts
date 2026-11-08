// A music genre's fixed tile colour: a token name from the MusicGenreTiles.svelte palette. The same genre always gets the same token; a genre not listed takes one by its server hue index.
const FIXED: Record<string, string> = {
	'hip-hop-rap': 'crimson',
	country: 'amber',
	pop: 'raspberry',
	'randb-soul': 'plum',
	alternative: 'teal',
	latin: 'vermilion',
	rock: 'wine',
	soundtrack: 'midnight',
	'k-pop': 'magenta',
	'children-s-music': 'forest',
	dance: 'indigo',
	electronic: 'blue',
	metal: 'charcoal'
};
const SPARE = ['rust', 'crimson', 'plum', 'teal', 'amber', 'midnight', 'wine'];

/** The CSS custom property (--mg-*) for a genre slug. */
export function genreTintToken(slug: string, hue = 0): string {
	return `--mg-${FIXED[slug] ?? SPARE[((hue % SPARE.length) + SPARE.length) % SPARE.length]}`;
}
