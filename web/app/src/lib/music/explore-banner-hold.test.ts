import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// Explore Music: the "Your genre" banner came in after the rows and pushed them down
// (CLS 0.15). Its place is held with the same banner (placeholders) until the rows answer.
const page = readFileSync(join(__dirname, '..', '..', 'routes', 'explore', '+page.svelte'), 'utf8');
const rows = readFileSync(join(__dirname, '..', 'MusicTrendingRows.svelte'), 'utf8');

describe('Explore Music banner place', () => {
	it('is held by the same banner until the music rows answer', () => {
		expect(page).toMatch(/bind:ready=\{musicReady\}/);
		expect(page).toMatch(/musicBannerHold = \$derived\(type === 'music' && !musicFav && !musicReady/);
		const hold = page.indexOf('{#if musicBannerHold}');
		const real = page.indexOf('{:else if musicBannerShown && musicFav}');
		expect(hold).toBeGreaterThan(0);
		expect(real).toBeGreaterThan(hold);
		// The held banner is the same component with the same words, so the exact height.
		expect(page.slice(hold, real)).toMatch(/<GenreBanner layout=\{2\}[^>]*eyebrow="Picked from your library" line="Albums and artists from it you could ask for\."/);
	});

	it('the rows say ready only once the genres are whole', () => {
		expect(rows).toMatch(/ready = \$bindable\(false\)/);
		expect(rows).toMatch(/ready = !a\.partial;/);
	});
});
