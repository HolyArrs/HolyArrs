import {
	applySize,
	blankBody,
	bytesPerMinute,
	checkBody,
	checkOwnTrait,
	copyName,
	fieldKey,
	gbPerHour,
	isMusic,
	ladderFor,
	musicLadder,
	neverChoices,
	ownTerms,
	sizeFields,
	stepLabel,
	tidyBody,
	toggleKind,
	traitLabel,
	traitsFor,
	usedWords,
	videoLadder
} from './pick-rules.ts';
import type { OwnTrait, PickRulesBody } from './types.ts';
import { describe, expect, it } from 'vitest';

const gb = 2 ** 30;

describe('ladders', () => {
	it('video kinds use the video ladder and music uses its own', () => {
		expect(ladderFor(['movie'])).toEqual(videoLadder);
		expect(ladderFor(['movie', 'show'])).toEqual(videoLadder);
		expect(ladderFor(['music'])).toEqual(musicLadder);
		expect(videoLadder[0]).toBe('cam');
		expect(videoLadder.at(-1)).toBe('2160p_remux');
		expect(musicLadder).toEqual(['lossy_low', 'lossy_mid', 'lossy_high', 'lossless', 'lossless_hires']);
		expect(isMusic(['music'])).toBe(true);
		expect(isMusic(['show'])).toBe(false);
	});

	it('names steps in plain words', () => {
		expect(stepLabel('1080p_web')).toBe('1080p web');
		expect(stepLabel('720p_bluray')).toBe('720p Blu-ray');
		expect(stepLabel('2160p_remux')).toBe('4K remux');
		expect(stepLabel('sd_dvd')).toBe('DVD');
		expect(stepLabel('lossless')).toBe('Lossless (CD quality)');
		expect(stepLabel('odd_step')).toBe('odd step');
	});
});

describe('traits', () => {
	it('names traits in plain words, unknown ones as they are', () => {
		expect(traitLabel('hevc')).toBe('HEVC (smaller files)');
		expect(traitLabel('hardcoded_subs')).toBe('Subtitles burned into the picture');
		expect(traitLabel('cd_source')).toBe('Ripped from a CD');
		expect(traitLabel('sample')).toBe('Only a short sample');
		expect(traitLabel('new_thing')).toBe('new thing');
	});

	it('offers the kind’s own traits, and the safety traits only for Never', () => {
		expect(traitsFor(['movie'])).toContain('hevc');
		expect(traitsFor(['movie'])).not.toContain('flac');
		expect(traitsFor(['music'])).toContain('flac');
		expect(traitsFor(['music'])).not.toContain('sample');
		expect(neverChoices(['show'])).toContain('sample');
		expect(neverChoices(['show'])).toContain('3d');
		expect(neverChoices(['music'])).toContain('lossy_transcode');
	});
});

describe('kinds', () => {
	it('music stands alone; movies and TV shows go together', () => {
		expect(toggleKind(['movie'], 'show', true)).toEqual(['movie', 'show']);
		expect(toggleKind(['movie', 'show'], 'music', true)).toEqual(['music']);
		expect(toggleKind(['music'], 'movie', true)).toEqual(['movie']);
		expect(toggleKind(['movie', 'show'], 'movie', false)).toEqual(['show']);
		expect(toggleKind(['show', 'movie'], 'show', true)).toEqual(['movie', 'show']);
	});
});

