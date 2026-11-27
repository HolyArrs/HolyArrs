import { pickLoadFrom, ymd } from './calendar.ts';
import { describe, expect, it } from 'vitest';

// a day picked in the small month more than a year back did nothing.
describe('pickLoadFrom', () => {
	const start = new Date('2025-11-26T00:00:00'); // the days loaded start a year back
	it('loads back to a day older than a year', () => {
		const d = pickLoadFrom('2025-11-16', start);
		expect(d && ymd(d)).toBe('2025-11-16');
	});
	it('asks nothing when the day is already loaded', () => {
		expect(pickLoadFrom('2025-12-01', start)).toBeNull();
		expect(pickLoadFrom('2025-11-26', start)).toBeNull();
	});
});
