import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { HLS_VERSION, hlsScript } from './hls-player.ts';

describe('hls.js', () => {
	it('is pinned to one exact version, shipped as its own file', () => {
		const pkg = JSON.parse(readFileSync(new URL('../../package.json', import.meta.url), 'utf8'));
		expect(pkg.dependencies['hls.js']).toBe(HLS_VERSION);
		expect(hlsScript).toBe(`/vendor/hls-${HLS_VERSION}.min.js`);
	});
});
