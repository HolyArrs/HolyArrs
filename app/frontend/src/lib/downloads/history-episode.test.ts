import { historyQuery, readFilter } from './history.ts';
import { describe, expect, it } from 'vitest';

describe('history of one episode', () => {
	it('asks for the episode and keeps it in the address', () => {
		const f = readFilter(new URLSearchParams('title=3&episode=41'));
		expect(f.episode).toBe(41);
		expect(new URLSearchParams(historyQuery(f)).get('episode')).toBe('41');
		expect(new URLSearchParams(historyQuery(f)).get('title')).toBeNull();
		expect(readFilter(new URLSearchParams('episode=x')).episode).toBeUndefined();
	});
});
