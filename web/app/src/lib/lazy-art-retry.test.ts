import { describe, expect, it } from 'vitest';
import { RETRY_MS, retried } from './lazy-art.ts';

describe('music picture retries', () => {
	it('asks twice more, waiting longer the second time', () => {
		expect(RETRY_MS).toHaveLength(2);
		expect(RETRY_MS[1]).toBeGreaterThan(RETRY_MS[0]);
	});
	it('marks the address so the browser asks again, keeping its query', () => {
		expect(retried('/api/v1/music-trending/art/album/a', 0)).toBe('/api/v1/music-trending/art/album/a');
		expect(retried('/api/v1/music-trending/art/album/a', 1)).toBe('/api/v1/music-trending/art/album/a?r=1');
		expect(retried('/api/v1/music-trending/art/album/a?size=500', 2)).toBe('/api/v1/music-trending/art/album/a?size=500&r=2');
	});
});
