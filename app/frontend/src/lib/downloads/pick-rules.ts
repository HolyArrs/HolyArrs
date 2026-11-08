// Words and checks for the Pick rules editor, kept apart from the pages so they can be tested. The checks mirror the server's so a mistake shows at once; the server still has the last word.
import type { OwnTrait, PickRulesBody, PickRulesSize, PickRulesUse } from './types.ts';

export type RulesKind = 'movie' | 'show' | 'music';

export interface SizeFields {
	min: number | '';
	max: number | '';
}

/** The movie and show quality ladder, low to high. */
export const videoLadder = [
	'cam', 'telesync', 'sd_tv', 'sd_dvd',
	'720p_tv', '720p_web', '720p_bluray',
	'1080p_tv', '1080p_web', '1080p_bluray', '1080p_remux',
	'2160p_web', '2160p_bluray', '2160p_remux'
];

/** The music quality ladder, low to high. */
export const musicLadder = ['lossy_low', 'lossy_mid', 'lossy_high', 'lossless', 'lossless_hires'];

const videoTraits = ['hevc', 'ddp_audio', 'web_dl', 'upscaled', 'dubbed_only', 'hardcoded_subs', '3d', 'cam', 'telesync', 'remux', 'av1', 'dv', 'hdr10', 'hdr10plus', 'atmos'];
const musicTraits = ['cd_source', 'web_source', 'vinyl_source', 'log_cue', 'hires', 'mp3', 'aac', 'opus', 'vorbis', 'flac', 'alac', 'single_image', 'lossy_transcode', 'deluxe'];

/** Traits that can only be in Never. */
export const safetyTraits = ['sample', 'executable', 'password_protected', 'disc_image'];

/** Languages HolyArrs can tell apart in a Release name. */
export const languages: [string, string][] = [
	['en', 'English'],
	['fr', 'French'],
	['de', 'German'],
	['it', 'Italian'],
	['es', 'Spanish'],
	['ru', 'Russian'],
	['ja', 'Japanese'],
	['ko', 'Korean']
];

export const kindChoices: [RulesKind, string][] = [
	['movie', 'Movies'],
	['show', 'TV shows'],
	['music', 'Music']
];

export const preferChoices: { value: PickRulesBody['protocol']['prefer']; label: string }[] = [
	{ value: 'either', label: 'Either' },
	{ value: 'torrent', label: 'Torrent' },
	{ value: 'usenet', label: 'Usenet' }
];

export const orderChoices: [PickRulesBody['order'], string, string][] = [
	['quality_then_points', 'The best step first, then points', 'A higher step always wins; points decide between Releases on the same step.'],
	['points_then_quality', 'Most points first, then the step', 'Points always win; the step decides when the points are equal.']
];

export const properChoices: [PickRulesBody['proper_policy']['mode'], string, string][] = [
	['take_any', 'Replace with any fixed copy', 'A fixed re-release of the same step replaces the file, even once it is good enough.'],
	['same_group_only', 'Only a fix from the same group', 'Only a fixed copy from the group that made the current file replaces it.'],
	['prefer_only', 'Prefer fixed copies, never replace for one', 'A fixed copy wins when choosing, but never replaces a file you have.'],
	['ignore', 'Ignore fixed copies', 'A fixed copy counts like any other Release.']
];

const steps: Record<string, string> = {
	cam: 'Filmed in a cinema',
	telesync: 'Cinema copy with clean sound',
	sd_tv: 'SD from TV',
	sd_dvd: 'DVD',
	'720p_tv': '720p TV',
	'720p_web': '720p web',
	'720p_bluray': '720p Blu-ray',
	'1080p_tv': '1080p TV',
	'1080p_web': '1080p web',
	'1080p_bluray': '1080p Blu-ray',
	'1080p_remux': '1080p remux',
	'2160p_web': '4K web',
	'2160p_bluray': '4K Blu-ray',
	'2160p_remux': '4K remux',
	lossy_low: 'Lossy, under 192 kbps',
	lossy_mid: 'Lossy, 192 to 256 kbps',
	lossy_high: 'Lossy at its best (MP3 320 or V0)',
	lossless: 'Lossless (CD quality)',
	lossless_hires: 'Lossless hi-res'
};

