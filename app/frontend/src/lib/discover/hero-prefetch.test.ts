import { afterEach, describe, expect, it, vi } from 'vitest';
import { heroAhead } from './hero-prefetch.ts';
import { setLook } from '../core/look.svelte.ts';
import { registerPictures } from '../core/pictures.ts';

// Two artists with a stored wide picture, so heroPictures has something to warm in either look.
const QUIET = '0383dadf-2a4e-4d10-a46a-e9e041da8eb3';
const LOUD = '1f9df192-a621-4f54-8850-2c5373b7eac9';
registerPictures({ [`wide:${QUIET}`]: { v: 'a1', w: 1920, h: 1080 }, [`wide:${LOUD}`]: { v: 'a1', w: 1920, h: 1080 } });

function catchImages(): string[] {
	const made: string[] = [];
	vi.stubGlobal('Image', class { decoding = ''; set src(u: string) { made.push(u); } });
	return made;
}

describe('heroAhead', () => {
	afterEach(() => {
		setLook('full');
		vi.unstubAllGlobals();
	});
	it('warms nothing in the minimal look', () => {
		const made = catchImages();
		const preload = vi.fn();
		setLook('minimal');
		heroAhead('/title/movie/550', preload);
		heroAhead(`/music/artist/${QUIET}`, preload);
		expect(preload).not.toHaveBeenCalled();
		expect(made).toEqual([]);
	});
	it('warms the artist picture and hands a title link to the preloader in the full look', () => {
		const made = catchImages();
		const preload = vi.fn();
		heroAhead('/title/movie/550', preload);
		heroAhead(`/music/artist/${LOUD}`, preload);
		expect(preload).toHaveBeenCalledTimes(1);
		expect(made.some((u) => u.includes(`/music-trending/art/wide/${LOUD}`))).toBe(true);
	});
});
