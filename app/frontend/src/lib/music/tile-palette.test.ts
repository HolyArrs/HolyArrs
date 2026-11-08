import { assignTileColours, preferredColour, RECENT, TILE_FIXED, TILE_PALETTE, tileSlug } from './tile-palette.ts';
import { describe, expect, it } from 'vitest';

// WCAG relative luminance and contrast, for the white name on a tile.
const lin = (c: number) => {
	const x = c / 255;
	return x <= 0.04045 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4;
};
const rgb = (hex: string) => [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));
const luminance = (hex: string) => {
	const [r, g, b] = rgb(hex).map(lin);
	return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};
const contrast = (a: string, b: string) => {
	const x = luminance(a);
	const y = luminance(b);
	return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
};
// CIE76 distance in Lab: how far apart two colours look.
const lab = (hex: string) => {
	const [r, g, b] = rgb(hex).map(lin);
	const X = (0.4124 * r + 0.3576 * g + 0.1805 * b) / 0.95047;
	const Y = 0.2126 * r + 0.7152 * g + 0.0722 * b;
	const Z = (0.0193 * r + 0.1192 * g + 0.9505 * b) / 1.08883;
	const f = (t: number) => (t > 0.008856 ? Math.cbrt(t) : 7.787 * t + 16 / 116);
	return [116 * f(Y) - 16, 500 * (f(X) - f(Y)), 200 * (f(Y) - f(Z))];
};
const distance = (a: string, b: string) => {
	const x = lab(a);
	const y = lab(b);
	return Math.hypot(x[0] - y[0], x[1] - y[1], x[2] - y[2]);
};

// The page backgrounds (tokens.css): light #ffffff, dark #0b0b0c.
const LIGHT_BG = '#ffffff';
const DARK_BG = '#0b0b0c';

// The keys the server sends, in its order.
const GENRES = ['pop', 'hip-hop', 'rock', 'latin', 'rnb', 'country', 'dance', 'indie', 'alternative', 'metal', 'kpop', 'afro', 'jazz', 'classical', 'soul-funk', 'folk', 'reggae', 'blues', 'punk', 'christian', 'soundtracks', 'kids'].map((k) => `genre:${k}`);
const MOODS = ['chill', 'party', 'love', 'sad', 'happy', 'focus', 'workout', 'sleep', 'romance', 'energy', 'throwback', 'feel-good'].map((k) => `mood:${k}`);
const SEASONS = ['halloween', 'christmas', 'new-year', 'valentines', 'summer', 'dia-de-muertos', 'lunar-new-year', 'pride', 'thanksgiving', 'back-to-school', 'fall', 'spring'].map((k) => `season:${k}`);
const DECADES = ['1960s', '1970s', '1980s', '1990s', '2000s', '2010s', '2020s'].map((k) => `decade:${k}`);
const ACTIVITIES = ['driving', 'cooking', 'gaming', 'studying', 'party', 'gym', 'running', 'home'].map((k) => `activity:${k}`);

