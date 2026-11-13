// Words and checks for the Pick rules editor, kept apart from the pages so they can be tested. The checks mirror the server's so a mistake shows at once; the server still has the last word.
import { t as say } from '#lib/core/i18n/i18n.svelte.ts';
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
	tup<[string, string]>('en', () => say('downloads.pr.lang.en')),
	tup<[string, string]>('fr', () => say('downloads.pr.lang.fr')),
	tup<[string, string]>('de', () => say('downloads.pr.lang.de')),
	tup<[string, string]>('it', () => say('downloads.pr.lang.it')),
	tup<[string, string]>('es', () => say('downloads.pr.lang.es')),
	tup<[string, string]>('ru', () => say('downloads.pr.lang.ru')),
	tup<[string, string]>('ja', () => say('downloads.pr.lang.ja')),
	tup<[string, string]>('ko', () => say('downloads.pr.lang.ko')),
	tup<[string, string]>('pl', () => say('downloads.pr.lang.pl')),
	tup<[string, string]>('nl', () => say('downloads.pr.lang.nl')),
	tup<[string, string]>('pt', () => say('downloads.pr.lang.pt')),
	tup<[string, string]>('sv', () => say('downloads.pr.lang.sv')),
	tup<[string, string]>('da', () => say('downloads.pr.lang.da')),
	tup<[string, string]>('no', () => say('downloads.pr.lang.no')),
	tup<[string, string]>('fi', () => say('downloads.pr.lang.fi')),
	tup<[string, string]>('cs', () => say('downloads.pr.lang.cs')),
	tup<[string, string]>('hu', () => say('downloads.pr.lang.hu')),
	tup<[string, string]>('el', () => say('downloads.pr.lang.el')),
	tup<[string, string]>('he', () => say('downloads.pr.lang.he')),
	tup<[string, string]>('tr', () => say('downloads.pr.lang.tr')),
	tup<[string, string]>('ar', () => say('downloads.pr.lang.ar')),
	tup<[string, string]>('hi', () => say('downloads.pr.lang.hi')),
	tup<[string, string]>('zh', () => say('downloads.pr.lang.zh')),
	tup<[string, string]>('th', () => say('downloads.pr.lang.th')),
	tup<[string, string]>('vi', () => say('downloads.pr.lang.vi')),
	tup<[string, string]>('uk', () => say('downloads.pr.lang.uk'))
];

export const kindChoices: [RulesKind, string][] = [
	tup<[RulesKind, string]>('movie', () => say('downloads.kind.movies')),
	tup<[RulesKind, string]>('show', () => say('downloads.kind.shows')),
	tup<[RulesKind, string]>('music', () => say('downloads.kind.music'))
];

export const preferChoices: { value: PickRulesBody['protocol']['prefer']; label: string }[] = [
	{ value: 'either', get label() { return say('downloads.pr.either'); } },
	{ value: 'torrent', label: 'Torrent' },
	{ value: 'usenet', label: 'Usenet' }
];

export const orderChoices: [PickRulesBody['order'], string, string][] = [
	tup<[PickRulesBody['order'], string, string]>('quality_then_points', () => say('downloads.pr.order.quality_then_points.label'), () => say('downloads.pr.order.quality_then_points.desc')),
	tup<[PickRulesBody['order'], string, string]>('points_then_quality', () => say('downloads.pr.order.points_then_quality.label'), () => say('downloads.pr.order.points_then_quality.desc'))
];

export const properChoices: [PickRulesBody['proper_policy']['mode'], string, string][] = [
	tup<[PickRulesBody['proper_policy']['mode'], string, string]>('take_any', () => say('downloads.pr.proper.take_any.label'), () => say('downloads.pr.proper.take_any.desc')),
	tup<[PickRulesBody['proper_policy']['mode'], string, string]>('same_group_only', () => say('downloads.pr.proper.same_group_only.label'), () => say('downloads.pr.proper.same_group_only.desc')),
	tup<[PickRulesBody['proper_policy']['mode'], string, string]>('prefer_only', () => say('downloads.pr.proper.prefer_only.label'), () => say('downloads.pr.proper.prefer_only.desc')),
	tup<[PickRulesBody['proper_policy']['mode'], string, string]>('ignore', () => say('downloads.pr.proper.ignore.label'), () => say('downloads.pr.proper.ignore.desc'))
];



const plain = (v: string) => v.replaceAll('_', ' ');

