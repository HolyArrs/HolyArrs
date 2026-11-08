import { MEMBER_LINE, outcomeLine, whyWords } from './search-outcome.ts';
import { describe, expect, it } from 'vitest';

describe('search outcome', () => {
	it('tells pause reasons apart', () => {
		expect(whyWords('rate_limited')).toBe('was paused (slowed down)');
		expect(whyWords('down')).toBe('was down');
		expect(whyWords('blocked')).toBe('was blocked');
		expect(whyWords('bot_check')).toBe("was blocked by the site's bot check");
		expect(whyWords('site_ban')).toBe("refuses HolyArrs's address");
		expect(whyWords('login_refused')).toBe('was key refused');
		expect(whyWords('broken_definition')).toBe('is broken');
		expect(whyWords('something')).toBe('was paused');
	});
	it('says nothing until a search came back empty', () => {
		expect(outcomeLine(null)).toBe('');
		expect(outcomeLine({ empty: false })).toBe('');
	});
	it('gives members the plain line and admins the Sources', () => {
		expect(outcomeLine({ empty: true })).toBe(MEMBER_LINE);
		expect(outcomeLine({ empty: true, sources: [] })).toBe(MEMBER_LINE);
		expect(outcomeLine({ empty: true, sources: [{ id: 1, name: 'A', why: 'down' }] })).toBe('Nothing found:');
	});
});
