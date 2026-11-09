import { shortSynopsis } from './synopsis.ts';
import { describe, expect, it } from 'vitest';

describe('shortSynopsis', () => {
	it('keeps short text as is', () => {
		expect(shortSynopsis('A man climbs a mountain.')).toBe('A man climbs a mountain.');
	});
	it('keeps whole sentences under the limit', () => {
		const t = 'One sentence here. ' + 'Two '.repeat(40) + 'end.';
		expect(shortSynopsis(t, 60)).toBe('One sentence here.');
	});
	it('cuts a long first sentence at a word with an ellipsis', () => {
		const t = 'word '.repeat(60).trim() + '.';
		const s = shortSynopsis(t, 50);
		expect(s.endsWith('…')).toBe(true);
		expect(s.length).toBeLessThanOrEqual(51);
	});
	it('handles empty input', () => {
		expect(shortSynopsis(undefined)).toBe('');
	});
});