/** A line by key, or `fallback` when the key has no line. */
const word = (key: string, fallback: string) => {
	const v = say(key);
	return v === key ? fallback : v;
};

/** A tuple whose parts that are functions are read each time, so they follow the language picked. */
function tup<T extends unknown[]>(...parts: unknown[]): T {
	const a = parts.map((x) => (typeof x === 'function' ? '' : x));
	parts.forEach((x, i) => {
		if (typeof x === 'function') Object.defineProperty(a, i, { get: x as () => string, enumerable: true });
	});
	return a as T;
}

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
	return word(`downloads.pr.steps.${q}`, plain(q));
}

/** Whether a trait key names one of your own traits (own:<id>). */
export function isOwnTrait(t: string): boolean {
	return t.startsWith('own:');
}

/** A trait in plain words; `mine` names your own traits. */
export function traitLabel(t: string, mine: OwnTrait[] = []): string {
	if (isOwnTrait(t)) return mine.find((o) => o.key === t)?.name ?? say('downloads.pr.removedTrait');
	return word(`downloads.pr.traits.${t}`, plain(t));
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
	if (!n) err.name = say('downloads.pr.err.traitName');
	else if ([...n].length > 60) err.name = say('downloads.pr.err.name60');
	if (terms.length === 0) err.terms = say('downloads.pr.err.termsMin');
	else if (terms.length > 50) err.terms = say('downloads.pr.err.terms50');
	else if (terms.some((t) => [...t].length > 200)) err.terms = say('downloads.pr.err.term200');
	// Patterns are left to the server because its patterns differ from the browser's.
	return err;
}

/** A size trait's one term, "over-upto" in GB; an empty end is open. */
export function sizeTerm(over: string, upTo: string): string {
	return `${over.trim()}-${upTo.trim()}`;
}

/** Splits a size term back into its two ends. */
export function sizeParts(term: string | undefined): { over: string; upTo: string } {
	const [over = '', upTo = ''] = (term ?? '').split('-');
	return { over: over.trim(), upTo: upTo.trim() };
}

