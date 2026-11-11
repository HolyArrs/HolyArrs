// Heroes the user is about to open get their big pictures fetched ahead so the page's hero draws at once. A mouse resting on a card (80 ms) or keyboard focus on one preloads its hero; focus also preloads the next card's.
import { appPicture, coverFor } from '../music/index.ts';
import { remembered, type DetailKind } from '../metadata/index.ts';
import { pictureFor } from '../core/pictures.ts';
import { proxied } from '../core/tmdb-image.ts';
import { minimalLook } from '../core/look.svelte.ts';

const album = /\/music\/album\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?:[?#]|$)/;
const artist = /\/music\/artist\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?:[?#]|$)/;
const title = /\/title\/(movie|show)\/(\d+)(?:[?#/]|$)/;

/** The big pictures a music page shows first: an album's hero cover, an artist's wide background and logo (stored ones only). */
export function heroPictures(href: string, ratio = globalThis.devicePixelRatio || 1): string[] {
	let m = album.exec(href);
	if (m) return pictureFor('album', m[1]) ? [coverFor(appPicture(`/api/v1/music-trending/art/album/${m[1]}`), 260, ratio, true).src].filter((u): u is string => !!u) : [];
	m = artist.exec(href);
	if (!m) return [];
	const id = m[1];
	return (['wide', 'logo'] as const).filter((k) => pictureFor(k, id)).map((k) => appPicture(`/api/v1/music-trending/art/${k}/${id}`) as string);
}

const asked = new Set<string>();

/** Preloads the hero behind a link: music pictures here, a title's backdrop through `preload` (the title page's own preloadHero). */
export function heroAhead(href: string | null | undefined, preload: (backdrop: string | null | undefined) => void): void {
	// The minimal look shows no big pictures, so none are fetched ahead.
	if (!href || minimalLook()) return;
	for (const u of heroPictures(href)) warm(u);
	const t = title.exec(href);
	if (t) preload(remembered(t[1] as DetailKind, t[2])?.backdrop);
}

/** Watches hovers and focus for the whole app; returns its own removal. */
export function installHeroAhead(preload: (backdrop: string | null | undefined) => void): () => void {
	let timer: ReturnType<typeof setTimeout> | undefined;
	const link = (e: Event) => (e.target as Element | null)?.closest?.('a[href]') as HTMLAnchorElement | null;
	const over = (e: PointerEvent) => {
		if (e.pointerType !== 'mouse') return;
		clearTimeout(timer);
		const a = link(e);
		if (a) timer = setTimeout(() => heroAhead(a.getAttribute('href'), preload), 80);
	};
	const focus = (e: FocusEvent) => {
		const a = link(e);
		if (!a) return;
		heroAhead(a.getAttribute('href'), preload);
		// The next card in the row is the likely next stop.
		const row = a.closest('.row');
		if (!row) return;
		const hits = [...row.querySelectorAll<HTMLAnchorElement>('a.hit[href]')];
		const i = hits.indexOf(a);
		if (i >= 0 && hits[i + 1]) heroAhead(hits[i + 1].getAttribute('href'), preload);
	};
	window.addEventListener('pointerover', over, { capture: true, passive: true });
	window.addEventListener('focusin', focus, { capture: true });
	return () => {
		clearTimeout(timer);
		window.removeEventListener('pointerover', over, { capture: true });
		window.removeEventListener('focusin', focus, { capture: true });
	};
}

function warm(url: string): void {
	if (asked.has(url)) return;
	asked.add(url);
	const im = new Image();
	im.decoding = 'async';
	im.src = proxied(url);
}
