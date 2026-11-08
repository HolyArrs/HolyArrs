import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

// The global .row utility wraps, and a wrapping column flexbox sizes its grid at a 0 px width, so the Explore tile row must not wrap.
describe('Explore music tile row', () => {
	it('does not wrap', () => {
		const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'MusicTrendingRows.svelte'), 'utf8');
		expect(src.match(/\.plain \{([^}]+)\}/)?.[1] ?? '').toMatch(/flex-wrap: nowrap/);
	});
});
