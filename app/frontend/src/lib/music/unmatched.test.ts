import { describe, expect, it } from 'vitest';
import { unmatchedReason } from './unmatched.ts';

describe('unmatchedReason', () => {
	it('words each music reason, the by-hand ones and an unknown key', () => {
		expect(unmatchedReason('music_duplicate')).toBe('music.unmatched.reason.music_duplicate');
		expect(unmatchedReason('music_other_artist')).toBe('music.unmatched.reason.music_other_artist');
		expect(unmatchedReason('unmatched')).toBe('discover.leftovers.reason.unmatched');
		expect(unmatchedReason('replaced_by_hand')).toBe('discover.leftovers.reason.replaced_by_hand');
		expect(unmatchedReason('something_new')).toBe('music.unmatched.reason.music_no_album');
	});
});
