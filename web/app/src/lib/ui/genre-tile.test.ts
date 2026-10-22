import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

// The tile keeps its own 16:9 box, so no parent grid can collapse it to a
// 0 px row. overflow: hidden made it a scroll container, whose aspect-ratio
// height a grid row does not count; clip rounds the corners without that.
describe('GenreTile', () => {
	const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'GenreTile.svelte'), 'utf8');
	const rule = src.match(/\.ui-gtile \{([^}]+)\}/)?.[1] ?? '';
	it('reserves its own 16:9 box', () => {
		expect(rule).toMatch(/width: 100%/);
		expect(rule).toMatch(/aspect-ratio: 16 \/ 9/);
	});
	it('clips instead of being a scroll container', () => {
		expect(rule).toMatch(/overflow: clip/);
		expect(rule).not.toMatch(/overflow: (hidden|auto|scroll)/);
	});
});

// The global .row utility wraps; a wrapping column flexbox sizes its grid at
// a 0 px width, so the Explore tile row must not wrap.
describe('Explore music tile row', () => {
	it('does not wrap', () => {
		const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), '..', 'MusicTrendingRows.svelte'), 'utf8');
		expect(src.match(/\.plain \{([^}]+)\}/)?.[1] ?? '').toMatch(/flex-wrap: nowrap/);
	});
});
