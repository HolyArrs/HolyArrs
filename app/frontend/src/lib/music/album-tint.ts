// An album page's colour: the whole page sits on a deep, muted colour taken from the cover (dark olive for green, deep maroon for red, navy for blue, near black for dark), with light text on it.

export type Rgb = [number, number, number];

/** The page colour set: the page, the lower band and the text tones. */
export interface AlbumTint {
	bg: string;
	band: string;
	text: string;
	soft: string;
	rule: string;
	hover: string;
	/** White text on bg (n:1). */
	contrast: number;
	/** The soft text tone on bg (n:1). */
	softContrast: number;
}

/** The tint of a page whose cover gave no colour: a quiet near black. */
export const FALLBACK = '#1d1d22';

/** The softer text's white share. */
export const SOFT = 0.72;

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));

export function hexRgb(hex: string): Rgb | null {
	const m = /^#?([0-9a-f]{6})$/i.exec(hex.trim());
	if (!m) return null;
	const n = parseInt(m[1], 16);
	return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

export function rgbHex([r, g, b]: Rgb): string {
	return `#${[r, g, b].map((v) => Math.round(clamp(v, 0, 255)).toString(16).padStart(2, '0')).join('')}`;
}

export function rgbHsl([r, g, b]: Rgb): [number, number, number] {
	const R = r / 255, G = g / 255, B = b / 255;
	const max = Math.max(R, G, B), min = Math.min(R, G, B);
	const l = (max + min) / 2;
	if (max === min) return [0, 0, l];
	const d = max - min;
	const s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
	const h = max === R ? (G - B) / d + (G < B ? 6 : 0) : max === G ? (B - R) / d + 2 : (R - G) / d + 4;
	return [h * 60, s, l];
}

export function hslRgb([h, s, l]: [number, number, number]): Rgb {
	const k = (n: number) => (n + h / 30) % 12;
	const a = s * Math.min(l, 1 - l);
	const f = (n: number) => l - a * Math.max(-1, Math.min(k(n) - 3, 9 - k(n), 1));
	return [f(0) * 255, f(8) * 255, f(4) * 255];
}

/** The WCAG contrast ratio of two colours. */
export function contrast(a: Rgb, b: Rgb): number {
	const x = lum(a), y = lum(b);
	return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

/** Turns a cover colour into the page colour: muted, dark, with light text at AA (soft) and AAA (white). */
export function deepen(hex: string | null | undefined): string {
	const rgb = hex ? hexRgb(hex) : null;
	if (!rgb) return FALLBACK;
	const [h, s0, l0] = rgbHsl(rgb);
	// A grey or very dark cover gets a near black with a breath of its hue.
	if (s0 < 0.1 || l0 < 0.1) return rgbHex(hslRgb([h, Math.min(s0, 0.1), 0.12]));
	const s = clamp(s0 * 0.7, 0.22, 0.55);
	let l = clamp(l0 * 0.6, 0.14, 0.3);
	const white: Rgb = [255, 255, 255];
	for (let i = 0; i < 40; i++) {
		const c = hslRgb([h, s, l]);
		if (contrast(white, c) >= 7 && contrast(overWhite(c, SOFT), c) >= 4.5) break;
		l -= 0.01;
	}
	return rgbHex(hslRgb([h, s, Math.max(l, 0.06)]));
}

/** The page colour set for a cover colour. */
export function tintOf(cover: string | null | undefined): AlbumTint {
	const bg = deepen(cover);
	const c = hexRgb(bg) as Rgb;
	const [h, s, l] = rgbHsl(c);
	const band = rgbHex(hslRgb([h, s, Math.max(0.04, l - 0.04)]));
	const soft = overWhite(c, SOFT);
	return {
		bg,
		band,
		text: '#ffffff',
		soft: rgbHex(soft),
		rule: 'rgb(255 255 255 / 14%)',
		hover: 'rgb(255 255 255 / 9%)',
		contrast: Math.round(contrast([255, 255, 255], c) * 100) / 100,
		softContrast: Math.round(contrast(soft, c) * 100) / 100
	};
}

/** Picks a cover's main colour from its pixels (RGBA bytes). Pixels are grouped by hue in 24 bins and weighted by saturation, skipping near white and near black; the heaviest bin's average wins. */
export function dominant(px: Uint8ClampedArray | number[]): string | null {
	const bins = Array.from({ length: 24 }, () => ({ w: 0, r: 0, g: 0, b: 0 }));
	let n = 0, ar = 0, ag = 0, ab = 0;
	for (let i = 0; i + 3 < px.length; i += 4) {
		if (px[i + 3] < 128) continue;
		const rgb: Rgb = [px[i], px[i + 1], px[i + 2]];
		n++; ar += rgb[0]; ag += rgb[1]; ab += rgb[2];
		const [h, s, l] = rgbHsl(rgb);
		if (l < 0.08 || l > 0.94 || s < 0.15) continue;
		const w = s * (1 - Math.abs(l - 0.5));
		const bin = bins[Math.floor(h / 15) % 24];
		bin.w += w; bin.r += rgb[0] * w; bin.g += rgb[1] * w; bin.b += rgb[2] * w;
	}
	if (!n) return null;
	const best = bins.reduce((a, b) => (b.w > a.w ? b : a));
	// The colour must cover a fair share of the picture, else the cover reads as its average.
	if (best.w < n * 0.06) return rgbHex([ar / n, ag / n, ab / n]);
	return rgbHex([best.r / best.w, best.g / best.w, best.b / best.w]);
}

/** Reads a loaded cover image on a 32px canvas; null when it cannot (not same origin, no canvas). */
export function sampleCover(img: HTMLImageElement): string | null {
	try {
		const c = document.createElement('canvas');
		c.width = c.height = 32;
		const g = c.getContext('2d', { willReadFrequently: true });
		if (!g) return null;
		g.drawImage(img, 0, 0, 32, 32);
		return dominant(g.getImageData(0, 0, 32, 32).data);
	} catch {
		return null;
	}
}

let leaving: ReturnType<typeof setTimeout> | undefined;

/** Puts an album colour on the whole window: <html data-tint="album"> and --tint-* (app.css). */
export function paintWindow(t: AlbumTint, root: HTMLElement = document.documentElement): void {
	clearTimeout(leaving);
	const s = root.style;
	s.setProperty('--tint-bg', t.bg);
	s.setProperty('--tint-bg-2', t.band);
	s.setProperty('--tint-ink', t.text);
	s.setProperty('--tint-soft', t.soft);
	s.setProperty('--tint-line', t.rule);
	root.dataset.tint = 'album';
}

/** Fades the window back: "leaving" keeps the transitions for one slow beat, then everything is removed. */
export function unpaintWindow(root: HTMLElement = document.documentElement, ms = 400): void {
	if (root.dataset.tint !== 'album') return;
	root.dataset.tint = 'leaving';
	clearTimeout(leaving);
	leaving = setTimeout(() => {
		if (root.dataset.tint !== 'leaving') return;
		delete root.dataset.tint;
		for (const k of ['--tint-bg', '--tint-bg-2', '--tint-ink', '--tint-soft', '--tint-line']) root.style.removeProperty(k);
	}, ms);
}

function lum([r, g, b]: Rgb): number {
	const c = (v: number) => {
		const x = v / 255;
		return x <= 0.03928 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4;
	};
	return 0.2126 * c(r) + 0.7152 * c(g) + 0.0722 * c(b);
}

/** Blends white over a colour by a share (0 to 1). */
function overWhite(c: Rgb, share: number): Rgb {
	return [c[0] + (255 - c[0]) * share, c[1] + (255 - c[1]) * share, c[2] + (255 - c[2]) * share];
}
