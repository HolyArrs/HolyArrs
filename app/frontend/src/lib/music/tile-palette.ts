// The Music Browse tile palette: one colour per tile, the same on every page that shows Browse tiles (Explore, New & Popular, Search). Sixteen colours, bright and saturated but dark enough for white text at AA (4.5:1).

export interface TileColour {
	name: string;
	hex: string;
}

export const TILE_PALETTE: readonly TileColour[] = [
	{ name: 'red', hex: '#d62839' },
	{ name: 'orange', hex: '#c4520a' },
	{ name: 'mustard', hex: '#9c6c00' },
	{ name: 'green', hex: '#23802f' },
	{ name: 'emerald', hex: '#0b7a55' },
	{ name: 'teal', hex: '#0a7b8a' },
	{ name: 'steel', hex: '#3b6ea5' },
	{ name: 'blue', hex: '#2a5fe0' },
	{ name: 'navy', hex: '#2b3b99' },
	{ name: 'indigo', hex: '#4a3bcb' },
	{ name: 'violet', hex: '#a02fc0' },
	{ name: 'lavender', hex: '#7b5ed0' },
	{ name: 'magenta', hex: '#c4208c' },
	{ name: 'pink', hex: '#d3336e' },
	{ name: 'terracotta', hex: '#a8452b' },
	{ name: 'slate', hex: '#4e6075' }
];

/** Once the palette is used up, a colour waits this many tiles before it comes back. */
export const RECENT = 10;

/** The colour of each common key (kind prefix off). Within a kind no two keys share one. */
export const TILE_FIXED: Readonly<Record<string, string>> = {
	// Genres (the first sixteen in the Browse layout all differ).
	pop: 'pink', 'hip-hop': 'orange', rock: 'red', latin: 'mustard', rnb: 'violet', country: 'terracotta', dance: 'blue', indie: 'emerald',
	alternative: 'teal', metal: 'slate', kpop: 'magenta', afro: 'green', jazz: 'navy', classical: 'steel', 'soul-funk': 'lavender', folk: 'indigo',
	reggae: 'mustard', blues: 'violet', punk: 'red', christian: 'pink', soundtracks: 'orange', kids: 'terracotta',
	// Moods.
	chill: 'teal', party: 'indigo', love: 'magenta', sad: 'navy', happy: 'green', focus: 'steel', workout: 'orange', sleep: 'slate', romance: 'pink',
	energy: 'red', throwback: 'mustard', 'feel-good': 'emerald',
	// Celebrations and seasons.
	halloween: 'lavender', christmas: 'green', 'new-year': 'steel', valentines: 'pink', summer: 'mustard', 'dia-de-muertos': 'magenta', 'lunar-new-year': 'red',
	pride: 'indigo', thanksgiving: 'terracotta', 'back-to-school': 'blue', fall: 'orange', spring: 'emerald',
	// Decades.
	'1960s': 'terracotta', '1970s': 'mustard', '1980s': 'magenta', '1990s': 'green', '2000s': 'steel', '2010s': 'teal', '2020s': 'blue',
	// Activities (Party is the mood's, so both tiles match).
	driving: 'navy', cooking: 'red', gaming: 'violet', studying: 'steel', gym: 'orange', running: 'green', home: 'terracotta'
};

// Other spellings of the same page: the old chart slugs and the plain names.
const ALIAS: Readonly<Record<string, string>> = {
	'hip-hop-rap': 'hip-hop', hiphop: 'hip-hop', 'r-b-soul': 'rnb', 'r-and-b': 'rnb', 'randb-soul': 'rnb', 'r-and-b-soul': 'rnb', electronic: 'dance', 'dance-and-electronic': 'dance',
	'k-pop': 'kpop', 'children-s-music': 'kids', 'kids-and-family': 'kids', soundtrack: 'soundtracks', 'singer-songwriter': 'folk', 'folk-and-acoustic': 'folk',
	'christian-gospel': 'christian', 'christian-and-gospel': 'christian', gospel: 'christian', afrobeats: 'afro', african: 'afro', 'reggae-and-caribbean': 'reggae',
	'hard-rock': 'rock', 'indie-pop': 'indie', 'pop-latino': 'latin', 'urbano-latino': 'latin', 'musica-mexicana': 'latin', 'soul-and-funk': 'soul-funk',
	'high-energy': 'energy', romantic: 'romance', 'valentine-s': 'valentines', 'chill-at-home': 'home'
};

/** A key's slug: the kind prefix off, lower case, plain dashes, a decade as "1990s". */
export function tileSlug(key: string): string {
	const s = key
		.trim()
		.toLowerCase()
		.replace(/^[a-z]+:/, '')
		.replace(/&/g, ' and ')
		.replace(/[^a-z0-9]+/g, '-')
		.replace(/^-+|-+$/g, '');
	const decade = /^(\d{2})s$/.exec(s);
	if (decade) return `${Number(decade[1]) >= 50 ? '19' : '20'}${s}`;
	return ALIAS[s] ?? s;
}

const INDEX = new Map(TILE_PALETTE.map((c, i) => [c.name, i]));

/** The palette index a key asks for: its fixed colour, else a hash of its slug. */
export function preferredColour(key: string): number {
	const slug = tileSlug(key);
	const fixed = TILE_FIXED[slug];
	const at = fixed === undefined ? undefined : INDEX.get(fixed);
	return at ?? hash(slug) % TILE_PALETTE.length;
}

/** Gives each key of one grid, in order, a colour: the one it prefers, or the next unused one. A tile's colour depends only on the tiles before it, so a grid that shows more tiles on a wider screen keeps the colours it had. */
export function assignTileColours(keys: readonly string[]): TileColour[] {
	const n = TILE_PALETTE.length;
	const used = new Set<number>();
	const recent: number[] = [];
	const out: TileColour[] = [];
	for (const key of keys) {
		if (used.size >= n) used.clear();
		const want = preferredColour(key);
		let pick = want;
		for (let step = 0; step < n; step++) {
			const c = (want + step) % n;
			if (!used.has(c) && !recent.includes(c)) {
				pick = c;
				break;
			}
		}
		used.add(pick);
		recent.push(pick);
		if (recent.length > RECENT) recent.shift();
		out.push(TILE_PALETTE[pick]);
	}
	return out;
}

function hash(s: string): number {
	let h = 2166136261;
	for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619);
	return h >>> 0;
}
