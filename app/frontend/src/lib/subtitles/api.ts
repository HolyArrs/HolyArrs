import { episodeCode } from '../core/episodes.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type HI = 'normal' | 'hi' | 'either';

export interface Want {
	language: string;
	hearing_impaired: HI;
	forced: boolean;
}

export interface Language {
	code: string;
	name: string;
}

export interface SubtitleSettings {
	languages: Want[];
	pass_mark_movie: number;
	pass_mark_episode: number;
	use_built_in: boolean;
	ignore_picture_tracks: boolean;
	hi_suffix: 'hi' | 'cc' | 'sdh';
	search_hours: number;
	better: boolean;
	better_hours: number;
	better_days: number;
	batch: number;
	adaptive: boolean;
	sync_from_fps: number;
	sync_to_fps: number;
	sync_step_ms: number;
}

export interface SiteLimits {
	gap_ms: number;
	per_window: number;
	window_seconds: number;
	downloads_per_day: number;
}

export interface SiteView {
	ID: number;
	name: string;
	kind: string;
	url: string;
	state: 'enabled' | 'paused' | 'disabled';
	pause_reason: string | null;
	paused_until: string | null;
	failures: number;
	limits: SiteLimits;
	downloads_today: number;
	allow_local_network: boolean;
	priority: number;
	has_account: boolean;
}

export interface SiteKind {
	kind: string;
	name: string;
	default_url?: string;
	movies: boolean;
	shows: boolean;
	needs_account: boolean;
	account?: ('username' | 'password' | 'api_key')[];
	note?: string;
	default_off?: boolean;
	later?: boolean;
}

export interface ScorePart {
	name: string;
	points: number;
	matched: boolean;
}

export interface SubtitleRow {
	ID: number;
	file_id: number;
	language: string;
	forced: boolean;
	hearing_impaired: boolean;
	place: 'built_in' | 'beside' | 'wish';
	codec?: string;
	relative_path?: string;
	site?: string;
	site_key?: string;
	score?: number;
	score_parts: ScorePart[];
	origin: 'found' | 'fetched' | 'by_hand';
	state: 'present' | 'replaced' | 'gone' | 'wanted' | 'none_good_enough';
	saved_at?: string;
	tries: number;
	next_try_at?: string;
	sync: Record<string, unknown>;
}

export interface Resolved {
	languages: Want[];
	from: 'episode' | 'version' | 'title' | 'default' | string;
}

export interface WishRow {
	ID: number;
	title_id: number;
	version_id?: number;
	item_id?: number;
	languages: Want[];
}

export interface SubtitleFile {
	file_id: number;
	item_id: number;
	version_id: number;
	relative_path: string;
	wish: Resolved;
	subtitles: SubtitleRow[];
}

export interface TitleSubtitles {
	wishes: WishRow[];
	files: SubtitleFile[];
}

export interface Candidate {
	site: string;
	key: string;
	language: string;
	hearing_impaired: boolean;
	forced: boolean;
	release_names?: string[];
	title?: string;
	year?: number;
	season?: number;
	episode?: number;
	uploader?: string;
	trusted?: boolean;
	downloads?: number;
	format?: string;
	score: { total: number; parts: ScorePart[]; refused?: string };
}

export interface Outcome {
	want: Want;
	result: string;
	path?: string;
	score?: number;
	site?: string;
	reason?: string;
	choices?: Candidate[];
}

export interface Choices {
	file_id: number;
	outcomes: Outcome[];
	sites_asked: string[];
}

export interface MissingRow {
	file_id: number;
	item_id: number;
	title_id: number;
	title: string;
	kind: 'movie' | 'episode';
	season?: number;
	episode?: number;
	language: string;
	forced: boolean;
	hearing_impaired: boolean;
	state: 'wanted' | 'none_good_enough';
	tries: number;
	next_try_at?: string;
}

export interface MissingTitle {
	id: number;
	name: string;
	kind: string;
	count: number;
}

export interface SiteForm {
	kind: string;
	name: string;
	url: string;
	priority: number;
	allow_local_network: boolean;
	enabled: boolean;
	limits: SiteLimits;
	username: string;
	password: string;
	api_key: string;
}

// These word lists use getters, so each line follows the language picked when it is read.
export const hiWords: Record<HI, string> = {
	get normal() { return t('subtitles.hi.normal'); },
	get hi() { return t('subtitles.hi.only'); },
	get either() { return t('subtitles.hi.either'); }
};

export const pauseWords: Record<string, string> = {
	get down() { return t('subtitles.pause.down'); },
	get rate_limited() { return t('subtitles.pause.rateLimited'); },
	get download_limit() { return t('subtitles.pause.downloadLimit'); },
	get login_refused() { return t('subtitles.pause.loginRefused'); },
	get blocked() { return t('subtitles.pause.blocked'); },
	get unreadable() { return t('subtitles.pause.unreadable'); }
};

