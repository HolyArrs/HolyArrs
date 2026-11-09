import { episodeCode } from '../core/episodes.ts';

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

export const hiWords: Record<HI, string> = {
	normal: 'Normal (a hearing-impaired one counts too)',
	hi: 'Hearing-impaired only',
	either: 'Either'
};

export const pauseWords: Record<string, string> = {
	down: 'it did not answer',
	rate_limited: 'it asked HolyArrs to slow down',
	download_limit: 'its download limit for today is reached',
	login_refused: 'it refused the account details',
	blocked: 'its address is on the local network, which it may not reach',
	unreadable: 'its answer could not be read'
};

export const accountWords: Record<string, string> = Object.fromEntries([
	['username', 'User name'],
	['password', 'Password'],
	['api_key', 'API key']
]);

export const partWords: Record<string, string> = {
	name: 'name',
	year: 'year',
	season: 'season',
	episode: 'episode',
	group: 'release group',
	source: 'source (Blu-ray, WEB…)',
	resolution: 'resolution',
	codec: 'video codec',
	audio: 'audio',
	edition: 'edition',
	as_wished: 'hearing-impaired and forced as wished',
	trusted: 'trusted uploader',
	sync: 'frame rate hint'
};

export function langName(code: string, langs: Language[]): string {
	return langs.find((l) => l.code === code)?.name ?? code;
}

export function wantLabel(w: Want, langs: Language[]): string {
	const parts = [langName(w.language, langs)];
	if (w.hearing_impaired === 'hi') parts.push('hearing-impaired only');
	if (w.hearing_impaired === 'either') parts.push('normal or hearing-impaired');
	if (w.forced) parts.push('forced only');
	return parts.join(', ');
}

export function wishLine(ws: Want[], langs: Language[]): string {
	return ws.length ? ws.map((w) => wantLabel(w, langs)).join(' · ') : 'None';
}

export function fromWords(from: string, movie: boolean): string {
	switch (from) {
		case 'episode':
			return 'set for this episode';
		case 'version':
			return 'set for this Version';
		case 'title':
			return movie ? 'set for this movie' : 'set for this show';
		default:
			return 'the default languages';
	}
}

export function wishProblem(ws: Want[]): string {
	if (ws.length > 10) return 'At most 10 languages.';
	const seen = new Set<string>();
	for (const w of ws) {
		if (!w.language) return 'Pick a language on every row.';
		const k = `${w.language}|${w.forced}`;
		if (seen.has(k)) return 'A language can be there twice only once full and once forced.';
		seen.add(k);
	}
	return '';
}

export function kindLabel(s: Pick<SubtitleRow, 'hearing_impaired' | 'forced'>): string {
	if (s.forced) return 'Forced';
	if (s.hearing_impaired) return 'Hearing-impaired';
	return 'Normal';
}

export function sourceLabel(s: Pick<SubtitleRow, 'place' | 'origin' | 'site' | 'codec'>): string {
	if (s.place === 'built_in') return s.codec ? `Inside the video (${s.codec})` : 'Inside the video';
	if (s.origin === 'found') return 'Found beside the video';
	if (s.origin === 'by_hand') return s.site ? `Chosen by hand from ${s.site}` : 'Chosen by hand';
	return s.site ? `From ${s.site}` : 'Saved by HolyArrs';
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
		const next = r.next_try_at ? `; next try ${when(r.next_try_at)}` : '';
		return `None over the Pass mark yet (${r.tries} ${r.tries === 1 ? 'try' : 'tries'})${next}`;
	}
	return r.tries ? `Not found yet (${r.tries} ${r.tries === 1 ? 'try' : 'tries'})` : 'Not looked for yet';
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
	if (k?.needs_account && !s.has_account) return { label: 'Needs your account', text: 'Fill in its account details; until then it is never asked.', tone: 'fail' };
	if (s.state === 'disabled') return { label: 'Paused', text: 'Paused by you: searches skip it.', tone: 'idle' };
	if (s.state === 'paused') {
		const why = (s.pause_reason && pauseWords[s.pause_reason]) || 'it failed';
		const until = s.paused_until ? ` until ${when(s.paused_until)}` : '';
		return { label: 'Resting', text: `Resting${until}: ${why}. HolyArrs tries it again after that.`, tone: 'wait' };
	}
	return { label: 'On', text: 'Asked on every subtitle search.', tone: 'ok' };
}

export function kindCovers(k: Pick<SiteKind, 'movies' | 'shows'>): string {
	if (k.movies && k.shows) return 'movies and shows';
	return k.movies ? 'movies only' : 'shows only';
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
		text: p.matched ? `+${p.points} ${partWords[p.name] ?? p.name} matches` : `${partWords[p.name] ?? p.name} does not match`,
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
	if (!ms) return 'No shift';
	const s = Math.abs(ms) / 1000;
	return `${s} ${s === 1 ? 'second' : 'seconds'} ${ms > 0 ? 'later' : 'earlier'}`;
}

export function jobsDone(ids: number[], states: Record<number, string>): number {
	return ids.filter((id) => ['done', 'failed', 'cancelled'].includes(states[id] ?? '')).length;
}
