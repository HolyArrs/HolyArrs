// Every link to an artist page (rows, cards, similar artists, credits) gets the trailers' two moments: on screen for a while, the artist is named in the in-view batch; at hover start, its video is asked for.
import { prefetchArtistVideo, watchArtistVideo } from '../core/trailer/autoplay.svelte.ts';
import { minimalLook } from '../core/look.svelte.ts';

const ARTIST = /\/music\/artist\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?:[/?#]|$)/i;

/** The MusicBrainz id an artist page link names; null when it is not one. */
export function artistOfHref(href: string | null | undefined): string | null {
	const m = href?.match(ARTIST);
	return m ? m[1].toLowerCase() : null;
}

/** Watches artist links in view and on hover. Returns the cleanup. */
export function installArtistVideoPrefetch(): () => void {
	if (typeof window === 'undefined' || typeof MutationObserver === 'undefined') return () => {};
	const stops = new Map<Element, () => void>();
	const scan = (root: ParentNode) => {
		// The minimal look plays no videos, so it asks for none.
		if (minimalLook()) return;
		for (const a of root.querySelectorAll?.('a[href*="/music/artist/"]') ?? []) {
			if (stops.has(a)) continue;
			const mbid = artistOfHref(a.getAttribute('href'));
			if (mbid) stops.set(a, watchArtistVideo(a, mbid));
		}
	};
	const sweep = () => {
		for (const [a, stop] of stops) {
			if (!a.isConnected) {
				stop();
				stops.delete(a);
			}
		}
	};
	let queued = false;
	const mo = new MutationObserver(() => {
		if (queued) return;
		queued = true;
		requestAnimationFrame(() => {
			queued = false;
			sweep();
			scan(document);
		});
	});
	mo.observe(document.body, { childList: true, subtree: true });
	scan(document);
	const over = (e: PointerEvent) => {
		if (e.pointerType !== 'mouse' || minimalLook()) return;
		const mbid = artistOfHref((e.target as Element | null)?.closest?.('a[href]')?.getAttribute('href'));
		if (mbid) prefetchArtistVideo(mbid);
	};
	window.addEventListener('pointerover', over, { capture: true, passive: true });
	return () => {
		mo.disconnect();
		window.removeEventListener('pointerover', over, { capture: true });
		for (const stop of stops.values()) stop();
		stops.clear();
	};
}