describe('blankBody', () => {
	it('starts video rules at 720p to 1080p, Good enough at 1080p web, with the safety Nevers', () => {
		const b = blankBody('movie');
		expect(b.steps.map((s) => s.quality)).toEqual(videoLadder);
		expect(b.steps.filter((s) => s.allowed).map((s) => s.quality)).toEqual(['720p_tv', '720p_web', '720p_bluray', '1080p_tv', '1080p_web', '1080p_bluray']);
		expect(b.good_enough_at).toBe('1080p_web');
		expect(b.never).toEqual(['sample', 'executable', 'password_protected', 'disc_image']);
		expect(b.protocol).toEqual({ prefer: 'either', wait_minutes_torrent: 0, wait_minutes_usenet: 0, skip_wait_at_good_enough: false });
		expect(b.order).toBe('quality_then_points');
		expect(b.proper_policy.mode).toBe('take_any');
		expect(checkBody('New rules', ['movie'], b)).toEqual({});
	});

	it('starts music rules on the music ladder, Good enough at lossless', () => {
		const b = blankBody('music');
		expect(b.steps.map((s) => s.quality)).toEqual(musicLadder);
		expect(b.good_enough_at).toBe('lossless');
		expect(b.size_limits).toEqual({});
		expect(checkBody('Albums', ['music'], b)).toEqual({});
	});

	it('gives a fresh copy each time', () => {
		const a = blankBody('show');
		a.never.push('3d');
		expect(blankBody('show').never).not.toContain('3d');
	});
});

describe('usedWords', () => {
	it('says where a rule is used, in plain words', () => {
		expect(usedWords({ versions: 3, library_folders: 1, outside_lists: 0, wishlist: 0, title_defaults: 0 })).toBe('Used by 3 Versions and 1 Library folder');
		expect(usedWords({ versions: 1, library_folders: 2, outside_lists: 1, wishlist: 1, title_defaults: 2 })).toBe(
			'Used by 1 Version, 2 Library folders, 1 Outside list, 1 Request rule and 2 Movies and TV defaults'
		);
		expect(usedWords({ versions: 0, library_folders: 0, outside_lists: 0, wishlist: 0, title_defaults: 0 })).toBe('Not used yet');
	});

	it('takes the total from the list too', () => {
		expect(usedWords(0)).toBe('Not used yet');
		expect(usedWords(1)).toBe('Used in 1 place');
		expect(usedWords(4)).toBe('Used in 4 places');
		expect(usedWords(undefined)).toBe('');
	});
});

describe('checkBody', () => {
	const good = () => blankBody('movie');

	it('needs a name of 1 to 200 characters', () => {
		expect(checkBody('  ', ['movie'], good()).name).toBe('Give these Pick rules a name.');
		expect(checkBody('x'.repeat(201), ['movie'], good()).name).toBe('A name can be at most 200 characters.');
		expect(checkBody('x'.repeat(200), ['movie'], good()).name).toBeUndefined();
	});

	it('needs one to three kinds, and music alone', () => {
		expect(checkBody('A', [], good()).kinds).toBe('Pick movies, shows or music.');
		expect(checkBody('A', ['movie', 'music'], good()).kinds).toBe('Music Pick rules are for music alone.');
		expect(checkBody('A', ['movie', 'movie'], good()).kinds).toBe('Each kind can be picked once.');
	});

	it('needs steps from the right ladder, each once, at least one on, and Good enough on one of them', () => {
		expect(checkBody('A', ['music'], good()).steps).toBe('These steps are for another kind.');
		const twice = good();
		twice.steps.push({ quality: 'cam', allowed: false });
		expect(checkBody('A', ['movie'], twice).steps).toBe('Each step can be listed once.');
		const none = good();
		none.steps = none.steps.map((s) => ({ ...s, allowed: false }));
		expect(checkBody('A', ['movie'], none).steps).toBe('Turn on at least one step.');
		const off = good();
		off.good_enough_at = 'cam';
		expect(checkBody('A', ['movie'], off).good_enough_at).toBe('Good enough must be a step that is on.');
	});

	it('checks traits and points', () => {
		const b = good();
		b.traits = [
			{ trait: 'hevc', points: 10 },
			{ trait: 'hevc', points: 5 }
		];
		expect(checkBody('A', ['movie'], b).traits).toBe('Each trait can be listed once.');
		b.traits = [{ trait: 'flac', points: 1 }];
		expect(checkBody('A', ['movie'], b).traits).toBe('flac is not a trait for these kinds.');
		b.traits = [{ trait: 'hevc', points: 10001 }];
		expect(checkBody('A', ['movie'], b).traits).toBe('Points go from -10000 to 10000.');
		const p = good();
		p.min_points = -10001;
		p.upgrade_min_points = 20000;
		expect(checkBody('A', ['movie'], p)).toMatchObject({ min_points: 'Points go from -10000 to 10000.', upgrade_min_points: 'Points go from -10000 to 10000.' });
	});

	it('keeps safety traits for Never only', () => {
		const b = good();
		b.must_have = ['sample'];
		expect(checkBody('A', ['movie'], b).must_have).toBe('Only a short sample cannot be a must-have.');
		b.must_have = ['hevc', 'hevc'];
		expect(checkBody('A', ['movie'], b).must_have).toBe('Each trait can be listed once.');
		b.must_have = [];
		b.never = ['sample', 'hdr10'];
		expect(checkBody('A', ['movie'], b).never).toBeUndefined();
	});

	it('checks sizes, waits, languages and the fixed-copy age', () => {
		const b = good();
		b.size_limits = { '*': { min: 10, max: 5, per_minute: true } };
		expect(checkBody('A', ['movie'], b).size_limits).toBe('The smallest size cannot be more than the largest.');
		b.size_limits = { '*': { min: -1, max: 5, per_minute: true } };
		expect(checkBody('A', ['movie'], b).size_limits).toBe('Sizes cannot be below 0.');
		b.size_limits = { lossless: { min: 1, max: 5, per_minute: true } };
		expect(checkBody('A', ['movie'], b).size_limits).toBe('A size is set for a step these rules do not have.');
		const w = good();
		w.protocol.wait_minutes_torrent = 10081;
		w.protocol.wait_minutes_usenet = -1;
		expect(checkBody('A', ['movie'], w)).toMatchObject({
			wait_minutes_torrent: 'A wait can be 0 to 10080 minutes (7 days).',
			wait_minutes_usenet: 'A wait can be 0 to 10080 minutes (7 days).'
		});
		const l = good();
		l.language.wanted = ['en', 'xx'];
		expect(checkBody('A', ['movie'], l).language).toBe('xx is not a language HolyArrs knows.');
		const a = good();
		a.proper_policy.max_age_days = 36501;
		expect(checkBody('A', ['movie'], a).max_age_days).toBe('Days go from 0 to 36500.');
	});
});

