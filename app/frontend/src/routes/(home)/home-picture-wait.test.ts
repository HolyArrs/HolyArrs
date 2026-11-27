import { describe, expect, it } from 'vitest';
import { pictureWait } from './home.ts';

// Home held the loading screen on every visit, for up to 30 s, while its tile pictures downloaded.
describe('pictureWait', () => {
	it('waits briefly for pictures on the first Home of the session only', () => {
		const first = pictureWait();
		expect(first).toBeGreaterThan(0);
		expect(first).toBeLessThanOrEqual(1500);
		expect(pictureWait()).toBe(0);
		expect(pictureWait()).toBe(0);
	});
});