/** Finds what the server would refuse in a size trait. */
export function checkSizeTrait(name: string, over: string, upTo: string): Record<string, string> {
	const err = checkOwnTrait(name, ['x']);
	const read = (s: string) => (s.trim() === '' ? 0 : Number(s));
	const lo = read(over);
	const hi = read(upTo);
	if (!Number.isFinite(lo) || !Number.isFinite(hi) || lo < 0 || hi < 0 || (lo === 0 && hi === 0) || (hi > 0 && hi <= lo)) err.terms = say('downloads.pr.err.sizeRange');
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

const useKeys: (keyof PickRulesUse)[] = ['versions', 'library_folders', 'outside_lists', 'wishlist', 'title_defaults'];

/** Where Pick rules are used: the full counts from one rule, or the total from the list. */
export function usedWords(use: PickRulesUse | number | undefined): string {
	if (use === undefined) return '';
	if (typeof use === 'number') return use === 0 ? say('downloads.pr.notUsed') : say(use === 1 ? 'downloads.pr.usedIn.one' : 'downloads.pr.usedIn.other', { count: use });
	const parts = useKeys.filter((k) => use[k] > 0).map((k) => say(`downloads.pr.use.${k}.${use[k] === 1 ? 'one' : 'other'}`, { count: use[k] }));
	return parts.length ? say('downloads.pr.usedBy', { list: joinWords(parts) }) : say('downloads.pr.notUsed');
}

/** The server's bound for any points value, both ways: a 32-bit number, as the old apps keep scores. */
export const maxPoints = 2147483647;
const inPoints = (n: number) => Number.isInteger(n) && n >= -maxPoints && n <= maxPoints;
const pointsText = () => say('downloads.pr.err.points');
const waitText = () => say('downloads.pr.err.wait');

/** Finds what the server would refuse, as field to words. Empty means it looks fine. */
export function checkBody(name: string, kinds: string[], body: PickRulesBody, mine: OwnTrait[] = []): Record<string, string> {
	const err: Record<string, string> = {};
	const n = name.trim();
	if (!n) err.name = say('downloads.pr.err.rulesName');
	else if ([...n].length > 200) err.name = say('downloads.pr.err.name200');

	if (kinds.length === 0 || kinds.length > 3) err.kinds = say('downloads.pr.err.kinds');
	else if (new Set(kinds).size !== kinds.length) err.kinds = say('downloads.pr.err.kindOnce');
	else if (kinds.some((k) => !kindChoices.some(([c]) => c === k))) err.kinds = say('downloads.pr.err.kinds');
	else if (isMusic(kinds) && kinds.length > 1) err.kinds = say('downloads.pr.err.musicAlone');

	const ladder = ladderFor(kinds);
	const qualities = body.steps.map((s) => s.quality);
	if (qualities.some((q) => !ladder.includes(q))) err.steps = say('downloads.pr.err.stepsOther');
	else if (new Set(qualities).size !== qualities.length) err.steps = say('downloads.pr.err.stepOnce');
	else if (!body.steps.some((s) => s.allowed)) err.steps = say('downloads.pr.err.stepOn');
	if (!err.steps && !body.steps.some((s) => s.allowed && s.quality === body.good_enough_at)) err.good_enough_at = say('downloads.pr.err.goodEnough');

	const own = traitsFor(kinds, mine);
	const names = body.traits.map((t) => t.trait);
	if (new Set(names).size !== names.length) err.traits = say('downloads.pr.err.traitOnce');
	else if (names.some((t) => !own.includes(t))) {
		const bad = names.find((t) => !own.includes(t)) ?? '';
		err.traits = isOwnTrait(bad) ? say('downloads.pr.err.ownRemoved') : say('downloads.pr.err.notTrait', { bad });
	}
	else if (body.traits.some((t) => !inPoints(t.points))) err.traits = pointsText();

	const must = listProblem(body.must_have, own, say('downloads.pr.err.aMust'));
	if (must) err.must_have = must;
	const never = listProblem(body.never, neverChoices(kinds, mine), say('downloads.pr.err.inNever'));
	if (never) err.never = never;

	if (!inPoints(body.min_points)) err.min_points = pointsText();
	if (!inPoints(body.upgrade_min_points)) err.upgrade_min_points = pointsText();
	if (body.good_enough_points !== undefined && !inPoints(body.good_enough_points)) err.good_enough_points = pointsText();

	for (const [step, l] of Object.entries(body.size_limits)) {
		if (step !== '*' && !qualities.includes(step)) err.size_limits = say('downloads.pr.err.sizeStep');
		else if (l.min < 0 || l.max < 0) err.size_limits = say('downloads.pr.err.sizeNeg');
		else if (l.max < l.min) err.size_limits = say('downloads.pr.err.sizeOrder');
		else if (l.max > 2 ** 50) err.size_limits = say('downloads.pr.err.sizeBig');
		if (err.size_limits) break;
	}

	const wait = (m: number) => Number.isInteger(m) && m >= 0 && m <= 10080;
	if (!wait(body.protocol.wait_minutes_torrent)) err.wait_minutes_torrent = waitText();
	if (!wait(body.protocol.wait_minutes_usenet)) err.wait_minutes_usenet = waitText();

	const known = languages.map(([c]) => c);
	const lang = body.language.wanted.find((l) => !known.includes(l));
	if (lang !== undefined) err.language = say('downloads.pr.err.langUnknown', { lang });
	else if (new Set(body.language.wanted).size !== body.language.wanted.length) err.language = say('downloads.pr.err.langOnce');

	const age = body.proper_policy.max_age_days ?? 0;
	if (!Number.isInteger(age) || age < 0 || age > 36500) err.max_age_days = say('downloads.pr.err.days');
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
	// An empty good enough points box means the step alone decides.
	const gep = b.good_enough_points as unknown;
	if (gep === undefined || gep === null || gep === '') delete b.good_enough_points;
	else b.good_enough_points = whole(gep);
	b.protocol.wait_minutes_torrent = whole(b.protocol.wait_minutes_torrent);
	b.protocol.wait_minutes_usenet = whole(b.protocol.wait_minutes_usenet);
	if (!b.tags?.length) delete b.tags;
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
	return parts.length < 2 ? parts.join('') : say('downloads.pr.andJoin', { first: parts.slice(0, -1).join(', '), last: parts.at(-1) ?? '' });
}

function listProblem(values: string[], allowed: string[], what: string): string | undefined {
	if (new Set(values).size !== values.length) return say('downloads.pr.err.traitOnce');
	const bad = values.find((v) => !allowed.includes(v));
	if (bad === undefined) return undefined;
	if (safetyTraits.includes(bad)) return say('downloads.pr.err.cannotBe', { trait: traitLabel(bad), what });
	return isOwnTrait(bad) ? say('downloads.pr.err.ownRemoved') : say('downloads.pr.err.notTrait', { bad });
}