const traits: Record<string, string> = {
	hevc: 'HEVC (smaller files)',
	ddp_audio: 'Dolby Digital Plus sound',
	web_dl: 'Straight from a streaming service',
	upscaled: 'Upscaled from a lower step',
	dubbed_only: 'Dubbed, without the original sound',
	hardcoded_subs: 'Subtitles burned into the picture',
	'3d': '3D',
	cam: 'Filmed in a cinema',
	telesync: 'Cinema copy with clean sound',
	remux: 'Remux (the disc, untouched)',
	av1: 'AV1',
	dv: 'Dolby Vision',
	hdr10: 'HDR10',
	hdr10plus: 'HDR10+',
	atmos: 'Dolby Atmos',
	cd_source: 'Ripped from a CD',
	web_source: 'Bought from a web store',
	vinyl_source: 'Recorded from vinyl',
	log_cue: 'With a rip log and cue sheet',
	hires: 'Hi-res',
	mp3: 'MP3',
	aac: 'AAC',
	opus: 'Opus',
	vorbis: 'Ogg Vorbis',
	flac: 'FLAC',
	alac: 'ALAC (Apple lossless)',
	single_image: 'One file for the whole album',
	lossy_transcode: 'Lossy sound passed off as lossless',
	deluxe: 'Deluxe edition',
	sample: 'Only a short sample',
	executable: 'Holds a program',
	password_protected: 'Locked with a password',
	disc_image: 'A whole disc image'
};

const plain = (v: string) => v.replaceAll('_', ' ');

/** Whether the kinds are music (music stands alone). */
export function isMusic(kinds: string[]): boolean {
	return kinds.includes('music');
}

/** The quality ladder for the kinds. */
export function ladderFor(kinds: string[]): string[] {
	return isMusic(kinds) ? musicLadder : videoLadder;
}

/** A quality step in plain words. */
export function stepLabel(q: string): string {
	return steps[q] ?? plain(q);
}

/** Whether a trait key names one of your own traits (own:<id>). */
export function isOwnTrait(t: string): boolean {
	return t.startsWith('own:');
}

/** A trait in plain words; `mine` names your own traits. */
export function traitLabel(t: string, mine: OwnTrait[] = []): string {
	if (isOwnTrait(t)) return mine.find((o) => o.key === t)?.name ?? 'A trait of yours that was removed';
	return traits[t] ?? plain(t);
}

/** Traits that can earn points or be a must-have for the kinds, your own last. */
export function traitsFor(kinds: string[], mine: OwnTrait[] = []): string[] {
	return [...(isMusic(kinds) ? musicTraits : videoTraits), ...mine.map((o) => o.key)];
}

/** Traits Never may hold: the kind's own, yours and the safety ones. */
export function neverChoices(kinds: string[], mine: OwnTrait[] = []): string[] {
	return [...traitsFor(kinds, mine), ...safetyTraits];
}

/** Reads a trait's terms box: one term per line, blank lines dropped. */
export function ownTerms(text: string): string[] {
	return text.split(/\r?\n/).map((t) => t.trim()).filter(Boolean);
}

/** Finds what the server would refuse in a trait, as field to words. Patterns are checked only by the server. */
export function checkOwnTrait(name: string, terms: string[]): Record<string, string> {
	const err: Record<string, string> = {};
	const n = name.trim();
	if (!n) err.name = 'Give the trait a name.';
	else if ([...n].length > 60) err.name = 'A name can be at most 60 characters.';
	if (terms.length === 0) err.terms = 'List at least one word or pattern.';
	else if (terms.length > 50) err.terms = 'List at most 50 terms.';
	else if (terms.some((t) => [...t].length > 200)) err.terms = 'Each term holds at most 200 characters.';
	// Patterns are left to the server because its patterns differ from the browser's.
	return err;
}

/** Turns one kind on or off; music cannot share with movies or TV shows. */
export function toggleKind(kinds: string[], kind: RulesKind, on: boolean): string[] {
	if (!on) return kinds.filter((k) => k !== kind);
	if (kind === 'music') return ['music'];
	const next = new Set(kinds.filter((k) => k !== 'music'));
	next.add(kind);
	return kindChoices.map(([k]) => k).filter((k) => next.has(k));
}

const gb = 2 ** 30;

/** Where new Pick rules start: sensible steps, the safety Nevers, no traits. */
export function blankBody(kind: RulesKind): PickRulesBody {
	const music = kind === 'music';
	const on = music ? ['lossy_high', 'lossless', 'lossless_hires'] : ['720p_tv', '720p_web', '720p_bluray', '1080p_tv', '1080p_web', '1080p_bluray'];
	return {
		steps: (music ? musicLadder : videoLadder).map((quality) => ({ quality, allowed: on.includes(quality) })),
		good_enough_at: music ? 'lossless' : '1080p_web',
		traits: [],
		must_have: [],
		never: [...safetyTraits],
		min_points: 0,
		// 0.5 to 6 GB per hour of video; album sizes vary too much for one limit.
		size_limits: music ? {} : { '*': { min: Math.floor(gb / 2 / 60), max: Math.floor((6 * gb) / 60), per_minute: true } },
		protocol: { prefer: 'either', wait_minutes_torrent: 0, wait_minutes_usenet: 0, skip_wait_at_good_enough: false },
		language: { wanted: [], original_ok: true },
		order: 'quality_then_points',
		upgrade_min_points: 10,
		proper_policy: { mode: 'take_any' }
	};
}