export const accountWords: Record<string, string> = {
	get username() { return t('subtitles.account.username'); },
	get password() { return t('subtitles.account.passwordLabel'); },
	get api_key() { return t('subtitles.account.apiKeyLabel'); }
};

export const partWords: Record<string, string> = {
	get name() { return t('subtitles.part.name'); },
	get year() { return t('subtitles.part.year'); },
	get season() { return t('subtitles.part.season'); },
	get episode() { return t('subtitles.part.episode'); },
	get group() { return t('subtitles.part.group'); },
	get source() { return t('subtitles.part.source'); },
	get resolution() { return t('subtitles.part.resolution'); },
	get codec() { return t('subtitles.part.codec'); },
	get audio() { return t('subtitles.part.audio'); },
	get edition() { return t('subtitles.part.edition'); },
	get as_wished() { return t('subtitles.part.asWished'); },
	get trusted() { return t('subtitles.part.trusted'); },
	get sync() { return t('subtitles.part.sync'); }
};

export function langName(code: string, langs: Language[]): string {
	return langs.find((l) => l.code === code)?.name ?? code;
}

export function wantLabel(w: Want, langs: Language[]): string {
	const parts = [langName(w.language, langs)];
	if (w.hearing_impaired === 'hi') parts.push(t('subtitles.want.hiOnly'));
	if (w.hearing_impaired === 'either') parts.push(t('subtitles.want.either'));
	if (w.forced) parts.push(t('subtitles.want.forcedOnly'));
	return parts.join(', ');
}

export function wishLine(ws: Want[], langs: Language[]): string {
	return ws.length ? ws.map((w) => wantLabel(w, langs)).join(' · ') : t('subtitles.want.none');
}

export function fromWords(from: string, movie: boolean): string {
	switch (from) {
		case 'episode':
			return t('subtitles.from.episode');
		case 'version':
			return t('subtitles.from.version');
		case 'title':
			return movie ? t('subtitles.from.movie') : t('subtitles.from.show');
		default:
			return t('subtitles.from.default');
	}
}

export function wishProblem(ws: Want[]): string {
	if (ws.length > 10) return t('subtitles.problem.tooMany');
	const seen = new Set<string>();
	for (const w of ws) {
		if (!w.language) return t('subtitles.problem.pick');
		const k = `${w.language}|${w.forced}`;
		if (seen.has(k)) return t('subtitles.problem.twice');
		seen.add(k);
	}
	return '';
}

export function kindLabel(s: Pick<SubtitleRow, 'hearing_impaired' | 'forced'>): string {
	if (s.forced) return t('subtitles.kind.forced');
	if (s.hearing_impaired) return t('subtitles.kind.hi');
	return t('subtitles.kind.normal');
}

export function sourceLabel(s: Pick<SubtitleRow, 'place' | 'origin' | 'site' | 'codec'>): string {
	if (s.place === 'built_in') return s.codec ? t('subtitles.source.insideCodec', { codec: s.codec }) : t('subtitles.source.inside');
	if (s.origin === 'found') return t('subtitles.source.found');
	if (s.origin === 'by_hand') return s.site ? t('subtitles.source.handFrom', { site: s.site }) : t('subtitles.source.hand');
	return s.site ? t('subtitles.source.from', { site: s.site }) : t('subtitles.source.saved');
}

export function ours(s: Pick<SubtitleRow, 'place' | 'origin' | 'state'>): boolean {
	return s.place === 'beside' && s.state === 'present' && (s.origin === 'fetched' || s.origin === 'by_hand');
}

export function present(rows: SubtitleRow[]): SubtitleRow[] {
	const order = { built_in: 0, beside: 1, wish: 2 };
	return rows.filter((r) => r.state === 'present').sort((a, b) => order[a.place] - order[b.place] || a.language.localeCompare(b.language));
}

export function missing(rows: SubtitleRow[]): SubtitleRow[] {
	return rows.filter((r) => r.place === 'wish' && (r.state === 'wanted' || r.state === 'none_good_enough'));
}

export function missingWords(r: Pick<SubtitleRow, 'state' | 'tries'> & { next_try_at?: string }, when: (s: string) => string): string {
	if (r.state === 'none_good_enough') {
		const next = r.next_try_at ? t('subtitles.missing.next', { when: when(r.next_try_at) }) : '';
		return t(r.tries === 1 ? 'subtitles.missing.noneGood.one' : 'subtitles.missing.noneGood.other', { count: r.tries }) + next;
	}
	return r.tries ? t(r.tries === 1 ? 'subtitles.missing.notFound.one' : 'subtitles.missing.notFound.other', { count: r.tries }) : t('subtitles.missing.notLooked');
}

export function fileName(p: string | undefined): string {
	if (!p) return '';
	const i = p.lastIndexOf('/');
	return i < 0 ? p : p.slice(i + 1);
}