describe('sizes', () => {
	it('turns GB per hour into bytes per minute and back', () => {
		expect(bytesPerMinute(6)).toBe(Math.round((6 * gb) / 60));
		expect(gbPerHour(Math.floor((6 * gb) / 60))).toBe(6);
		expect(gbPerHour(Math.floor(gb / 2 / 60))).toBe(0.5);
		expect(bytesPerMinute(0)).toBe(0);
	});

	it('shows Every step as two numbers, an open top as empty', () => {
		expect(sizeFields({ '*': { min: Math.floor(gb / 2 / 60), max: Math.floor((6 * gb) / 60), per_minute: true } })).toEqual({ min: 0.5, max: 6 });
		expect(sizeFields({})).toEqual({ min: '', max: '' });
		expect(sizeFields({ '*': { min: 0, max: 0, per_minute: true } })).toEqual({ min: 0, max: '' });
		expect(sizeFields({ '*': { min: 5, max: 2 ** 50, per_minute: true } }).max).toBe('');
	});

	it('writes Every step back, keeping untouched values and the other steps as they were', () => {
		const start = { '*': { min: 8947848, max: 107374182, per_minute: true }, lossless: { min: 1, max: 2, per_minute: true } };
		const body = { size_limits: start } as unknown as PickRulesBody;
		expect(applySize(body, { min: 0.5, max: 6 }, { min: 0.5, max: 6 }).size_limits).toEqual(start);
		expect(applySize(body, { min: 1, max: 8 }, { min: 0.5, max: 6 }).size_limits).toEqual({
			'*': { min: bytesPerMinute(1), max: bytesPerMinute(8), per_minute: true },
			lossless: { min: 1, max: 2, per_minute: true }
		});
		expect(applySize(body, { min: '', max: '' }, { min: 0.5, max: 6 }).size_limits).toEqual({ lossless: { min: 1, max: 2, per_minute: true } });
		expect(applySize(body, { min: 1, max: '' }, { min: 0.5, max: 6 }).size_limits['*']).toEqual({ min: bytesPerMinute(1), max: 2 ** 50, per_minute: true });
		expect(applySize(body, { min: '', max: 3 }, { min: 0.5, max: 6 }).size_limits['*']).toEqual({ min: 0, max: bytesPerMinute(3), per_minute: true });
		expect(body.size_limits).toEqual(start);
	});
});