const useWords: [keyof PickRulesUse, string, string][] = [
	['versions', 'Version', 'Versions'],
	['library_folders', 'Library folder', 'Library folders'],
	['outside_lists', 'Outside list', 'Outside lists'],
	['wishlist', 'Request rule', 'Request rules'],
	['title_defaults', 'Movies and TV default', 'Movies and TV defaults']
];

/** Where Pick rules are used: the full counts from one rule, or the total from the list. */
export function usedWords(use: PickRulesUse | number | undefined): string {
	if (use === undefined) return '';
	if (typeof use === 'number') return use === 0 ? 'Not used yet' : `Used in ${use} ${use === 1 ? 'place' : 'places'}`;
	const parts = useWords.filter(([k]) => use[k] > 0).map(([k, one, many]) => `${use[k]} ${use[k] === 1 ? one : many}`);
	return parts.length ? `Used by ${joinWords(parts)}` : 'Not used yet';
}

const inPoints = (n: number) => Number.isInteger(n) && n >= -10000 && n <= 10000;
const pointsText = 'Points go from -10000 to 10000.';
const waitText = 'A wait can be 0 to 10080 minutes (7 days).';

/** Finds what the server would refuse, as field to words. Empty means it looks fine. */
export function checkBody(name: string, kinds: string[], body: PickRulesBody, mine: OwnTrait[] = []): Record<string, string> {
	const err: Record<string, string> = {};
	const n = name.trim();
	if (!n) err.name = 'Give these Pick rules a name.';
	else if ([...n].length > 200) err.name = 'A name can be at most 200 characters.';

	if (kinds.length === 0 || kinds.length > 3) err.kinds = 'Pick movies, shows or music.';
	else if (new Set(kinds).size !== kinds.length) err.kinds = 'Each kind can be picked once.';
	else if (kinds.some((k) => !kindChoices.some(([c]) => c === k))) err.kinds = 'Pick movies, shows or music.';
	else if (isMusic(kinds) && kinds.length > 1) err.kinds = 'Music Pick rules are for music alone.';

	const ladder = ladderFor(kinds);
	const qualities = body.steps.map((s) => s.quality);
	if (qualities.some((q) => !ladder.includes(q))) err.steps = 'These steps are for another kind.';
	else if (new Set(qualities).size !== qualities.length) err.steps = 'Each step can be listed once.';
	else if (!body.steps.some((s) => s.allowed)) err.steps = 'Turn on at least one step.';
	if (!err.steps && !body.steps.some((s) => s.allowed && s.quality === body.good_enough_at)) err.good_enough_at = 'Good enough must be a step that is on.';

	const own = traitsFor(kinds, mine);
	const names = body.traits.map((t) => t.trait);
	if (new Set(names).size !== names.length) err.traits = 'Each trait can be listed once.';
	else if (names.some((t) => !own.includes(t))) {
		const bad = names.find((t) => !own.includes(t)) ?? '';
		err.traits = isOwnTrait(bad) ? 'A trait of yours that was removed is still listed; take it out.' : `${bad} is not a trait for these kinds.`;
	}
	else if (body.traits.some((t) => !inPoints(t.points))) err.traits = pointsText;

	const must = listProblem(body.must_have, own, 'a must-have');
	if (must) err.must_have = must;
	const never = listProblem(body.never, neverChoices(kinds, mine), 'in Never');
	if (never) err.never = never;

	if (!inPoints(body.min_points)) err.min_points = pointsText;
	if (!inPoints(body.upgrade_min_points)) err.upgrade_min_points = pointsText;
	if (body.good_enough_points !== undefined && !inPoints(body.good_enough_points)) err.good_enough_points = pointsText;

	for (const [step, l] of Object.entries(body.size_limits)) {
		if (step !== '*' && !qualities.includes(step)) err.size_limits = 'A size is set for a step these rules do not have.';
		else if (l.min < 0 || l.max < 0) err.size_limits = 'Sizes cannot be below 0.';
		else if (l.max < l.min) err.size_limits = 'The smallest size cannot be more than the largest.';
		else if (l.max > 2 ** 50) err.size_limits = 'The largest size is too big.';
		if (err.size_limits) break;
	}

	const wait = (m: number) => Number.isInteger(m) && m >= 0 && m <= 10080;
	if (!wait(body.protocol.wait_minutes_torrent)) err.wait_minutes_torrent = waitText;
	if (!wait(body.protocol.wait_minutes_usenet)) err.wait_minutes_usenet = waitText;

	const known = languages.map(([c]) => c);
	const lang = body.language.wanted.find((l) => !known.includes(l));
	if (lang !== undefined) err.language = `${lang} is not a language HolyArrs knows.`;
	else if (new Set(body.language.wanted).size !== body.language.wanted.length) err.language = 'Each language can be picked once.';

	const age = body.proper_policy.max_age_days ?? 0;
	if (!Number.isInteger(age) || age < 0 || age > 36500) err.max_age_days = 'Days go from 0 to 36500.';
	return err;
}