export function missingName(r: Pick<MissingRow, 'title' | 'kind' | 'season' | 'episode'>): string {
	if (r.kind === 'episode' && r.season !== undefined && r.episode !== undefined) return `${r.title} ${episodeCode(r.season, r.episode)}`;
	return r.title;
}

export function missingQuery(f: { language?: string; kind?: string; title?: number }, limit: number, offset: number): string {
	const q = new URLSearchParams();
	if (f.language) q.set('language', f.language);
	if (f.kind) q.set('kind', f.kind);
	if (f.title) q.set('title', String(f.title));
	q.set('limit', String(limit));
	if (offset) q.set('offset', String(offset));
	return q.toString();
}

export function siteState(s: Pick<SiteView, 'state' | 'pause_reason' | 'paused_until' | 'has_account'>, k: SiteKind | undefined, when: (s: string) => string): { label: string; text: string; tone: 'ok' | 'wait' | 'fail' | 'idle' } {
	if (k?.needs_account && !s.has_account) return { label: t('subtitles.site.needsAccount'), text: t('subtitles.site.needsAccountText'), tone: 'fail' };
	if (s.state === 'disabled') return { label: t('subtitles.site.paused'), text: t('subtitles.site.pausedText'), tone: 'idle' };
	if (s.state === 'paused') {
		const why = (s.pause_reason && pauseWords[s.pause_reason]) || t('subtitles.pause.failed');
		return { label: t('subtitles.site.resting'), text: s.paused_until ? t('subtitles.site.restingUntil', { when: when(s.paused_until), why }) : t('subtitles.site.restingText', { why }), tone: 'wait' };
	}
	return { label: t('subtitles.site.on'), text: t('subtitles.site.onText'), tone: 'ok' };
}

export function kindCovers(k: Pick<SiteKind, 'movies' | 'shows'>): string {
	if (k.movies && k.shows) return t('subtitles.covers.both');
	return k.movies ? t('subtitles.covers.movies') : t('subtitles.covers.shows');
}

export function siteForm(k: SiteKind, s?: SiteView): SiteForm {
	return {
		kind: k.kind,
		name: s?.name ?? k.name,
		url: s?.url ?? k.default_url ?? 'http://',
		priority: s?.priority ?? 25,
		allow_local_network: s?.allow_local_network ?? false,
		enabled: s ? s.state !== 'disabled' : !k.default_off,
		limits: s ? { ...s.limits } : { gap_ms: 1000, per_window: 60, window_seconds: 3600, downloads_per_day: 0 },
		username: '',
		password: '',
		api_key: ''
	};
}

export function siteBody(f: SiteForm): Record<string, unknown> {
	const b: Record<string, unknown> = {
		kind: f.kind,
		name: f.name.trim(),
		url: f.url.trim(),
		priority: Number(f.priority),
		allow_local_network: f.allow_local_network,
		enabled: f.enabled,
		limits: {
			gap_ms: Number(f.limits.gap_ms),
			per_window: Number(f.limits.per_window),
			window_seconds: Number(f.limits.window_seconds),
			downloads_per_day: Number(f.limits.downloads_per_day)
		}
	};
	for (const k of ['username', 'password', 'api_key'] as const) if (f[k]) b[k] = f[k];
	return b;
}

export function accountMissing(k: SiteKind, f: SiteForm, hasAccount: boolean): string[] {
	if (!k.needs_account) return [];
	const typed = (k.account ?? []).filter((a) => f[a]);
	if (hasAccount && typed.length === 0) return [];
	return (k.account ?? []).filter((a) => !f[a]);
}

export function scoreLines(parts: ScorePart[]): { text: string; matched: boolean }[] {
	const out = parts.map((p) => ({
		text: p.matched ? t('subtitles.score.matches', { points: p.points, part: partWords[p.name] ?? p.name }) : t('subtitles.score.noMatch', { part: partWords[p.name] ?? p.name }),
		matched: p.matched
	}));
	return out.sort((a, b) => Number(b.matched) - Number(a.matched));
}

export function passes(score: number, passMark: number): boolean {
	return score >= passMark;
}

export function syncBody(offsetMs: number, fps: { on: boolean; from: number; to: number }): { offset_ms?: number; from_fps?: number; to_fps?: number } {
	const b: { offset_ms?: number; from_fps?: number; to_fps?: number } = {};
	if (offsetMs) b.offset_ms = Math.round(offsetMs);
	if (fps.on && fps.from && fps.to && fps.from !== fps.to) {
		b.from_fps = Number(fps.from);
		b.to_fps = Number(fps.to);
	}
	return b;
}

export function offsetWords(ms: number): string {
	if (!ms) return t('subtitles.offset.none');
	const s = Math.abs(ms) / 1000;
	return t(`subtitles.offset.${ms > 0 ? 'later' : 'earlier'}.${s === 1 ? 'one' : 'other'}`, { s });
}

export function jobsDone(ids: number[], states: Record<number, string>): number {
	return ids.filter((id) => ['done', 'failed', 'cancelled'].includes(states[id] ?? '')).length;
}
