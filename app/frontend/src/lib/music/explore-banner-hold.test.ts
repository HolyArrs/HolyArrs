import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// Explore Music: the "Your genre" banner came in after the rows and pushed them down. Its place is held with the same banner (placeholders) until the rows answer.
const page = readFileSync(join(__dirname, '..', '..', 'routes', 'explore', '+page.svelte'), 'utf8');
const banner = readFileSync(join(__dirname, '..', 'discover', 'explore-banner.svelte.ts'), 'utf8');
const rows = readFileSync(join(__dirname, 'MusicTrendingRows.svelte'), 'utf8');

describe('Explore Music banner place', () => {
	it('is held by the same banner until the music rows answer', () => {
		expect(page).toMatch(/bind:ready=\{bannerView\.musicReady\}/);
		expect(banner).toMatch(/musicBannerHold = \$derived\(ctx\.type === 'music' && !musicFav && !musicReady/);
		const hold = page.indexOf('{#if bannerView.musicBannerHold}');
		const real = page.indexOf('{:else if bannerView.musicBannerShown && bannerView.musicFav}');
		expect(hold).toBeGreaterThan(0);
		expect(real).toBeGreaterThan(hold);
		// The held banner is the same component with the same words, so the exact height.
		expect(page.slice(hold, real)).toMatch(/name=\{t\('discover\.explore\.yourName', \{ name: bannerView\.musicBannerName \|\| t\('discover\.explore\.music'\) \}\)\}/);
		expect(page.slice(hold, real)).toMatch(/<GenreBanner layout=\{2\}[^>]*eyebrow=\{t\('discover\.explore\.pickedFromYourLibrary'\)\} line=\{t\('discover\.explore\.albumsAndArtistsFrom'\)\}/);
	});

	it('placeholder picks are as tall as the captionless covers', () => {
		const gb = readFileSync(join(__dirname, '..', 'discover', 'GenreBanner.svelte'), 'utf8');
		expect(gb).toMatch(/\.l2 \.picks :global\(\.sk-line\) \{ display: none; \}/);
	});

	it('the rows say ready only once the genres are whole', () => {
		expect(rows).toMatch(/ready = \$bindable\(false\)/);
		expect(rows).toMatch(/ready = !a\.partial;/);
	});
});