const whole = (v: unknown): number => {
	const n = v === null || v === undefined || v === '' ? 0 : Number(v);
	return Number.isFinite(n) ? Math.round(n) : 0;
};

/** Copies the body with every number box a whole number; an empty box is 0, and a 0 day limit is left out. */
export function tidyBody(body: PickRulesBody): PickRulesBody {
	const b: PickRulesBody = JSON.parse(JSON.stringify(body));
	b.traits = b.traits.map((t) => ({ trait: t.trait, points: whole(t.points) }));
	b.min_points = whole(b.min_points);
	b.upgrade_min_points = whole(b.upgrade_min_points);
	if (b.good_enough_points !== undefined && b.good_enough_points !== null) b.good_enough_points = whole(b.good_enough_points);
	b.protocol.wait_minutes_torrent = whole(b.protocol.wait_minutes_torrent);
	b.protocol.wait_minutes_usenet = whole(b.protocol.wait_minutes_usenet);
	const days = whole(b.proper_policy.max_age_days);
	if (days) b.proper_policy.max_age_days = days;
	else delete b.proper_policy.max_age_days;
	return b;
}

/** The server's ceiling for a size; as the largest size it means no limit at the top. */
const open = 2 ** 50;

/** Bytes per minute of runtime as GB per hour, to two places. */
export function gbPerHour(bytesPerMin: number): number {
	return Math.round(((bytesPerMin * 60) / gb) * 100) / 100;
}

/** GB per hour as bytes per minute of runtime. */
export function bytesPerMinute(gbHour: number): number {
	return Math.round((gbHour * gb) / 60);
}

/** The Every step limit as GB per hour; empty is no limit. */
export function sizeFields(limits: Record<string, PickRulesSize>): SizeFields {
	const l = limits['*'];
	if (!l || !l.per_minute) return { min: '', max: '' };
	return { min: gbPerHour(l.min), max: l.max === 0 || l.max >= open ? '' : gbPerHour(l.max) };
}

/** Writes the Every step fields back into a copy of the body; untouched fields keep their exact bytes. */
export function applySize(body: PickRulesBody, fields: SizeFields, start: SizeFields): PickRulesBody {
	const size_limits = { ...body.size_limits };
	if (fields.min === start.min && fields.max === start.max) return { ...body, size_limits };
	if (fields.min === '' && fields.max === '') delete size_limits['*'];
	else size_limits['*'] = { min: fields.min === '' ? 0 : bytesPerMinute(fields.min), max: fields.max === '' ? open : bytesPerMinute(fields.max), per_minute: true };
	return { ...body, size_limits };
}

const nested: Record<string, string> = {
	'protocol.prefer': 'prefer',
	'protocol.wait_minutes_torrent': 'wait_minutes_torrent',
	'protocol.wait_minutes_usenet': 'wait_minutes_usenet',
	'protocol.skip_wait_at_good_enough': 'prefer',
	'proper_policy.max_age_days': 'max_age_days'
};

/** Maps the field a server error names (such as "body.steps[2].quality") to the field on the page. */
export function fieldKey(field: string): string {
	const parts = field
		.replace(/^body\./, '')
		.replace(/\[[^\]]*\]/g, '')
		.split('.')
		.filter(Boolean);
	if (!parts.length) return '';
	return nested[`${parts[0]}.${parts[1]}`] ?? parts[0];
}

/** Names a copy, kept within the 200-character limit. */
export function copyName(name: string): string {
	const tail = ' copy';
	return [...name].slice(0, 200 - tail.length).join('') + tail;
}

function joinWords(parts: string[]): string {
	return parts.length < 2 ? parts.join('') : `${parts.slice(0, -1).join(', ')} and ${parts.at(-1)}`;
}

function listProblem(values: string[], allowed: string[], what: string): string | undefined {
	if (new Set(values).size !== values.length) return 'Each trait can be listed once.';
	const bad = values.find((v) => !allowed.includes(v));
	if (bad === undefined) return undefined;
	if (safetyTraits.includes(bad)) return `${traitLabel(bad)} cannot be ${what}.`;
	return isOwnTrait(bad) ? 'A trait of yours that was removed is still listed; take it out.' : `${bad} is not a trait for these kinds.`;
}