describe('tidyBody', () => {
	it('turns empty or odd number boxes into whole numbers, and drops a 0 day limit', () => {
		const b = blankBody('movie') as unknown as Record<string, unknown> & PickRulesBody;
		b.traits = [{ trait: 'hevc', points: null as unknown as number }, { trait: 'av1', points: 2.6 }];
		b.min_points = '' as unknown as number;
		b.upgrade_min_points = 12;
		b.protocol.wait_minutes_torrent = null as unknown as number;
		b.protocol.wait_minutes_usenet = 30;
		b.proper_policy.max_age_days = 0;
		const t = tidyBody(b);
		expect(t.traits).toEqual([{ trait: 'hevc', points: 0 }, { trait: 'av1', points: 3 }]);
		expect(t.min_points).toBe(0);
		expect(t.upgrade_min_points).toBe(12);
		expect(t.protocol.wait_minutes_torrent).toBe(0);
		expect(t.protocol.wait_minutes_usenet).toBe(30);
		expect('max_age_days' in t.proper_policy).toBe(false);
		b.proper_policy.max_age_days = 14;
		expect(tidyBody(b).proper_policy.max_age_days).toBe(14);
		expect(b.min_points).toBe('');
	});
});

describe('server fields', () => {
	it('maps a server field to the field on the page', () => {
		expect(fieldKey('name')).toBe('name');
		expect(fieldKey('body.steps[3].quality')).toBe('steps');
		expect(fieldKey('traits[0].points')).toBe('traits');
		expect(fieldKey('protocol.wait_minutes_usenet')).toBe('wait_minutes_usenet');
		expect(fieldKey('protocol.prefer')).toBe('prefer');
		expect(fieldKey('proper_policy.max_age_days')).toBe('max_age_days');
		expect(fieldKey('proper_policy.mode')).toBe('proper_policy');
		expect(fieldKey('language.wanted')).toBe('language');
		expect(fieldKey('size_limits.*')).toBe('size_limits');
		expect(fieldKey('')).toBe('');
	});

	it('names a copy', () => {
		expect(copyName('Standard')).toBe('Standard copy');
		expect(copyName('x'.repeat(199))).toHaveLength(200);
	});
});

describe('your own traits', () => {
	const mine: OwnTrait[] = [{ id: 4, key: 'own:4', name: 'Dutch subs', match: 'name', terms: ['NLSubs'] }];

	it('are offered after the built-in ones and named by their name', () => {
		expect(traitsFor(['movie'], mine).at(-1)).toBe('own:4');
		expect(neverChoices(['music'], mine)).toContain('own:4');
		expect(traitLabel('own:4', mine)).toBe('Dutch subs');
		expect(traitLabel('own:9', mine)).toBe('A trait of yours that was removed');
	});

	it('pass the checks only while they exist', () => {
		const b = blankBody('movie');
		b.traits = [{ trait: 'own:4', points: 50 }];
		b.must_have = ['own:4'];
		expect(checkBody('x', ['movie'], b, mine)).toEqual({});
		const gone = checkBody('x', ['movie'], b, []);
		expect(gone.traits).toMatch(/removed/);
		expect(gone.must_have).toMatch(/removed/);
	});

	it('read one term per line and check them like the server', () => {
		expect(ownTerms(' NLSubs \r\n\n/\\bdutch\\b/\n')).toEqual(['NLSubs', '/\\bdutch\\b/']);
		expect(checkOwnTrait('Dutch', ['x'])).toEqual({});
		expect(checkOwnTrait(' ', ['x']).name).toBeTruthy();
		expect(checkOwnTrait('x'.repeat(61), ['x']).name).toBeTruthy();
		expect(checkOwnTrait('A', []).terms).toBeTruthy();
		expect(checkOwnTrait('A', ['/(?i)x265/']).terms).toBeUndefined();
	});
});
