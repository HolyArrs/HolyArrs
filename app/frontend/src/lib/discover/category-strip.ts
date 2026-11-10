// The category strip under the Explore hero: the household's Browse categories, one from each section in turn (genre, mood, story, world, era, around the world, collection, then again), from the Browse answers the page already has.
import type { ExploreType } from './explore-type.ts';
import { type BrowseTile } from './search-browse.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

/** GET /explore/strip: the member's strip, picked and ordered on the server. */
export interface StripAnswer {
	type: ExploreType;
	tiles: BrowseTile[];
	/** Some Browse sections were still building. */
	partial?: boolean;
}

/** The small label above a tile's name, per section. */
export const SECTION_LABEL: Readonly<Record<string, string>> = {
	get genres() { return t('discover.categoryStrip.genre'); },
	get moods() { return t('discover.categoryStrip.mood'); },
	get stories() { return t('discover.categoryStrip.story'); },
	get worlds() { return t('discover.categoryStrip.world'); },
	get eras() { return t('discover.categoryStrip.era'); },
	get world() { return t('discover.categoryStrip.aroundTheWorld'); },
	get collections() { return t('discover.categoryStrip.collection'); }
};

/** Groups near-duplicates; two of a group never sit side by side. */
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

/** A small repeatable random generator (mulberry32) from a string. */
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

/** The seed's day part (local date), so the strip turns over daily. */
export function dayKey(d: Date): string {
	return `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`;
}

/** A section's tiles in taste order (the server's order), then the larger count, with a small daily jitter so the top few take turns. */
export function ranked(tiles: readonly BrowseTile[], rnd: () => number): BrowseTile[] {
	return tiles
		.map((t, i) => ({ t, s: i - (t.count ?? 0) / 1000 + rnd() * 3 }))
		.sort((a, b) => a.s - b.s)
		.map((x) => x.t);
}

/** The titles of the rows below, trimmed, once each, at most 40 of up to 80 characters. */
export function stripRowNames(rows: readonly { title?: string }[]): string[] {
	const out: string[] = [];
	for (const r of rows) {
		const t = (r.title ?? '').trim();
		if (t && t.length <= 80 && !out.includes(t)) out.push(t);
		if (out.length === 40) break;
	}
	return out;
}

/** Asks for the strip with the rows below (so it skips what they show) and the viewer's time zone. */
export function stripPath(t: ExploreType, rows: readonly string[], tz?: string): string {
	const p = new URLSearchParams({ type: t });
	for (const r of rows) p.append('row', r);
	if (tz) p.set('tz', tz);
	return `/explore/strip?${p.toString()}`;
}

/** What the strip counts a tile by: its category id, or a genre tile's lower-case name. */
export function stripKey(t: Pick<BrowseTile, 'category' | 'name'>): string {
	return t.category || t.name.toLowerCase();
}

/** Makes the answer's tiles the strip's; an empty category id is dropped, the section kept. */
export function fromStrip(tiles: readonly BrowseTile[]): (BrowseTile & { section: string })[] {
	return tiles.map(({ category, ...t }) => ({ ...t, ...(category ? { category } : {}), section: t.section ?? 'genres' }));
}

/** Gathers the keys of tiles the strip showed and sends them once, after `delay` ms of quiet (each key once per batch, at most 40). */
export function shownBatcher(send: (keys: string[]) => void, delay = 1500) {
	let keys: string[] = [];
	let timer: ReturnType<typeof setTimeout> | undefined;
	const flush = () => {
		if (timer) clearTimeout(timer);
		timer = undefined;
		if (!keys.length) return;
		const out = keys.slice(0, 40);
		keys = [];
		send(out);
	};
	return {
		add(more: readonly string[]) {
			for (const k of more) if (!keys.includes(k)) keys.push(k);
			if (timer) clearTimeout(timer);
			timer = setTimeout(flush, delay);
		},
		flush
	};
}
