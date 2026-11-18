import { MEMBER_LINE, outcomeLine, outcomeNote, whyWords } from './search-outcome.ts';
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
		// A Source left out for its tags is not called paused.
		expect(whyWords('its tags (kids) are not on this title')).toBe('its tags (kids) are not on this title');
		expect(whyWords("its tags (kids) are not in the Pick rules' tags (hd)")).toBe("its tags (kids) are not in the Pick rules' tags (hd)");
		expect(whyWords('it has no tags, and the Pick rules only take Sources tagged hd')).toBe('it has no tags, and the Pick rules only take Sources tagged hd');
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
	it('passes on what the search said about the folder tags', () => {
		const note = "No Source this search can use has this folder's tags, so Sources without tags were asked too.";
		expect(outcomeNote({ empty: true, sources: [], note })).toBe(note);
		expect(outcomeNote({ empty: true, sources: [] })).toBe('');
		expect(outcomeNote({ empty: false, note })).toBe('');
	});
});
