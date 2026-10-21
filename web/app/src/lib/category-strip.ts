// The category strip under the Explore hero: the household's Browse
// categories, one from each section in turn (genre, mood, story, world, era,
// around the world, collection, then again), from the Browse answers the
// server already keeps warm. Pure.
import type { ExploreType } from './explore.ts';
import { strictTiles, type BrowseTile } from './search-browse.ts';

/** STRIP_MAX is how many categories the strip shows before All categories. */
export const STRIP_MAX = 18;

/** STRIP_ORDER is the section pattern the strip repeats. */
export const STRIP_ORDER = ['genres', 'moods', 'stories', 'worlds', 'eras', 'world', 'collections'] as const;

/** The small label above a tile's name, per section. */
export const SECTION_LABEL: Readonly<Record<string, string>> = {
	genres: 'Genre', moods: 'Mood', stories: 'Story', worlds: 'World', eras: 'Era', world: 'Around the world', collections: 'Collection'
};

/** KIN groups near-duplicates; two of a group never sit side by side. */
const KIN: readonly (readonly string[])[] = [
	['thriller', 'edge-of-your-seat', 'adrenaline', 'suspense', 'conspiracies', 'spies', 'assassins'],
	['horror', 'scary', 'zombies', 'vampires', 'monsters', 'haunted houses', 'serial killers'],
	['comedy', 'laugh out loud', 'quirky', 'satirical', 'dark comedy'],
	['romance', 'swoon-worthy', 'love triangles', 'weddings'],
	['crime', 'gritty', 'heists', 'gangsters', 'cops', 'con artists', 'prison'],
	['mystery', 'whodunits', 'mind-bending', 'nordic noir'],
	['drama', 'tearjerkers', 'slow burn', 'thought-provoking'],
	['feel-good', 'uplifting', 'inspiring', 'cozy', 'nostalgic'],
	['family', 'animation', 'animals', 'fairy tales', 'friendship', 'family ties'],
	['history', 'world war ii', 'cold war', 'victorian era', 'ancient worlds', 'medieval', 'classics before 1970'],
	['space', 'aliens', 'robots & ai', 'cyberpunk', 'parallel worlds', 'time travel', 'dystopia', 'post-apocalyptic'],
	['fantasy worlds', 'magic', 'dragons', 'epic'],
	['dark', 'revenge'],
	['japanese', 'anime'],
	['spanish', 'latin american']
];

/** kin is a category's near-duplicate group, or its own name. */
export function kin(name: string): string {
	const n = name.toLowerCase();
	const i = KIN.findIndex((g) => g.includes(n));
	return i < 0 ? n : `kin${i}`;
}

/** seeded is a small repeatable random (mulberry32) from a string. */
export function seeded(seed: string): () => number {
	let h = 1779033703;
	for (const ch of seed) h = Math.imul(h ^ ch.charCodeAt(0), 3432918353);
	let a = h >>> 0;
	return () => {
		a = (a + 0x6d2b79f5) | 0;
		let t = Math.imul(a ^ (a >>> 15), 1 | a);
		t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
		return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
	};
}

/** dayKey is the seed's day part (local date), so the strip turns over daily. */
export function dayKey(d: Date): string {
	return `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`;
}

/**
 * ranked is a section's tiles in taste order (the server's order), then the
 * larger count, with a small daily jitter so the top few take turns.
 */
export function ranked(tiles: readonly BrowseTile[], rnd: () => number): BrowseTile[] {
	return tiles
		.map((t, i) => ({ t, s: i - (t.count ?? 0) / 1000 + rnd() * 3 }))
		.sort((a, b) => a.s - b.s)
		.map((x) => x.t);
}

/**
 * stripTiles takes one category from each section in STRIP_ORDER in turn
 * (a section with none left is skipped), the tab's kind only, one name once,
 * and never a near-duplicate next to the one before it.
 */
export function stripTiles(sections: Readonly<Record<string, readonly BrowseTile[]>>, tab: ExploreType, seed: string, max = STRIP_MAX): (BrowseTile & { section: string })[] {
	const rnd = seeded(seed);
	const lists = STRIP_ORDER.map((id) => ({ id, left: ranked(strictTiles([...(sections[id] ?? [])], tab), rnd) }));
	const out: (BrowseTile & { section: string })[] = [];
	const names = new Set<string>();
	const kins = new Set<string>();
	let last = '';
	for (let round = 0; out.length < max && lists.some((l) => l.left.length); round++) {
		for (const l of lists) {
			if (out.length >= max) break;
			// Drop names already shown; take the first that is not kin to the tile before or anything shown.
			l.left = l.left.filter((t) => !names.has(t.name.toLowerCase()));
			let i = l.left.findIndex((t) => !kins.has(kin(t.name)));
			if (i < 0) i = l.left.findIndex((t) => kin(t.name) !== last);
			if (i < 0) continue;
			const [t] = l.left.splice(i, 1);
			names.add(t.name.toLowerCase());
			last = kin(t.name);
			kins.add(last);
			out.push({ ...t, section: l.id });
		}
	}
	return out;
}
