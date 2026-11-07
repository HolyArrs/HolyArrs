import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { missLine } from './trailer-miss.ts';

describe('missLine', () => {
	it('says why a video will not play, in plain words', () => {
		expect(missLine('age_restricted')).toBe("YouTube plays this trailer only for signed-in adults, so it can't play here.");
		expect(missLine('unavailable')).toBe('This trailer was removed from YouTube.');
		expect(missLine('sign_in')).toBe('This trailer is private or for members only on YouTube.');
		expect(missLine('unplayable')).toBe("YouTube won't play this trailer here.");
		expect(missLine('content_check')).toBe('YouTube shows this trailer only after a content warning.');
	});
	it('says trailers cannot be reached when YouTube refuses HolyArrs as a whole', () => {
		for (const why of ['bot_check', 'no_address', 'refused', 'network']) {
			expect(missLine(why)).toBe("Trailers can't be reached right now.");
		}
	});
	it("falls back to can't play here without a reason, or one it does not know", () => {
		expect(missLine(undefined)).toBe("This trailer can't play here.");
		expect(missLine('')).toBe("This trailer can't play here.");
		expect(missLine('bad_id')).toBe("This trailer can't play here.");
		expect(missLine('something_new')).toBe("This trailer can't play here.");
	});
	it('names what plays', () => {
		expect(missLine('unavailable', 'teaser')).toBe('This teaser was removed from YouTube.');
		expect(missLine(undefined, 'music video')).toBe("This music video can't play here.");
	});
	it('never shows a reason code', () => {
		for (const why of ['age_restricted', 'unavailable', 'sign_in', 'unplayable', 'content_check', 'bot_check', 'no_address', 'refused', 'network', 'bad_id']) {
			expect(missLine(why)).not.toMatch(/_|\bbot\b|\brefused\b|\bnetwork\b/);
		}
	});
});

describe('players use the shared line', () => {
	const theater = readFileSync(new URL('../../../routes/(frame)/Theater.svelte', import.meta.url), 'utf8');
	const trailers = readFileSync(new URL('../../discover/TitleTrailers.svelte', import.meta.url), 'utf8');
	it('the full-screen player shows missLine with the reason that rides on the embed', () => {
		expect(theater).toContain("import { missLine } from '#lib/core/trailer/trailer-miss.ts';");
		expect(theater).toContain('<p>{missLine(why, word)}</p>');
		expect(theater).toMatch(/s\.kind === 'embed' && s\.why\) return s\.why/);
		expect(theater).not.toMatch(/<p>[^<]*can't play here/);
	});
	it("a title's trailer row hands the main trailer's reason to the YouTube player", () => {
		expect(trailers).toContain('missReason(kind, tmdb)');
		expect(trailers).toContain("{ kind: 'embed', key: v.key, why, told: videoReason(why) || undefined }");
	});
	it("the title page's Trailer button opens the player with the reason when nothing plays", () => {
		const hero = readFileSync(new URL('../../ui/TitleHero.svelte', import.meta.url), 'utf8');
		expect(hero).toContain('await trailerClickSources(kind, tmdb, videos)');
		expect(hero).not.toMatch(/await trailerSources\(/);
	});
	it('a reason the server already gave shows at once, with no YouTube player behind it', () => {
		expect(theater).toMatch(/unavailable = now\.source\.kind === 'embed' && !!now\.source\.told && !now\.fallbacks\?\.length/);
		expect(theater).toContain("{#if !mediaOn || (src.kind === 'embed' && src.told)}");
	});
	it('the line wraps on a phone', () => {
		expect(theater).toMatch(/\.gone \{[^}]*padding: 24px/);
		expect(theater).toMatch(/\.gone p \{[^}]*max-width: 32ch/);
	});
});
