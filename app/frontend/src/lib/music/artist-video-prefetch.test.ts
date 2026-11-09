import { describe, expect, it, vi } from 'vitest';

vi.mock('../core/trailer/autoplay.svelte.ts', () => ({ prefetchArtistVideo: () => {}, watchArtistVideo: () => () => {} }));

const { artistOfHref } = await import('./artist-video-prefetch.ts');

describe('artistOfHref', () => {
	it('reads the artist from page links, with or without a base', () => {
		expect(artistOfHref('/music/artist/48262E82-db9f-4a92-b650-dfef979b73ec')).toBe('48262e82-db9f-4a92-b650-dfef979b73ec');
		expect(artistOfHref('/holyarrs/music/artist/48262e82-db9f-4a92-b650-dfef979b73ec?from=explore')).toBe('48262e82-db9f-4a92-b650-dfef979b73ec');
	});
	it('ignores other links', () => {
		expect(artistOfHref('/music/album/48262e82-db9f-4a92-b650-dfef979b73ec')).toBeNull();
		expect(artistOfHref('/music/artist/nope')).toBeNull();
		expect(artistOfHref(null)).toBeNull();
	});
});