describe('tile palette', () => {
	it('has sixteen named colours of our own, each once', () => {
		expect(TILE_PALETTE.map((c) => c.name)).toEqual(['red', 'orange', 'mustard', 'green', 'emerald', 'teal', 'steel', 'blue', 'navy', 'indigo', 'violet', 'lavender', 'magenta', 'pink', 'terracotta', 'slate']);
		expect(new Set(TILE_PALETTE.map((c) => c.hex)).size).toBe(16);
		for (const c of TILE_PALETTE) expect(c.hex).toMatch(/^#[0-9a-f]{6}$/);
	});
	it('keeps the white name at AA (4.5:1) on every colour', () => {
		for (const c of TILE_PALETTE) expect(contrast(c.hex, '#ffffff'), c.name).toBeGreaterThanOrEqual(4.5);
	});
	it('stands out from both page backgrounds', () => {
		for (const c of TILE_PALETTE) {
			expect(contrast(c.hex, LIGHT_BG), `${c.name} on light`).toBeGreaterThanOrEqual(3);
			expect(contrast(c.hex, DARK_BG), `${c.name} on dark`).toBeGreaterThanOrEqual(1.8);
		}
	});
	it('keeps the colours apart from each other', () => {
		for (let i = 0; i < TILE_PALETTE.length; i++) {
			for (let j = i + 1; j < TILE_PALETTE.length; j++) {
				expect(distance(TILE_PALETTE[i].hex, TILE_PALETTE[j].hex), `${TILE_PALETTE[i].name} / ${TILE_PALETTE[j].name}`).toBeGreaterThanOrEqual(15);
			}
		}
	});
	it('is not the Spotify palette', () => {
		const theirs = ['#e13300', '#1e3264', '#8d67ab', '#dc148c', '#e8115b', '#509bf5', '#477d95', '#ba5d07', '#148a08', '#8400e7'];
		for (const c of TILE_PALETTE) expect(theirs).not.toContain(c.hex);
	});
});

describe('tile slugs', () => {
	it('takes the kind prefix off', () => {
		expect(tileSlug('genre:hip-hop')).toBe('hip-hop');
		expect(tileSlug('mood:chill')).toBe('chill');
		expect(tileSlug('season:halloween')).toBe('halloween');
		expect(tileSlug('decade:1990s')).toBe('1990s');
		expect(tileSlug('activity:driving')).toBe('driving');
	});
	it('reads the old chart slugs and plain names as the same page', () => {
		expect(tileSlug('genre:hip-hop-rap')).toBe('hip-hop');
		expect(tileSlug('Hip-Hop/Rap')).toBe('hip-hop');
		expect(tileSlug('genre:randb-soul')).toBe('rnb');
		expect(tileSlug('R&B')).toBe('rnb');
		expect(tileSlug('K-Pop')).toBe('kpop');
		expect(tileSlug('children-s-music')).toBe('kids');
		expect(tileSlug('Dance & electronic')).toBe('dance');
		expect(tileSlug('decade:90s')).toBe('1990s');
		expect(tileSlug('80s')).toBe('1980s');
		expect(tileSlug('00s')).toBe('2000s');
	});
});

describe('fixed colours', () => {
	const names = new Set(TILE_PALETTE.map((c) => c.name));
	it('only use palette colours', () => {
		for (const [k, v] of Object.entries(TILE_FIXED)) expect(names.has(v), k).toBe(true);
	});
	it('cover every key of the Browse layout', () => {
		for (const k of [...GENRES, ...MOODS, ...SEASONS, ...DECADES, ...ACTIVITIES]) expect(TILE_FIXED[tileSlug(k)], k).toBeDefined();
	});
	it('give a kind no colour twice (the first sixteen genres)', () => {
		const of = (keys: string[]) => keys.map((k) => TILE_PALETTE[preferredColour(k)].name);
		expect(new Set(of(GENRES.slice(0, 16))).size).toBe(16);
		expect(new Set(of(GENRES.slice(16))).size).toBe(GENRES.length - 16);
		expect(new Set(of(MOODS)).size).toBe(MOODS.length);
		expect(new Set(of(SEASONS)).size).toBe(SEASONS.length);
		expect(new Set(of(DECADES)).size).toBe(DECADES.length);
		expect(new Set(of(ACTIVITIES)).size).toBe(ACTIVITIES.length);
	});
	it('give Party the same colour as a mood and as an activity', () => {
		expect(preferredColour('mood:party')).toBe(preferredColour('activity:party'));
	});
	it('give an old chart slug the colour of its page', () => {
		expect(preferredColour('genre:hip-hop-rap')).toBe(preferredColour('genre:hip-hop'));
		expect(preferredColour('genre:randb-soul')).toBe(preferredColour('genre:rnb'));
	});
});

describe('colour by hash', () => {
	it('gives an unlisted key one colour, the same every time', () => {
		const a = preferredColour('genre:polka');
		expect(a).toBeGreaterThanOrEqual(0);
		expect(a).toBeLessThan(TILE_PALETTE.length);
		expect(preferredColour('genre:polka')).toBe(a);
		expect(preferredColour('polka')).toBe(a);
	});
	it('spreads unlisted keys over the palette', () => {
		const seen = new Set(Array.from({ length: 200 }, (_, i) => preferredColour(`genre:tag-${i}`)));
		expect(seen.size).toBeGreaterThan(10);
	});
});

describe('no repeats on a grid', () => {
	const hexes = (keys: string[]) => assignTileColours(keys).map((c) => c.hex);

	it('gives a tile its own colour when nothing before it has it', () => {
		const out = assignTileColours(['genre:pop', 'genre:rock']);
		expect(out.map((c) => c.name)).toEqual(['pink', 'red']);
	});
	it('steps to the next unused colour when the colour is taken', () => {
		// Both ask for pink: the second steps on to terracotta, the next one in the palette.
		const out = assignTileColours(['genre:pop', 'genre:christian']);
		expect(out.map((c) => c.name)).toEqual(['pink', 'terracotta']);
	});
	it('never repeats a colour on a grid of up to sixteen tiles, whatever they ask for', () => {
		const same = Array.from({ length: 16 }, (_, i) => `genre:pop-${i % 1}-${i}`);
		expect(new Set(hexes(same)).size).toBe(16);
		const asking = Array.from({ length: 16 }, () => 'genre:pop');
		expect(new Set(hexes(asking)).size).toBe(16);
	});
	it('gives every kind of Browse page a grid with no repeat', () => {
		for (const keys of [GENRES.slice(0, 16), MOODS, SEASONS, DECADES, ACTIVITIES]) expect(new Set(hexes(keys)).size).toBe(keys.length);
	});
	it('lets the Explore mix show distinct colours across kinds', () => {
		const mix = ['genre:pop', 'genre:hip-hop', 'mood:chill', 'genre:rock', 'season:halloween', 'genre:latin', 'decade:1990s', 'mood:party', 'genre:rnb', 'activity:driving', 'genre:country', 'mood:love', 'decade:2000s', 'genre:dance'];
		expect(new Set(hexes(mix)).size).toBe(mix.length);
	});
	it('keeps a fixed colour when nothing collides, on any page', () => {
		const alone = assignTileColours(['genre:metal'])[0].hex;
		const among = assignTileColours(['genre:pop', 'mood:chill', 'genre:metal'])[2].hex;
		expect(among).toBe(alone);
	});
	it('on a grid longer than the palette repeats only after RECENT tiles', () => {
		const keys = [...GENRES, ...MOODS, ...SEASONS, ...DECADES, ...ACTIVITIES];
		const out = hexes(keys);
		expect(new Set(out.slice(0, 16)).size).toBe(16);
		for (let i = 0; i < out.length; i++) {
			for (let j = i + 1; j < Math.min(out.length, i + RECENT); j++) expect(out[j], `tiles ${i} and ${j}`).not.toBe(out[i]);
		}
	});
	it('gives a tile the same colour whatever comes after it', () => {
		const keys = [...GENRES, ...MOODS];
		const all = hexes(keys);
		for (const n of [1, 5, 12, 14, 20, keys.length]) expect(hexes(keys.slice(0, n))).toEqual(all.slice(0, n));
	});
	it('gives the same answer for the same keys', () => {
		const keys = [...GENRES, ...MOODS, ...DECADES];
		expect(hexes(keys)).toEqual(hexes(keys));
		expect(assignTileColours([])).toEqual([]);
	});
});
